package logger

import "testing"

func TestNew_ValidLevel(t *testing.T) {
	log, err := New("info", true)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if log == nil {
		t.Fatal("expected logger")
	}
	log.Info("ok")
}

func TestNew_InvalidLevel(t *testing.T) {
	_, err := New("nope", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_JSONLogger(t *testing.T) {
	log, err := New("info", false)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if log == nil {
		t.Fatal("expected logger")
	}
	log.Info("ok")
}

func TestNop(t *testing.T) {
	log := Nop()
	log.Info("ok")
}
