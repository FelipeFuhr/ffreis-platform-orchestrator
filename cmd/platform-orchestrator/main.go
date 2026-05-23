package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ffreis/platform-orchestrator/cmd"
)

// exit is overridable in tests so they can observe the requested status code
// without terminating the test process.
var exit = os.Exit

func main() {
	// Catch SIGINT / SIGTERM so an interrupted orchestration cleanly cancels
	// in-flight subprocess work (terraform plan/apply etc.) rather than being
	// killed mid-step. The context is propagated to Cobra via cmd.ExecuteContext
	// so commands can pick it up from cmd.Context().
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	exit(cmd.ExecuteContext(ctx))
}
