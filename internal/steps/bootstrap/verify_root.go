package bootstrap

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// VerifyRootStep validates root credentials before any bootstrap work.
type VerifyRootStep struct{}

func NewVerifyRootStep() *VerifyRootStep { return &VerifyRootStep{} }

func (s *VerifyRootStep) ID() string                         { return "verify_root_credentials" }
func (s *VerifyRootStep) Name() string                       { return "Verify root AWS credentials" }
func (s *VerifyRootStep) Deps() []string                     { return nil }
func (s *VerifyRootStep) CredentialClass() credential.Class  { return credential.ClassRoot }
func (s *VerifyRootStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *VerifyRootStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *VerifyRootStep) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	_, err := execCtx.Config().Get(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAccountID)
	return err == nil, nil
}

func (s *VerifyRootStep) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	stsClient := newSTSClient(execCtx.AWSConfig())
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("GetCallerIdentity failed — check root credentials: %w", err)
	}
	if out.Account == nil {
		return fmt.Errorf("GetCallerIdentity returned no account ID")
	}
	if err := execCtx.Config().Set(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAccountID, *out.Account); err != nil {
		return fmt.Errorf("store account_id: %w", err)
	}
	execCtx.SetOutput("account_id", *out.Account)
	return nil
}

func (s *VerifyRootStep) Rollback(_ context.Context, _ *pipeline.ExecutionContext) error {
	return pipeline.ErrRollbackNotSupported(s.ID())
}
