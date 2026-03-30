package terraform

import (
	"context"
	"fmt"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

// ApplyStep runs `terraform apply`.
type ApplyStep struct{}

func NewApplyStep() *ApplyStep { return &ApplyStep{} }

func (s *ApplyStep) ID() string   { return "terraform_apply_org" }
func (s *ApplyStep) Name() string { return "Terraform apply (org module)" }
func (s *ApplyStep) Deps() []string {
	return []string{"terraform_plan_org"}
}
func (s *ApplyStep) CredentialClass() credential.Class  { return credential.ClassAdmin }
func (s *ApplyStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *ApplyStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *ApplyStep) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	v, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(),
		"orchestrator/step/terraform_apply_org/output/applied")
	return err == nil && v == "true", nil
}

func (s *ApplyStep) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return fmt.Errorf("input %q is required", configKeyOrgModulePath)
	}

	// Read plan_has_changes from configctl.
	planHasChanges, _ := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(),
		"orchestrator/step/terraform_plan_org/output/plan_has_changes")
	if planHasChanges == "false" {
		execCtx.Log().Info("no changes, skipping apply")
		execCtx.SetOutput("applied", "false")
		return nil
	}

	env, err := awsEnvFromConfig(ctx, execCtx.AWSConfig())
	if err != nil {
		return err
	}

	if execCtx.DryRun() {
		execCtx.Log().Info("[dry-run] would run: terraform apply in " + modulePath)
		execCtx.SetOutput("applied", "true")
		return nil
	}

	result, err := execCtx.Runner().Exec("terraform", []string{
		"apply", "-auto-approve", "-input=false", "org.tfplan",
	}, runner.ExecOptions{WorkDir: modulePath, Env: env})
	if err != nil {
		return fmt.Errorf("terraform apply: %w — stderr: %s", err, result.Stderr)
	}

	execCtx.SetOutput("applied", "true")
	return nil
}

func (s *ApplyStep) Rollback(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return fmt.Errorf("terraform destroy: module path not available")
	}

	env, err := awsEnvFromConfig(ctx, execCtx.AWSConfig())
	if err != nil {
		return err
	}

	result, err := execCtx.Runner().Exec("terraform", []string{
		"destroy", "-auto-approve", "-input=false",
	}, runner.ExecOptions{WorkDir: modulePath, Env: env})
	if err != nil {
		return fmt.Errorf("terraform destroy: %w — stderr: %s", err, result.Stderr)
	}
	return nil
}
