package oidc

import (
	"fmt"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// WriteOutputsStep copies final ARNs from _platform/global to the target project/env.
type WriteOutputsStep struct{}

func NewWriteOutputsStep() *WriteOutputsStep { return &WriteOutputsStep{} }

func (s *WriteOutputsStep) ID() string   { return "write_oidc_outputs" }
func (s *WriteOutputsStep) Name() string { return "Write OIDC outputs to target project" }
func (s *WriteOutputsStep) Deps() []string {
	return []string{"create_github_actions_role"}
}
func (s *WriteOutputsStep) CredentialClass() credential.Class  { return credential.ClassOperator }
func (s *WriteOutputsStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *WriteOutputsStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *WriteOutputsStep) IsDone(ctx *pipeline.ExecutionContext) (bool, error) {
	_, err := ctx.Config().Get(ctx.Context(), ctx.Project(), ctx.Env(), outputKeyGitHubRoleARN)
	return err == nil, nil
}

func (s *WriteOutputsStep) Run(ctx *pipeline.ExecutionContext) error {
	// 1. Read from _platform/global.
	roleARN, err := ctx.Config().Get(ctx.Context(), platformProject, globalEnv, githubRoleARNKey)
	if err != nil {
		return fmt.Errorf("read github_actions_role_arn from _platform/global: %w", err)
	}
	providerARN, err := ctx.Config().Get(ctx.Context(), platformProject, globalEnv, oidcProviderKey)
	if err != nil {
		return fmt.Errorf("read oidc_provider_arn from _platform/global: %w", err)
	}

	// 2. Write to target project/env.
	if err := ctx.Config().Set(ctx.Context(), ctx.Project(), ctx.Env(), outputKeyGitHubRoleARN, roleARN); err != nil {
		return fmt.Errorf("write github_actions_role_arn: %w", err)
	}
	if err := ctx.Config().Set(ctx.Context(), ctx.Project(), ctx.Env(), outputKeyOIDCProvider, providerARN); err != nil {
		return fmt.Errorf("write oidc_provider_arn: %w", err)
	}
	return nil
}

func (s *WriteOutputsStep) Rollback(ctx *pipeline.ExecutionContext) error {
	keys := []string{
		outputKeyGitHubRoleARN,
		outputKeyOIDCProvider,
	}
	for _, k := range keys {
		_ = ctx.Config().Delete(ctx.Context(), ctx.Project(), ctx.Env(), k)
	}
	return nil
}
