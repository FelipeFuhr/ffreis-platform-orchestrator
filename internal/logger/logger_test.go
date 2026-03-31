package logger

import "testing"

func TestNew_InvalidLevel(t *testing.T) {
	_, err := New("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNop(t *testing.T) {
	log := Nop()
	log.Info("ok")
}
