package bootstrap

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// EnableSCP enables the SERVICE_CONTROL_POLICY policy type on the root OU.
type EnableSCP struct {
	cfg configctl.Client
}

// NewEnableSCP constructs an EnableSCP step.
func NewEnableSCP(cfg configctl.Client) *EnableSCP {
	return &EnableSCP{cfg: cfg}
}

func (s *EnableSCP) ID() string   { return "enable_scp" }
func (s *EnableSCP) Name() string { return "Enable SCP policy type on root OU" }
func (s *EnableSCP) Deps() []string {
	return []string{"create_org"}
}
func (s *EnableSCP) CredentialClass() credential.Class  { return credential.ClassRoot }
func (s *EnableSCP) RequiredInputs() []prompt.InputSpec { return nil }
func (s *EnableSCP) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *EnableSCP) IsDone(ctx *pipeline.ExecutionContext) (bool, error) {
	orgClient := organizations.NewFromConfig(ctx.AWSConfig())
	out, err := orgClient.ListRoots(ctx.Context(), &organizations.ListRootsInput{})
	if err != nil {
		return false, nil
	}
	if len(out.Roots) == 0 {
		return false, nil
	}
	for _, pt := range out.Roots[0].PolicyTypes {
		if pt.Type == orgtypes.PolicyTypeServiceControlPolicy &&
			pt.Status == orgtypes.PolicyTypeStatusEnabled {
			return true, nil
		}
	}
	return false, nil
}

func (s *EnableSCP) Run(ctx *pipeline.ExecutionContext) error {
	if ctx.DryRun() {
		ctx.Log().Info("[dry-run] would enable SCP policy type")
		return nil
	}

	rootOUID, err := ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapRootOUID)
	if err != nil {
		return fmt.Errorf("root_ou_id not found: %w", err)
	}

	orgClient := organizations.NewFromConfig(ctx.AWSConfig())
	_, err = orgClient.EnablePolicyType(ctx.Context(), &organizations.EnablePolicyTypeInput{
		RootId:     aws.String(rootOUID),
		PolicyType: orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		var alreadyEnabled *orgtypes.PolicyTypeAlreadyEnabledException
		if !errors.As(err, &alreadyEnabled) {
			return fmt.Errorf("EnablePolicyType: %w", err)
		}
	}
	return nil
}

func (s *EnableSCP) Rollback(ctx *pipeline.ExecutionContext) error {
	rootOUID, err := ctx.Config().Get(ctx.Context(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapRootOUID)
	if err != nil {
		return nil // nothing to roll back
	}
	orgClient := organizations.NewFromConfig(ctx.AWSConfig())
	_, err = orgClient.DisablePolicyType(ctx.Context(), &organizations.DisablePolicyTypeInput{
		RootId:     aws.String(rootOUID),
		PolicyType: orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		return fmt.Errorf("DisablePolicyType: %w", err)
	}
	return nil
}
