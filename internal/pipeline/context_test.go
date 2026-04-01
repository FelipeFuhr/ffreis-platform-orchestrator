package pipeline

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/zap"

	"github.com/ffreis/platform-orchestrator/internal/runner"
)

type fakeRunner struct{}

func (fakeRunner) Exec(string, []string, runner.ExecOptions) (runner.ExecResult, error) {
	return runner.ExecResult{}, nil
}

func TestExecutionContextAccessorsAndOutputs(t *testing.T) {
	cfg := newMemConfigStore()
	log := zap.NewNop()
	awsCfg := aws.Config{Region: "us-east-1"}

	ctx := NewExecutionContext(ExecutionContextOptions{
		Config:    cfg,
		Runner:    fakeRunner{},
		AWSConfig: awsCfg,
		Log:       log,
		Project:   "platform",
		Env:       "dev",
		DryRun:    true,
	})

	if ctx.Config() != cfg || ctx.Runner() == nil || ctx.AWSConfig().Region != "us-east-1" {
		t.Fatal("unexpected accessor values")
	}
	if ctx.Log() != log || ctx.Project() != "platform" || ctx.Env() != "dev" || !ctx.DryRun() {
		t.Fatal("unexpected execution context metadata")
	}

	ctx.SetOutput("k", "v")
	got := ctx.Outputs()
	if got["k"] != "v" {
		t.Fatalf("expected output to be stored, got %#v", got)
	}
	got["k"] = "mutated"
	if ctx.Outputs()["k"] != "v" {
		t.Fatal("Outputs() should return a copy")
	}
}
