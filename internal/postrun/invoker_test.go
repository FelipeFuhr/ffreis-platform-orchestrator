package postrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInvoker_RedactsTokenOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script helper")
	}

	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fail.sh")
	token := "SECRET_TOKEN"

	script := "#!/bin/sh\n" +
		"echo \"token=" + token + "\" 1>&2\n" +
		"exit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	inv := New(Options{
		Binary: scriptPath,
		Token:  token,
	})

	err := inv.PlanAll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("expected token to be redacted from error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Fatalf("expected redaction marker in error, got: %v", err)
	}
}

func TestInvoker_BaseArgsAndDefaultBinary(t *testing.T) {
	inv := New(Options{
		Config:    "cfg.yaml",
		Workspace: "/tmp/ws",
		DryRun:    true,
	})
	if inv.opts.Binary != "platform-runner" {
		t.Fatalf("expected default binary, got %q", inv.opts.Binary)
	}

	args := inv.baseArgs("validate")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "validate") || !strings.Contains(got, "--config cfg.yaml") || !strings.Contains(got, "--workspace /tmp/ws") || !strings.Contains(got, "--dry-run") {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestInvoker_SubcommandsSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script helper")
	}

	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "ok.sh")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	inv := New(Options{
		Binary:      scriptPath,
		Config:      "cfg.yaml",
		RulesDir:    "/rules",
		TemplateDir: "/template",
		Workspace:   "/workspace",
		DryRun:      true,
	})

	for name, fn := range map[string]func(context.Context) error{
		"validate":      inv.Validate,
		"sync-template": inv.SyncTemplate,
		"plan-all":      inv.PlanAll,
	} {
		t.Run(name, func(t *testing.T) {
			if err := fn(context.Background()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
