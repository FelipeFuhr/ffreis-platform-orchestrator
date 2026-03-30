package pipelines

import (
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/steps"
	"github.com/ffreis/platform-orchestrator/internal/steps/bootstrap"
	"github.com/ffreis/platform-orchestrator/internal/steps/oidc"
	tfsteps "github.com/ffreis/platform-orchestrator/internal/steps/terraform"
)

// PlatformSetupPipeline builds the canonical platform provisioning pipeline.
// Register steps here to add them to the flow; dependencies are declared
// within each step implementation.
func PlatformSetupPipeline(cfg configctl.Client) (*pipeline.DAG, error) {
	reg := steps.NewRegistry()

	toRegister := []pipeline.Step{
		// Bootstrap (ClassRoot).
		bootstrap.NewCreateOrg(cfg),
		bootstrap.NewEnableSCP(cfg),
		bootstrap.NewCreateAdminRole(cfg),

		// Org Terraform (ClassAdmin).
		tfsteps.NewInitStep(),
		tfsteps.NewPlanStep(),
		tfsteps.NewApplyStep(),

		// OIDC (ClassAdmin / ClassOperator).
		oidc.NewCreateProviderStep(),
		oidc.NewCreateGitHubRoleStep(),
		oidc.NewWriteOutputsStep(),
	}

	for _, s := range toRegister {
		if err := reg.Register(s); err != nil {
			return nil, err
		}
	}

	return reg.BuildDAG()
}
