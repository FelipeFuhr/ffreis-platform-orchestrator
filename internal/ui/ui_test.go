package ui

import (
	"strings"
	"testing"
	"time"
)

func TestResolveMode(t *testing.T) {
	t.Parallel()

	mode, interactive, err := ResolveMode("auto", true, false, false)
	if err != nil {
		t.Fatalf("ResolveMode() error: %v", err)
	}
	if mode != ModeRich || !interactive {
		t.Fatalf("ResolveMode() = (%q, %v), want (%q, true)", mode, interactive, ModeRich)
	}

	mode, interactive, err = ResolveMode("auto", false, false, false)
	if err != nil {
		t.Fatalf("ResolveMode() error: %v", err)
	}
	if mode != ModePlain || interactive {
		t.Fatalf("ResolveMode() = (%q, %v), want (%q, false)", mode, interactive, ModePlain)
	}
}

func TestResolveMode_Invalid(t *testing.T) {
	t.Parallel()

	if _, _, err := ResolveMode("broken", true, true, false); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestPresenterPlainFormatting(t *testing.T) {
	t.Parallel()

	p := &Presenter{mode: ModePlain}
	if got := p.Badge("ok", "OK"); got != "[ok]" {
		t.Fatalf("Badge() = %q", got)
	}
	if got := p.Duration(1250 * time.Millisecond); got != "1.3s" {
		t.Fatalf("Duration() = %q", got)
	}
	if got := p.Status("warn", "STEP", "collecting inputs"); !strings.Contains(got, "[step]") {
		t.Fatalf("Status() = %q", got)
	}
}
