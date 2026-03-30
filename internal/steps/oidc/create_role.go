package oidc

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

const (
	githubRoleARNKey = platformcfg.KeyGitHubRoleARN
)

// CreateGitHubRoleStep creates the IAM role trusted by the OIDC provider.
type CreateGitHubRoleStep struct{}

func NewCreateGitHubRoleStep() *CreateGitHubRoleStep { return &CreateGitHubRoleStep{} }

func (s *CreateGitHubRoleStep) ID() string   { return "create_github_actions_role" }
func (s *CreateGitHubRoleStep) Name() string { return "Create IAM role for GitHub Actions" }
func (s *CreateGitHubRoleStep) Deps() []string {
	return []string{"create_oidc_provider"}
}
func (s *CreateGitHubRoleStep) CredentialClass() credential.Class { return credential.ClassAdmin }
func (s *CreateGitHubRoleStep) RequiredInputs() []prompt.InputSpec {
	return []prompt.InputSpec{
		{Key: configKeyOIDCGitHubOrg, Label: "GitHub organisation name"},
		{Key: configKeyOIDCGitHubRepo, Label: "GitHub repository (or '*' for all)"},
		{Key: configKeyOIDCRoleName, Label: "IAM role name for GitHub Actions", Default: defaultGitHubRoleName},
	}
}
func (s *CreateGitHubRoleStep) RetryPolicy() pipeline.RetryPolicy { return pipeline.NoRetry }

func (s *CreateGitHubRoleStep) IsDone(ctx *pipeline.ExecutionContext) (bool, error) {
	roleName, err := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), configKeyOIDCRoleName)
	if err != nil || roleName == "" {
		roleName = defaultGitHubRoleName
	}
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	if _, err := iamClient.GetRole(ctx.Context(), &iam.GetRoleInput{RoleName: aws.String(roleName)}); err != nil {
		return false, nil
	}
	_, err = ctx.Config().Get(ctx.Context(), platformProject, globalEnv, githubRoleARNKey)
	return err == nil, nil
}

func (s *CreateGitHubRoleStep) Run(ctx *pipeline.ExecutionContext) error {
	if ctx.DryRun() {
		ctx.Log().Info("[dry-run] would create GitHub Actions IAM role")
		return nil
	}

	// 1. Read inputs from configctl.
	providerARN, err := ctx.Config().Get(ctx.Context(), platformProject, globalEnv, oidcProviderKey)
	if err != nil {
		return fmt.Errorf("oidc_provider_arn not found: %w", err)
	}
	githubOrg, err := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), configKeyOIDCGitHubOrg)
	if err != nil {
		return fmt.Errorf("oidc/github_org not found: %w", err)
	}
	githubRepo, _ := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), configKeyOIDCGitHubRepo)
	if githubRepo == "" {
		githubRepo = "*"
	}
	roleName, _ := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), configKeyOIDCRoleName)
	if roleName == "" {
		roleName = defaultGitHubRoleName
	}

	// 2. Get account ID.
	stsClient := sts.NewFromConfig(ctx.AWSConfig())
	identity, err := stsClient.GetCallerIdentity(ctx.Context(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("GetCallerIdentity: %w", err)
	}
	_ = identity // not needed directly in trust policy, providerARN is used

	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Federated": "%s"},
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringLike": {
        "token.actions.githubusercontent.com:sub": "repo:%s/%s:*"
      },
      "StringEquals": {
        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
      }
    }
  }]
}`, providerARN, githubOrg, githubRepo)

	iamClient := iam.NewFromConfig(ctx.AWSConfig())

	// 4. CreateRole; handle EntityAlreadyExistsException.
	var roleARN string
	createOut, err := iamClient.CreateRole(ctx.Context(), &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Description:              aws.String("GitHub Actions deployment role — managed by platform-orchestrator"),
		Tags:                     []iamtypes.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("platform-orchestrator")}},
	})
	if err != nil {
		getOut, getErr := iamClient.GetRole(ctx.Context(), &iam.GetRoleInput{RoleName: aws.String(roleName)})
		if getErr != nil {
			return fmt.Errorf("CreateRole %q: %w", roleName, err)
		}
		roleARN = *getOut.Role.Arn
	} else {
		roleARN = *createOut.Role.Arn
	}

	// 5. Write ARN to configctl.
	if err := ctx.Config().Set(ctx.Context(), platformProject, globalEnv, githubRoleARNKey, roleARN); err != nil {
		return fmt.Errorf("store github_actions_role_arn: %w", err)
	}

	// 6. Set output.
	ctx.SetOutput("github_actions_role_arn", roleARN)
	return nil
}

func (s *CreateGitHubRoleStep) Rollback(ctx *pipeline.ExecutionContext) error {
	roleName, _ := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), configKeyOIDCRoleName)
	if roleName == "" {
		roleName = defaultGitHubRoleName
	}
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	if _, err := iamClient.DeleteRole(ctx.Context(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}); err != nil {
		return fmt.Errorf("DeleteRole %q: %w", roleName, err)
	}
	return ctx.Config().Delete(ctx.Context(), platformProject, globalEnv, githubRoleARNKey)
}
