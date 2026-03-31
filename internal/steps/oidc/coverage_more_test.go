package oidc

import (
	"context"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

func TestOIDCStepMetadata(t *testing.T) {
	provider := NewCreateProviderStep()
	if len(provider.Deps()) != 1 || provider.CredentialClass() != credential.ClassAdmin || len(provider.RequiredInputs()) != 0 || provider.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected provider metadata")
	}

	role := NewCreateGitHubRoleStep()
	if len(role.Deps()) != 1 || role.CredentialClass() != credential.ClassAdmin || len(role.RequiredInputs()) != 3 || role.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected role metadata")
	}

	write := NewWriteOutputsStep()
	if len(write.Deps()) != 1 || write.CredentialClass() != credential.ClassOperator || len(write.RequiredInputs()) != 0 || write.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected write outputs metadata")
	}
}

func TestWriteOutputsStep_MetadataBranches(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	step := NewWriteOutputsStep()

	if step.ID() == "" || step.Name() == "" {
		t.Fatal("expected write outputs identifiers")
	}

	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
	if err := step.Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected missing config error")
	}

	_ = cfg.Set(context.Background(), platformProject, globalEnv, githubRoleARNKey, "role-arn")
	_ = cfg.Set(context.Background(), platformProject, globalEnv, oidcProviderKey, "provider-arn")
	_ = cfg.Set(context.Background(), execCtx.Project(), execCtx.Env(), outputKeyGitHubRoleARN, "role-arn")
	done, err = step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() after config = %v, %v", done, err)
	}
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() success error: %v", err)
	}
}
