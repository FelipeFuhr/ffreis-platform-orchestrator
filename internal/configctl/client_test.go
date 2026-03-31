package configctl

import "testing"

func TestIsNotFound(t *testing.T) {
	err := &ErrNotFoundError{Key: "x"}
	if err.Error() != "key not found: x" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
	if !IsNotFound(err) {
		t.Fatal("expected IsNotFound to be true")
	}
	if IsNotFound(nil) {
		t.Fatal("expected nil to be false")
	}
	if IsNotFound(assertErr("other")) {
		t.Fatal("expected unrelated error to be false")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
