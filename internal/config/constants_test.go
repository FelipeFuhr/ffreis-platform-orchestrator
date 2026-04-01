package config

import "testing"

func TestKeyBuilders(t *testing.T) {
	if got, want := RunPrefix("abc"), "orchestrator/run/abc"; got != want {
		t.Fatalf("RunPrefix() = %q, want %q", got, want)
	}
	if got, want := RunMetaKey("abc"), "orchestrator/run/abc/meta"; got != want {
		t.Fatalf("RunMetaKey() = %q, want %q", got, want)
	}
	if got, want := StepStateKey("abc", "step-1"), "orchestrator/run/abc/step/step-1/state"; got != want {
		t.Fatalf("StepStateKey() = %q, want %q", got, want)
	}
	if got, want := StepOutputKey("step-1", "out"), "orchestrator/step/step-1/output/out"; got != want {
		t.Fatalf("StepOutputKey() = %q, want %q", got, want)
	}
}
