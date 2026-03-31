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
