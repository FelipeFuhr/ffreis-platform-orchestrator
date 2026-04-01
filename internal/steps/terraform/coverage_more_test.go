package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	if err := dirExists(dir); err != nil {
		t.Fatalf("dirExists(dir) error: %v", err)
	}

	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if err := dirExists(file); err == nil {
		t.Fatal("expected not-a-directory error")
	}
	if err := dirExists(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing directory error")
	}
}

func TestTerraformStepMetadata(t *testing.T) {
	initStep := NewInitStep()
	if initStep.ID() == "" || initStep.Name() == "" || initStep.CredentialClass() != credential.ClassAdmin {
		t.Fatal("unexpected init step metadata")
	}
	if len(initStep.Deps()) != 1 || len(initStep.RequiredInputs()) != 4 || initStep.RetryPolicy() != pipeline.RetryThrice {
		t.Fatal("unexpected init step configuration")
	}

	planStep := NewPlanStep()
	if planStep.ID() == "" || planStep.Name() == "" || planStep.CredentialClass() != credential.ClassAdmin {
		t.Fatal("unexpected plan step metadata")
	}
	if len(planStep.Deps()) != 1 || len(planStep.RequiredInputs()) != 0 || planStep.RetryPolicy() != pipeline.RetryThrice {
		t.Fatal("unexpected plan step configuration")
	}

	applyStep := NewApplyStep()
	if applyStep.ID() == "" || applyStep.Name() == "" || applyStep.CredentialClass() != credential.ClassAdmin {
		t.Fatal("unexpected apply step metadata")
	}
	if len(applyStep.Deps()) != 1 || len(applyStep.RequiredInputs()) != 0 || applyStep.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected apply step configuration")
	}
}

func TestInitStep_IsDoneAndRunSuccess(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	stateFile := filepath.Join(modulePath, ".terraform", "terraform.tfstate")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(stateFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)
	_ = cfg.Set(context.Background(), "platform", "dev", "terraform/backend_bucket", "bucket")
	_ = cfg.Set(context.Background(), "platform", "dev", "terraform/backend_region", "us-east-1")

	step := NewInitStep()
	execCtx := newExecCtx(t, cfg, &recordingRunner{}, false)
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}

	r := &recordingRunner{}
	execCtx = newExecCtx(t, cfg, r, false)
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if r.lastCmd != "terraform" || r.lastOpts.WorkDir != modulePath {
		t.Fatalf("unexpected runner call: %+v", r)
	}
	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}

func TestInitStep_RunError(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)
	_ = cfg.Set(context.Background(), "platform", "dev", "terraform/backend_bucket", "bucket")
	_ = cfg.Set(context.Background(), "platform", "dev", "terraform/backend_region", "us-east-1")

	r := &recordingRunner{result: runnerResult("stderr"), err: errors.New("boom")}
	execCtx := newExecCtx(t, cfg, r, false)
	if err := NewInitStep().Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected terraform init error")
	}
}

func TestPlanStep_MetadataBranches(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)
	_ = cfg.Set(context.Background(), "platform", "dev", "orchestrator/step/terraform_plan_org/output/plan_has_changes", "false")
	if err := os.WriteFile(filepath.Join(modulePath, "org.tfplan"), []byte("plan"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	step := NewPlanStep()
	execCtx := newExecCtx(t, cfg, &recordingRunner{}, false)
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}

	execCtx = newExecCtx(t, cfg, &recordingRunner{}, true)
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("dry-run Run() error: %v", err)
	}
	if got := execCtx.Outputs()["plan_has_changes"]; got != "false" {
		t.Fatalf("plan_has_changes = %q", got)
	}
}

func TestApplyStep_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)
	_ = cfg.Set(context.Background(), "platform", "dev", "orchestrator/step/terraform_plan_org/output/plan_has_changes", "true")
	_ = cfg.Set(context.Background(), "platform", "dev", "orchestrator/step/terraform_apply_org/output/applied", "true")

	step := NewApplyStep()
	execCtx := newExecCtx(t, cfg, &recordingRunner{}, false)
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}

	r := &recordingRunner{}
	execCtx = newExecCtx(t, cfg, r, false)
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if got := execCtx.Outputs()["applied"]; got != "true" {
		t.Fatalf("applied output = %q", got)
	}

	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}

func TestApplyStep_RollbackMissingModule(t *testing.T) {
	if err := NewApplyStep().Rollback(context.Background(), newExecCtx(t, newMemConfig(), &recordingRunner{}, false)); err == nil {
		t.Fatal("expected rollback error")
	}
}

func TestApplyStep_RunError(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)
	_ = cfg.Set(context.Background(), "platform", "dev", "orchestrator/step/terraform_plan_org/output/plan_has_changes", "true")

	r := &recordingRunner{result: runnerResult("stderr"), err: errors.New("apply failed")}
	execCtx := newExecCtx(t, cfg, r, false)
	if err := NewApplyStep().Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected apply error")
	}
}

func runnerResult(stderr string) runner.ExecResult {
	return runner.ExecResult{Stderr: stderr}
}
