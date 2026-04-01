package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/postrun"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/pipelines"
)

// runFlags holds flags specific to the init command.
type runFlags struct {
	run               bool
	runnerBinary      string
	runnerConfig      string
	runnerRulesDir    string
	runnerTemplateDir string
	runnerWorkspace   string
	runnerToken       string
}

var (
	buildPlatformSetupPipeline = pipelines.PlatformSetupPipeline
	newPostrunInvoker          = postrun.New
	newBatchPrompter           = prompt.NewBatchPrompter
	newInteractivePrompter     = prompt.NewInteractivePrompter
	newCollector               = prompt.NewCollector
)

func newInitCmd(d *deps, gf *globalFlags) *cobra.Command {
	rf := &runFlags{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Start a new platform provisioning run",
		Long: `init collects all required inputs interactively, stores them in DynamoDB,
then executes the full platform pipeline in dependency order.

On failure the run ID is preserved; use 'resume' to continue.

With --run, platform-runner is invoked after the pipeline completes to:
  1. validate    — run guardian checks across all repos
  2. sync-template — apply template updates (safe files only)
  3. plan-all    — produce a Terraform plan preview (never applies)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireProjectEnv(gf); err != nil {
				return err
			}
			return runInit(cmd, d, gf, rf)
		},
	}

	// --run and related flags: no import coupling to platform-runner package.
	cmd.Flags().BoolVar(&rf.run, "run", false,
		"Invoke platform-runner after the pipeline (validate → sync-template → plan-all)")
	cmd.Flags().StringVar(&rf.runnerBinary, "runner-binary", "platform-runner",
		"Path to the platform-runner binary")
	cmd.Flags().StringVar(&rf.runnerConfig, "runner-config", "",
		"Config path (YAML file or DynamoDB table) passed to platform-runner")
	cmd.Flags().StringVar(&rf.runnerRulesDir, "runner-rules-dir", "",
		"Rules directory passed to platform-runner validate")
	cmd.Flags().StringVar(&rf.runnerTemplateDir, "runner-template-dir", "",
		"Template directory passed to platform-runner sync-template")
	cmd.Flags().StringVar(&rf.runnerWorkspace, "runner-workspace", "./workspace",
		"Workspace directory passed to platform-runner")
	cmd.Flags().StringVar(&rf.runnerToken, "runner-token", "",
		"GitHub token for platform-runner (falls back to GITHUB_TOKEN env)")

	return cmd
}

func runInit(cmd *cobra.Command, d *deps, gf *globalFlags, rf *runFlags) error {
	ctx := cmd.Context()
	runID := newRunID()
	d.log.Info("starting new run",
		zap.String("run_id", runID),
		zap.String("project", gf.project),
		zap.String("env", gf.env),
	)

	dag, err := buildPlatformSetupPipeline(d.cfgctl)
	if err != nil {
		return fmt.Errorf("build pipeline: %w", err)
	}

	// Collection phase.
	if err := collectInputs(ctx, d, gf, dag); err != nil {
		return fmt.Errorf("input collection: %w", err)
	}

	// Execute pipeline.
	eng := buildEngine(ctx, d, gf, dag, runID)
	if err := eng.Init(ctx, runID); err != nil {
		return err
	}

	// Post-pipeline: invoke platform-runner if --run is set.
	if rf.run {
		if err := invokeRunner(ctx, d, gf, rf); err != nil {
			return fmt.Errorf("platform-runner: %w", err)
		}
	}
	return nil
}

// invokeRunner calls platform-runner subcommands in sequence via subprocess.
// There is no package-level coupling to platform-runner.
func invokeRunner(ctx context.Context, d *deps, gf *globalFlags, rf *runFlags) error {
	token := rf.runnerToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	inv := newPostrunInvoker(postrun.Options{
		Binary:      rf.runnerBinary,
		Config:      rf.runnerConfig,
		RulesDir:    rf.runnerRulesDir,
		TemplateDir: rf.runnerTemplateDir,
		Workspace:   rf.runnerWorkspace,
		Token:       token,
		DryRun:      gf.dryRun,
	})

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"validate", inv.Validate},
		{"sync-template", inv.SyncTemplate},
		{"plan-all", inv.PlanAll},
	}

	for _, step := range steps {
		d.log.Info("invoking platform-runner", zap.String("subcommand", step.name))
		if err := step.fn(ctx); err != nil {
			d.log.Error("platform-runner step failed",
				zap.String("subcommand", step.name),
				zap.Error(err),
			)
			return fmt.Errorf("step %q: %w", step.name, err)
		}
		d.log.Info("platform-runner step complete", zap.String("subcommand", step.name))
	}
	return nil
}

func collectInputs(ctx context.Context, d *deps, gf *globalFlags, dag *pipeline.DAG) error {
	sorted, err := dag.TopoSort()
	if err != nil {
		return err
	}
	var specs []prompt.InputSpec
	for _, step := range sorted {
		specs = append(specs, step.RequiredInputs()...)
	}

	var pr prompt.Prompter
	if d.cfg.NonInteractive {
		pr = newBatchPrompter(d.cfgctl, gf.project, gf.env, func(f string, a ...any) {
			d.log.Info(fmt.Sprintf(f, a...))
		})
	} else {
		pr = newInteractivePrompter()
	}

	collector := newCollector(d.cfgctl, pr, gf.project, gf.env)
	result, err := collector.Collect(ctx, specs)
	if err != nil {
		return err
	}
	d.log.Info("inputs collected",
		zap.Int("collected", len(result.Collected)),
		zap.Int("skipped", len(result.Skipped)),
	)
	return nil
}
