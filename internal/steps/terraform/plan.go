package terraform

import (
	"context"
	"fmt"
	"os"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

// PlanStep runs `terraform plan` and records whether changes exist.
type PlanStep struct{}

func NewPlanStep() *PlanStep { return &PlanStep{} }

func (s *PlanStep) ID() string   { return "terraform_plan_org" }
func (s *PlanStep) Name() string { return "Terraform plan (org module)" }
func (s *PlanStep) Deps() []string {
	return []string{"terraform_init_org"}
}
func (s *PlanStep) CredentialClass() credential.Class  { return credential.ClassAdmin }
func (s *PlanStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *PlanStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.RetryThrice }

func (s *PlanStep) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return false, nil
	}
	// Check plan file exists.
	if _, statErr := os.Stat(modulePath + "/org.tfplan"); statErr != nil {
		return false, nil
	}
	// Check stored output.
	v, getErr := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(),
		"orchestrator/step/terraform_plan_org/output/plan_has_changes")
	return getErr == nil && v == "false", nil
}

func (s *PlanStep) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return fmt.Errorf("input %q is required", configKeyOrgModulePath)
	}

	env, err := awsEnvFromConfig(ctx, execCtx.AWSConfig())
	if err != nil {
		return err
	}

	if execCtx.DryRun() {
		execCtx.Log().Info("[dry-run] would run: terraform plan in " + modulePath)
		execCtx.SetOutput("plan_has_changes", "false")
		return nil
	}

	result, err := execCtx.Runner().Exec(ctx, "terraform", []string{
		"plan", "-out=org.tfplan", "-detailed-exitcode", "-input=false",
	}, runner.ExecOptions{WorkDir: modulePath, Env: env})

	hasChanges := "false"
	if err != nil {
		// Exit code 2 means changes are present — not an error.
		if result.ExitCode == 2 {
			hasChanges = "true"
		} else {
			return fmt.Errorf("terraform plan: %w", err)
		}
	}

	execCtx.SetOutput("plan_has_changes", hasChanges)
	return nil
}

func (s *PlanStep) Rollback(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return nil
	}
	planFile := modulePath + "/org.tfplan"
	if _, statErr := os.Stat(planFile); statErr == nil {
		_ = os.Remove(planFile)
	}
	return nil
}
