package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/logger"
)

type errResolver struct{ err error }

func (r errResolver) Resolve(context.Context, credential.Class) (aws.Config, error) {
	return aws.Config{}, r.err
}

func TestBackoffDuration(t *testing.T) {
	if got := backoffDuration(BackoffFixed, 3); got != 5*time.Second {
		t.Fatalf("fixed backoff = %v", got)
	}
	if got := backoffDuration(BackoffExponential, 3); got != 4*time.Second {
		t.Fatalf("exponential backoff = %v", got)
	}
	if got := backoffDuration(BackoffExponential, 10); got != 60*time.Second {
		t.Fatalf("capped backoff = %v", got)
	}
}

func TestEngineResumeRunningStateAndResolveError(t *testing.T) {
	store := newMemConfigStore()
	state := NewStateStore(store)
	if err := state.SaveStepState(context.Background(), "run-1", &StepState{StepID: "a", Status: StatusRunning}); err != nil {
		t.Fatalf("SaveStepState(): %v", err)
	}
	eng := NewEngine(EngineOptions{
		DAG:      NewDAG(),
		State:    state,
		Resolver: newClassTrackingResolver(),
		Config:   store,
		Runner:   &mockRunner{},
		Log:      logger.Nop(),
		Project:  testProject,
		Env:      testEnv,
	})
	if err := eng.Resume(context.Background(), "run-1"); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	got, err := state.LoadStepState(context.Background(), "run-1", "a")
	if err != nil || got.Status != StatusFailed {
		t.Fatalf("state after resume = %+v, %v", got, err)
	}

	step := &stubStep{id: "b", class: credential.ClassAdmin}
	dag := NewDAG()
	_ = dag.Add(step)
	eng = NewEngine(EngineOptions{
		DAG:      dag,
		State:    state,
		Resolver: errResolver{err: errors.New("resolve")},
		Config:   store,
		Runner:   &mockRunner{},
		Log:      logger.Nop(),
		Project:  testProject,
		Env:      testEnv,
	})
	if err := eng.Init(context.Background(), "run-2"); err == nil {
		t.Fatal("expected resolve error")
	}
}

func TestStateStore_ErrorBranches(t *testing.T) {
	store := NewStateStore(newMemConfigStore())
	if _, err := store.LoadRunMeta(context.Background(), "missing"); err == nil {
		t.Fatal("expected missing run meta error")
	}
}
