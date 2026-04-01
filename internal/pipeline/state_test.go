package pipeline

import (
	"context"
	"testing"
	"time"
)

func TestStateStore_SaveAndLoad(t *testing.T) {
	store := NewStateStore(newMemConfigStore())
	meta := &RunMeta{RunID: "run-1", Project: "platform", Env: "dev", StartedAt: time.Now(), Status: StatusRunning}

	if err := store.SaveRunMeta(context.Background(), meta); err != nil {
		t.Fatalf("SaveRunMeta() error: %v", err)
	}
	if got, err := store.LastRunID(context.Background()); err != nil || got != "run-1" {
		t.Fatalf("LastRunID() = %q, %v", got, err)
	}
	if got, err := store.LoadRunMeta(context.Background(), "run-1"); err != nil || got.RunID != "run-1" {
		t.Fatalf("LoadRunMeta() = %+v, %v", got, err)
	}

	step := &StepState{StepID: "s1", Status: StatusSucceeded, Attempts: 1, Outputs: map[string]string{"k": "v"}}
	if err := store.SaveStepState(context.Background(), "run-1", step); err != nil {
		t.Fatalf("SaveStepState() error: %v", err)
	}
	if got, err := store.LoadStepState(context.Background(), "run-1", "s1"); err != nil || got.StepID != "s1" {
		t.Fatalf("LoadStepState() = %+v, %v", got, err)
	}

	all, err := store.LoadAllStepStates(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("LoadAllStepStates() error: %v", err)
	}
	if len(all) != 1 || all["s1"] == nil {
		t.Fatalf("unexpected states: %+v", all)
	}
}

func TestStateStore_LoadStepStateMissing(t *testing.T) {
	store := NewStateStore(newMemConfigStore())
	got, err := store.LoadStepState(context.Background(), "run-1", "missing")
	if err != nil || got != nil {
		t.Fatalf("LoadStepState() = %+v, %v", got, err)
	}
}
