package pipeline

import "testing"

func TestRollbackNotSupportedHelpers(t *testing.T) {
	err := ErrRollbackNotSupported("step-a")
	if err == nil || err.Error() != "rollback not supported for step: step-a" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsRollbackNotSupported(err) {
		t.Fatal("expected IsRollbackNotSupported to be true")
	}
	if IsRollbackNotSupported(nil) {
		t.Fatal("expected nil to return false")
	}
	if IsRollbackNotSupported(assertErr("other")) {
		t.Fatal("expected unrelated error to return false")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
