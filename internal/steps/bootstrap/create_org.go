package bootstrap

import (
	"context"
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

// CreateOrg creates an AWS Organization. This is the first step in the pipeline.
type CreateOrg struct {
	cfg configctl.Client
}

// NewCreateOrg constructs a CreateOrg step.
func NewCreateOrg(cfg configctl.Client) *CreateOrg {
	return &CreateOrg{cfg: cfg}
}

func (s *CreateOrg) ID() string                         { return "create_org" }
func (s *CreateOrg) Name() string                       { return "Create AWS Organization" }
func (s *CreateOrg) Deps() []string                     { return nil }
func (s *CreateOrg) CredentialClass() credential.Class  { return credential.ClassRoot }
func (s *CreateOrg) RequiredInputs() []prompt.InputSpec { return nil }
func (s *CreateOrg) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *CreateOrg) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	orgClient := newOrganizationsClient(execCtx.AWSConfig())
	out, err := orgClient.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return false, nil
	}
	if out.Organization == nil || out.Organization.Id == nil {
		return false, nil
	}
	// Check if org ID is stored.
	_, err = execCtx.Config().Get(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapOrgID)
	return err == nil, nil
}

func (s *CreateOrg) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	if execCtx.DryRun() {
		execCtx.Log().Info("[dry-run] would create AWS Organization")
		return nil
	}

	orgClient := newOrganizationsClient(execCtx.AWSConfig())

	var orgID string

	createOut, err := orgClient.CreateOrganization(ctx, &organizations.CreateOrganizationInput{
		FeatureSet: orgtypes.OrganizationFeatureSetAll,
	})
	if err != nil {
		var alreadyInOrg *orgtypes.AlreadyInOrganizationException
		if !errors.As(err, &alreadyInOrg) {
			return fmt.Errorf("CreateOrganization: %w", err)
		}
		// Already in org — describe to get the ID.
		descOut, descErr := orgClient.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
		if descErr != nil {
			return fmt.Errorf("DescribeOrganization: %w", descErr)
		}
		orgID = aws.ToString(descOut.Organization.Id)
	} else {
		orgID = aws.ToString(createOut.Organization.Id)
	}

	// Get root OU ID.
	rootsOut, err := orgClient.ListRoots(ctx, &organizations.ListRootsInput{})
	if err != nil {
		return fmt.Errorf("ListRoots: %w", err)
	}
	if len(rootsOut.Roots) == 0 {
		return fmt.Errorf("no roots found in organization")
	}
	rootOUID := aws.ToString(rootsOut.Roots[0].Id)

	// Store in configctl.
	if err := execCtx.Config().Set(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapOrgID, orgID); err != nil {
		return fmt.Errorf("store org_id: %w", err)
	}
	if err := execCtx.Config().Set(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapRootOUID, rootOUID); err != nil {
		return fmt.Errorf("store root_ou_id: %w", err)
	}

	execCtx.SetOutput("org_id", orgID)
	execCtx.SetOutput("root_ou_id", rootOUID)
	return nil
}

func (s *CreateOrg) Rollback(_ context.Context, _ *pipeline.ExecutionContext) error {
	return pipeline.ErrRollbackNotSupported("create_org")
}
