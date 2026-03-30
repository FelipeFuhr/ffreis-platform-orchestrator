package bootstrap

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

const (
	adminRolePolicy = "arn:aws:iam::aws:policy/AdministratorAccess"
	defaultRoleName = "platform-admin"
)

// CreateAdminRole creates the IAM admin role using root credentials.
// After this step succeeds, all subsequent steps use ClassAdmin.
type CreateAdminRole struct {
	cfg configctl.Client
}

// NewCreateAdminRole constructs a CreateAdminRole step.
func NewCreateAdminRole(cfg configctl.Client) *CreateAdminRole {
	return &CreateAdminRole{cfg: cfg}
}

func (s *CreateAdminRole) ID() string   { return "create_admin_role" }
func (s *CreateAdminRole) Name() string { return "Create IAM admin role" }
func (s *CreateAdminRole) Deps() []string {
	return []string{"create_org"}
}
func (s *CreateAdminRole) CredentialClass() credential.Class { return credential.ClassRoot }
func (s *CreateAdminRole) RequiredInputs() []prompt.InputSpec {
	return []prompt.InputSpec{
		{Key: platformcfg.KeyAdminRoleName, Label: "Admin IAM role name", Default: defaultRoleName},
	}
}
func (s *CreateAdminRole) RetryPolicy() pipeline.RetryPolicy { return pipeline.NoRetry }

func (s *CreateAdminRole) IsDone(ctx *pipeline.ExecutionContext) (bool, error) {
	roleName, err := ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleName)
	if err != nil || roleName == "" {
		roleName = defaultRoleName
	}
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	if _, err := iamClient.GetRole(ctx.Context(), &iam.GetRoleInput{RoleName: aws.String(roleName)}); err != nil {
		return false, nil
	}
	_, err = ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN)
	return err == nil, nil
}

func (s *CreateAdminRole) Run(ctx *pipeline.ExecutionContext) error {
	if ctx.DryRun() {
		ctx.Log().Info("[dry-run] would create IAM admin role")
		return nil
	}

	// 1. Read role name from configctl.
	roleName, err := ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleName)
	if err != nil || roleName == "" {
		roleName = defaultRoleName
	}

	// 2. Get account ID via STS.
	stsClient := sts.NewFromConfig(ctx.AWSConfig())
	identity, err := stsClient.GetCallerIdentity(ctx.Context(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("GetCallerIdentity: %w", err)
	}
	accountID := *identity.Account

	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "arn:aws:iam::%s:root"},
    "Action": "sts:AssumeRole"
  }]
}`, accountID)

	iamClient := iam.NewFromConfig(ctx.AWSConfig())

	// 3. Create role; handle EntityAlreadyExistsException as success.
	var roleARN string
	createOut, err := iamClient.CreateRole(ctx.Context(), &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Description:              aws.String("Platform orchestrator admin role — managed by platform-orchestrator"),
	})
	if err != nil {
		// Check if it already exists.
		getOut, getErr := iamClient.GetRole(ctx.Context(), &iam.GetRoleInput{RoleName: aws.String(roleName)})
		if getErr != nil {
			return fmt.Errorf("CreateRole %q: %w", roleName, err)
		}
		roleARN = *getOut.Role.Arn
	} else {
		roleARN = *createOut.Role.Arn
	}

	// 4. Attach AdministratorAccess — treat "already attached" as success,
	// propagate all other errors (permission denied, throttle, etc.).
	if _, err := iamClient.AttachRolePolicy(ctx.Context(), &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(adminRolePolicy),
	}); err != nil {
		var alreadyExists *types.EntityAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return fmt.Errorf("AttachRolePolicy %q: %w", adminRolePolicy, err)
		}
	}

	// 5. Write ARN to configctl.
	if err := ctx.Config().Set(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN, roleARN); err != nil {
		return fmt.Errorf("store admin_role_arn: %w", err)
	}

	// 6. Set output.
	ctx.SetOutput("admin_role_arn", roleARN)
	return nil
}

func (s *CreateAdminRole) Rollback(ctx *pipeline.ExecutionContext) error {
	roleName, _ := ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleName)
	if roleName == "" {
		roleName = defaultRoleName
	}
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	if _, err := iamClient.DetachRolePolicy(ctx.Context(), &iam.DetachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(adminRolePolicy),
	}); err != nil {
		ctx.Log().Warn("DetachRolePolicy failed during rollback (may not be attached)")
	}
	if _, err := iamClient.DeleteRole(ctx.Context(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}); err != nil {
		return fmt.Errorf("DeleteRole %q: %w", roleName, err)
	}
	return ctx.Config().Delete(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN)
}
