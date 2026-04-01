package runner

import (
	"os"
	"strings"
	"testing"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	dashIdx := -1
	for i, a := range args {
		if a == "--" {
			dashIdx = i
			break
		}
	}
	if dashIdx == -1 || dashIdx+1 >= len(args) {
		os.Exit(2)
	}

	mode := args[dashIdx+1]
	switch mode {
	case "ok":
		_, _ = os.Stdout.WriteString("hello\n")
		_, _ = os.Stderr.WriteString("warn\n")
		os.Exit(0)
	case "exit2":
		_, _ = os.Stderr.WriteString("boom\n")
		os.Exit(2)
	default:
		os.Exit(3)
	}
}

func TestExecRunner_Success(t *testing.T) {
	r := NewExecRunner()
	res, err := r.Exec(os.Args[0], []string{"-test.run=TestHelperProcess", "--", "ok"}, ExecOptions{
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
		},
	})
	if err != nil {
		t.Fatalf("Exec() err = %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Fatalf("Stdout = %q, want to contain %q", res.Stdout, "hello")
	}
	if !strings.Contains(res.Stderr, "warn") {
		t.Fatalf("Stderr = %q, want to contain %q", res.Stderr, "warn")
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
}

func TestExecRunner_NonZeroExitIsError(t *testing.T) {
	r := NewExecRunner()
	res, err := r.Exec(os.Args[0], []string{"-test.run=TestHelperProcess", "--", "exit2"}, ExecOptions{
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.ExitCode != 2 {
		t.Fatalf("ExitCode = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}
