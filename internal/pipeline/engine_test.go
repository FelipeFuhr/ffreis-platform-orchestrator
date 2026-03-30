package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/credential"
)

const (
	runFullID      = "run-full"
	runResumeID    = "run-resume"
	runBootstrapID = "run-bootstrap"
)

// TestFullFlow: 3-step pipeline, all succeed; all states are StatusSucceeded.
func TestFullFlow(t *testing.T) {
	stepA := &stubStep{id: "a", class: credential.ClassRoot}
	stepB := &stubStep{id: "b", deps: []string{"a"}, class: credential.ClassAdmin}
	stepC := &stubStep{id: "c", deps: []string{"b"}, class: credential.ClassAdmin}

	dag := NewDAG()
	for _, s := range []Step{stepA, stepB, stepC} {
		if err := dag.Add(s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	store := newMemConfigStore()
	eng := buildTestEngine(dag, store)

	if err := eng.Init(context.Background(), runFullID); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// All three steps should have run once.
	for _, s := range []*stubStep{stepA, stepB, stepC} {
		if s.runCnt != 1 {
			t.Errorf("step %q: expected 1 run, got %d", s.id, s.runCnt)
		}
	}

	// All states should be succeeded.
	state := NewStateStore(store)
	for _, id := range []string{"a", "b", "c"} {
		ss, err := state.LoadStepState(context.Background(), runFullID, id)
		if err != nil {
			t.Fatalf("LoadStepState %q: %v", id, err)
		}
		if ss == nil {
			t.Fatalf("step %q has no state", id)
		}
		if ss.Status != StatusSucceeded {
			t.Errorf("step %q: expected %s, got %s", id, StatusSucceeded, ss.Status)
		}
	}
}

// TestResumeAfterFailure: step 2 fails on first run. On resume, step 1 is skipped
// (SUCCEEDED in store), steps 2 and 3 are re-run.
func TestResumeAfterFailure(t *testing.T) {
	stepA := &stubStep{id: "a", class: credential.ClassRoot}
	stepB := &stubStep{id: "b", deps: []string{"a"}, class: credential.ClassAdmin, runErr: fmt.Errorf("transient")}
	stepC := &stubStep{id: "c", deps: []string{"b"}, class: credential.ClassAdmin}

	dag := NewDAG()
	for _, s := range []Step{stepA, stepB, stepC} {
		if err := dag.Add(s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	store := newMemConfigStore()
	eng := buildTestEngine(dag, store)

	// First run: a succeeds, b fails, c never runs.
	err := eng.Init(context.Background(), runResumeID)
	if err == nil {
		t.Fatal("expected Init to return error on step failure")
	}
	if !strings.Contains(err.Error(), "transient") {
		t.Errorf("expected error to contain step message, got: %v", err)
	}

	if stepA.runCnt != 1 {
		t.Errorf("step a: expected 1 run, got %d", stepA.runCnt)
	}
	if stepB.runCnt != 1 {
		t.Errorf("step b: expected 1 run (failed), got %d", stepB.runCnt)
	}
	if stepC.runCnt != 0 {
		t.Errorf("step c should not have run, got %d", stepC.runCnt)
	}

	// Fix step b and resume.
	stepB.runErr = nil
	stepA.runCnt = 0
	stepB.runCnt = 0
	stepC.runCnt = 0

	if err := eng.Resume(context.Background(), runResumeID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	// Step a must be skipped (its stored state is SUCCEEDED).
	if stepA.runCnt != 0 {
		t.Errorf("step a should be skipped on resume, got %d runs", stepA.runCnt)
	}
	if stepB.runCnt != 1 {
		t.Errorf("step b should run once on resume, got %d", stepB.runCnt)
	}
	if stepC.runCnt != 1 {
		t.Errorf("step c should run once on resume, got %d", stepC.runCnt)
	}
}

// TestNoRootAfterBootstrap verifies the resolver sees ClassAdmin for admin steps,
// never ClassRoot for those steps.
func TestNoRootAfterBootstrap(t *testing.T) {
	store := newMemConfigStore()

	// Step 1 (ClassRoot) writes the admin role ARN during Run.
	step1 := &stubWriteARNStep{
		id:    "bootstrap",
		class: credential.ClassRoot,
		store: store,
	}
	// Step 2 (ClassAdmin) depends on the ARN being present.
	step2 := &stubStep{id: "admin_work", deps: []string{"bootstrap"}, class: credential.ClassAdmin}

	dag := NewDAG()
	if err := dag.Add(step1); err != nil {
		t.Fatalf("Add step1: %v", err)
	}
	if err := dag.Add(step2); err != nil {
		t.Fatalf("Add step2: %v", err)
	}

	tracker := newClassTrackingResolver()
	eng := buildTestEngineWithResolver(dag, store, tracker)

	if err := eng.Init(context.Background(), runBootstrapID); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// step1 → ClassRoot, step2 → ClassAdmin
	if len(tracker.resolved) != 2 {
		t.Fatalf("expected 2 resolutions, got %d", len(tracker.resolved))
	}
	if tracker.resolved[0] != credential.ClassRoot {
		t.Errorf("step1 should use ClassRoot, got %v", tracker.resolved[0])
	}
	if tracker.resolved[1] != credential.ClassAdmin {
		t.Errorf("step2 should use ClassAdmin, got %v", tracker.resolved[1])
	}
}
