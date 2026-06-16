package pipeline

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"time"

	"github.com/FelipeFuhr/ffreis-platform-cli/pkg/ui"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

// Engine executes a pipeline of steps.
type Engine struct {
	dag      *DAG
	state    *StateStore
	resolver credential.Resolver
	cfg      configctl.Client
	runner   runner.Runner
	log      *slog.Logger
	ui       *ui.Presenter
	project  string
	env      string
	dryRun   bool
	reporter progressReporter
}

type progressReporter interface {
	Report(kind, label, detail string)
}

type noopProgressReporter struct{}

func (noopProgressReporter) Report(string, string, string) {
	// Intentionally empty: noop implementation discards all progress reports.
}

type stderrProgressReporter struct {
	ui  *ui.Presenter
	out io.Writer
}

func (r stderrProgressReporter) Report(kind, label, detail string) {
	if r.ui == nil || r.out == nil {
		return
	}
	_, _ = io.WriteString(r.out, r.ui.Status(kind, label, detail)+"\n")
}

func newProgressReporter(presenter *ui.Presenter, out io.Writer) progressReporter {
	if presenter == nil || !presenter.Interactive() || out == nil {
		return noopProgressReporter{}
	}
	return stderrProgressReporter{ui: presenter, out: out}
}

// EngineOptions configures an Engine instance.
type EngineOptions struct {
	DAG         *DAG
	State       *StateStore
	Resolver    credential.Resolver
	Config      configctl.Client
	Runner      runner.Runner
	Log         *slog.Logger
	ProgressOut io.Writer
	UI          *ui.Presenter
	Project     string
	Env         string
	DryRun      bool
}

// NewEngine constructs an Engine.
func NewEngine(opts EngineOptions) *Engine {
	return &Engine{
		dag:      opts.DAG,
		state:    opts.State,
		resolver: opts.Resolver,
		cfg:      opts.Config,
		runner:   opts.Runner,
		log:      opts.Log,
		ui:       opts.UI,
		project:  opts.Project,
		env:      opts.Env,
		dryRun:   opts.DryRun,
		reporter: newProgressReporter(opts.UI, opts.ProgressOut),
	}
}

// Init starts a fresh run with the given runID.
// It persists RunMeta then executes the full pipeline.
func (e *Engine) Init(ctx context.Context, runID string) error {
	meta := &RunMeta{
		RunID:     runID,
		Project:   e.project,
		Env:       e.env,
		StartedAt: time.Now().UTC(),
		Status:    StatusRunning,
	}
	if err := e.state.SaveRunMeta(ctx, meta); err != nil {
		return fmt.Errorf("save run meta: %w", err)
	}
	err := e.execute(ctx, runID, nil)
	if err != nil {
		e.log.Error("pipeline stopped — run 'resume' to continue or 'rollback' to undo",
			"run_id", runID, "error", err)
		return err
	}
	e.log.Info("pipeline complete", "run_id", runID)
	return nil
}

// Resume loads existing step states and re-runs from the first non-succeeded step.
// RUNNING states are reset to FAILED (process-kill safety).
func (e *Engine) Resume(ctx context.Context, runID string) error {
	existing, err := e.state.LoadAllStepStates(ctx, runID)
	if err != nil {
		return fmt.Errorf("load step states for resume: %w", err)
	}
	// Reset any RUNNING states to FAILED (process was killed mid-step).
	for _, ss := range existing {
		if ss.Status == StatusRunning {
			ss.Status = StatusFailed
			ss.ErrMsg = "run was interrupted (process killed)"
			if serr := e.state.SaveStepState(ctx, runID, ss); serr != nil {
				e.log.Warn("could not update interrupted step state",
					"step", ss.StepID, "error", serr)
			}
		}
	}
	err = e.execute(ctx, runID, existing)
	if err != nil {
		e.log.Error("pipeline stopped — run 'resume' to continue or 'rollback' to undo",
			"run_id", runID, "error", err)
		return err
	}
	e.log.Info("pipeline complete", "run_id", runID)
	return nil
}

// execute is the shared core of Init and Resume.
// existing is nil on a fresh run and populated on resume.
func (e *Engine) execute(ctx context.Context, runID string, existing map[string]*StepState) error {
	sorted, err := e.dag.TopoSort()
	if err != nil {
		return err
	}

	for _, step := range sorted {
		if e.shouldSkipStep(step, existing) {
			continue
		}

		execCtx, err := e.buildExecutionContext(ctx, runID, step)
		if err != nil {
			return err
		}

		if e.shouldSkipBecauseDone(ctx, runID, step, execCtx) {
			continue
		}

		runningState, err := e.transitionToRunning(ctx, runID, step)
		if err != nil {
			return err
		}
		stepStarted := time.Now()
		e.progress("running", "STEP", fmt.Sprintf("%s (%s)", step.ID(), step.Name()))

		finalState, runErr := e.runWithRetry(ctx, step, execCtx, runningState)
		_ = e.state.SaveStepState(ctx, runID, finalState)

		if runErr != nil {
			return e.handleStepFailure(ctx, runID, step, finalState, runErr, stepStarted)
		}

		e.handleStepSuccess(ctx, runID, step, execCtx, finalState, stepStarted)
	}

	// All steps complete.
	meta := &RunMeta{
		RunID:   runID,
		Project: e.project,
		Env:     e.env,
		Status:  StatusSucceeded,
	}
	_ = e.state.SaveRunMeta(ctx, meta)
	return nil
}

func (e *Engine) shouldSkipStep(step Step, existing map[string]*StepState) bool {
	ss := e.stepStateFor(step.ID(), existing)
	if ss == nil {
		return false
	}
	// On resume: trust stored succeeded/skipped state, do not call IsDone again.
	if ss.Status != StatusSucceeded && ss.Status != StatusSkipped {
		return false
	}

	e.log.Info("skipping completed step",
		logKeyStepID, step.ID(),
		"status", string(ss.Status),
	)
	e.progress("muted", "SKIPPED", fmt.Sprintf("%s already %s", step.ID(), ss.Status))
	return true
}

func (e *Engine) buildExecutionContext(ctx context.Context, runID string, step Step) (*ExecutionContext, error) {
	awsCfg, err := e.resolver.Resolve(ctx, step.CredentialClass())
	if err != nil {
		failedState := &StepState{
			StepID:     step.ID(),
			Status:     StatusFailed,
			FinishedAt: time.Now().UTC(),
			Attempts:   1,
			ErrMsg:     fmt.Sprintf("resolve credentials: %v", err),
		}
		_ = e.state.SaveStepState(ctx, runID, failedState)
		return nil, fmt.Errorf("resolve credentials for step %q: %w", step.ID(), err)
	}

	return NewExecutionContext(ExecutionContextOptions{
		Config:    e.cfg,
		Runner:    e.runner,
		AWSConfig: awsCfg,
		Log:       e.log,
		Project:   e.project,
		Env:       e.env,
		DryRun:    e.dryRun,
	}), nil
}

func (e *Engine) shouldSkipBecauseDone(
	ctx context.Context,
	runID string,
	step Step,
	execCtx *ExecutionContext,
) bool {
	done, err := step.IsDone(ctx, execCtx)
	if err != nil {
		e.log.Warn("IsDone check failed, proceeding with execution",
			logKeyStepID, step.ID(), "error", err)
	}
	if !done {
		return false
	}

	skippedState := &StepState{
		StepID:     step.ID(),
		Status:     StatusSkipped,
		FinishedAt: time.Now().UTC(),
	}
	_ = e.state.SaveStepState(ctx, runID, skippedState)
	e.log.Info("step already done (IsDone=true)", logKeyStepID, step.ID())
	e.progress("muted", "SKIPPED", fmt.Sprintf("%s already satisfied", step.ID()))
	return true
}

func (e *Engine) transitionToRunning(ctx context.Context, runID string, step Step) (*StepState, error) {
	runningState := &StepState{
		StepID:    step.ID(),
		Status:    StatusRunning,
		StartedAt: time.Now().UTC(),
		Attempts:  0,
	}
	if err := e.state.SaveStepState(ctx, runID, runningState); err != nil {
		return nil, fmt.Errorf("persist RUNNING state for %q: %w", step.ID(), err)
	}
	return runningState, nil
}

func (e *Engine) handleStepSuccess(
	ctx context.Context,
	runID string,
	step Step,
	execCtx *ExecutionContext,
	finalState *StepState,
	started time.Time,
) {
	e.persistStepOutputs(ctx, step, execCtx)

	finalState.Status = StatusSucceeded
	finalState.FinishedAt = time.Now().UTC()
	_ = e.state.SaveStepState(ctx, runID, finalState)

	e.log.Info("step succeeded",
		logKeyStepID, step.ID(),
		"attempts", finalState.Attempts,
	)
	e.progress("ok", "ok", fmt.Sprintf("%s in %s", step.ID(), e.duration(time.Since(started))))
}

func (e *Engine) persistStepOutputs(ctx context.Context, step Step, execCtx *ExecutionContext) {
	for key, value := range execCtx.Outputs() {
		storageKey := platformcfg.StepOutputKey(step.ID(), key)
		if err := e.cfg.Set(ctx, e.project, e.env, storageKey, value); err != nil {
			e.log.Warn("failed to persist step output",
				logKeyStepID, step.ID(),
				"key", key,
				"error", err,
			)
		}
	}
}

func (e *Engine) handleStepFailure(
	ctx context.Context,
	runID string,
	step Step,
	finalState *StepState,
	runErr error,
	started time.Time,
) error {
	finalState.Status = StatusFailed
	finalState.ErrMsg = runErr.Error()
	finalState.FinishedAt = time.Now().UTC()
	_ = e.state.SaveStepState(ctx, runID, finalState)

	meta := &RunMeta{
		RunID:   runID,
		Project: e.project,
		Env:     e.env,
		Status:  StatusFailed,
	}
	_ = e.state.SaveRunMeta(ctx, meta)

	e.log.Error("step failed",
		logKeyStepID, step.ID(),
		"error", finalState.ErrMsg,
		"attempts", finalState.Attempts,
	)
	e.progress("error", "fail", fmt.Sprintf("%s after %s: %s", step.ID(), e.duration(time.Since(started)), finalState.ErrMsg))
	return fmt.Errorf("pipeline halted at step %q: %s", step.ID(), finalState.ErrMsg)
}

// runWithRetry runs a step respecting its RetryPolicy.
func (e *Engine) runWithRetry(ctx context.Context, step Step, execCtx *ExecutionContext, state *StepState) (*StepState, error) {
	policy := step.RetryPolicy()
	maxAttempts := policy.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		state.Attempts = attempt
		lastErr = step.Run(ctx, execCtx)
		if lastErr == nil {
			return state, nil
		}
		state.ErrMsg = lastErr.Error()
		e.log.Warn("step attempt failed",
			logKeyStepID, step.ID(),
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"error", lastErr,
		)
		e.progress("warn", "RETRY", fmt.Sprintf("%s attempt %d/%d failed: %v", step.ID(), attempt, maxAttempts, lastErr))
		if attempt < maxAttempts {
			wait := backoffDuration(policy.Backoff, attempt)
			e.log.Info("retrying step", logKeyStepID, step.ID(), "wait", wait)
			e.progress("running", "WAIT", fmt.Sprintf("%s retrying in %s", step.ID(), e.duration(wait)))
			select {
			case <-ctx.Done():
				return state, ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	state.Status = StatusFailed
	state.FinishedAt = time.Now().UTC()
	return state, lastErr
}

func backoffDuration(strategy BackoffStrategy, attempt int) time.Duration {
	switch strategy {
	case BackoffExponential:
		secs := math.Pow(2, float64(attempt-1))
		if secs > 60 {
			secs = 60
		}
		return time.Duration(secs) * time.Second
	default: // BackoffFixed
		return 5 * time.Second
	}
}

func (e *Engine) stepStateFor(id string, prior map[string]*StepState) *StepState {
	if prior == nil {
		return nil
	}
	return prior[id]
}

func (e *Engine) progress(kind, label, detail string) {
	if e.reporter == nil {
		return
	}
	e.reporter.Report(kind, label, detail)
}

func (e *Engine) duration(d time.Duration) string {
	if e.ui != nil {
		return e.ui.Duration(d)
	}
	return d.Round(100 * time.Millisecond).String()
}
