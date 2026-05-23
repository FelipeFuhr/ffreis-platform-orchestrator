package runner

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestExecRunner_ContextCancellationKillsSubprocess is the contract test for
// the ctx plumbing added to Runner.Exec. Before the change, Exec internally
// used context.Background, so a SIGTERM at the orchestrator did not kill any
// in-flight terraform/etc. subprocess. After the change, cancelling the ctx
// passed to Exec causes the subprocess to be killed and Exec to return.
//
// We use the same TestHelperProcess pattern as exec_test.go: re-invoke this
// test binary with a sentinel env var, sleep long enough that the parent
// must cancel us, and assert the parent's Exec returns within a reasonable
// bound.
func TestExecRunnerContextCancellationKillsSubprocess(t *testing.T) {
	if os.Getenv("EXEC_CANCEL_HELPER") == "1" {
		// Child path: sleep way longer than the test timeout. If our parent
		// fails to kill us via ctx cancel, the test will time out instead.
		time.Sleep(30 * time.Second)
		return
	}

	r := NewExecRunner()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel a moment after the subprocess is spawned. 100ms is enough for
	// exec.CommandContext to fork and start the child sleep.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := r.Exec(ctx, os.Args[0],
		[]string{"-test.run=TestExecRunnerContextCancellationKillsSubprocess", "-test.timeout=15s"},
		ExecOptions{
			Env: map[string]string{
				"EXEC_CANCEL_HELPER": "1",
				// PATH/HOME forwarded by baseline; the go test binary needs
				// them to locate /tmp and friends.
			},
		})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Exec returned nil error; expected cancellation to surface (elapsed=%s)", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Exec did not honor ctx cancellation (elapsed=%s, want < 5s)", elapsed)
	}
	// The error chain typically wraps "signal: killed" or a context error;
	// either is acceptable. The hard contract is "did not run 30s".
	t.Logf("Exec returned after %s with: %v", elapsed, err)

	// Belt-and-braces: confirm ctx is in fact done (not just a coincidence
	// of the helper exiting).
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
}
