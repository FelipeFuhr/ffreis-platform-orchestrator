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

// dirExists validates that a directory path exists on disk.
func dirExists(v string) error {
	info, err := os.Stat(v)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", v)
		}
		return fmt.Errorf("stat %s: %w", v, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory: %s", v)
	}
	return nil
}

// InitStep runs `terraform init` for the org module.
type InitStep struct{}

func NewInitStep() *InitStep { return &InitStep{} }

func (s *InitStep) ID() string   { return "terraform_init_org" }
func (s *InitStep) Name() string { return "Terraform init (org module)" }
func (s *InitStep) Deps() []string {
	return []string{"create_admin_role"}
}
func (s *InitStep) CredentialClass() credential.Class { return credential.ClassAdmin }
func (s *InitStep) RequiredInputs() []prompt.InputSpec {
	return []prompt.InputSpec{
		{
			Key:      configKeyOrgModulePath,
			Label:    "Path to org Terraform module",
			Validate: dirExists,
		},
		{Key: "terraform/backend_bucket", Label: "S3 backend bucket name"},
		{Key: "terraform/backend_key", Label: "S3 backend key", Default: "org/terraform.tfstate"},
		{Key: "terraform/backend_region", Label: "S3 backend region"},
	}
}
func (s *InitStep) RetryPolicy() pipeline.RetryPolicy { return pipeline.RetryThrice }

func (s *InitStep) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return false, nil
	}
	_, statErr := os.Stat(modulePath + "/.terraform/terraform.tfstate")
	return statErr == nil, nil
}

func (s *InitStep) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	modulePath, err := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), configKeyOrgModulePath)
	if err != nil || modulePath == "" {
		return fmt.Errorf("input %q is required", configKeyOrgModulePath)
	}
	backendBucket, _ := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), "terraform/backend_bucket")
	backendKey, _ := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), "terraform/backend_key")
	if backendKey == "" {
		backendKey = "org/terraform.tfstate"
	}
	backendRegion, _ := execCtx.Config().Get(ctx, execCtx.Project(), execCtx.Env(), "terraform/backend_region")

	env, err := awsEnvFromConfig(ctx, execCtx.AWSConfig())
	if err != nil {
		return err
	}

	if execCtx.DryRun() {
		execCtx.Log().Info("[dry-run] would run: terraform init in " + modulePath)
		return nil
	}

	_, err = execCtx.Runner().Exec("terraform", []string{
		"init",
		"-backend-config=bucket=" + backendBucket,
		"-backend-config=key=" + backendKey,
		"-backend-config=region=" + backendRegion,
		"-input=false",
	}, runner.ExecOptions{
		WorkDir: modulePath,
		Env:     env,
	})
	if err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}
	return nil
}

func (s *InitStep) Rollback(_ context.Context, _ *pipeline.ExecutionContext) error {
	// init is idempotent; removing .terraform is safe but unnecessary.
	return nil
}
