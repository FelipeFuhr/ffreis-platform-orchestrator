package config

// Platform configctl locations.
const (
	PlatformProject = "_platform"
	GlobalEnv       = "global"
)

// Config keys stored in configctl.
const (
	KeyAccountID       = "orchestrator/account_id"
	KeyAdminRoleARN    = "orchestrator/admin_role_arn"
	KeyAdminRoleName   = "orchestrator/admin_role_name"
	KeyOperatorRoleARN = "orchestrator/operator_role_arn"

	KeyBootstrapOrgID    = "orchestrator/bootstrap/org_id"
	KeyBootstrapRootOUID = "orchestrator/bootstrap/root_ou_id"

	KeyOIDCProviderARN = "orchestrator/oidc_provider_arn"
	KeyGitHubRoleARN   = "orchestrator/github_actions_role_arn"

	KeyLastRunID = "orchestrator/last_run_id"
)

const (
	runPrefix        = "orchestrator/run/"
	stepOutputPrefix = "orchestrator/step/"
)

func RunPrefix(runID string) string { return runPrefix + runID }

func RunMetaKey(runID string) string { return runPrefix + runID + "/meta" }

func StepStateKey(runID, stepID string) string {
	return runPrefix + runID + "/step/" + stepID + "/state"
}

func StepOutputKey(stepID, outputKey string) string {
	return stepOutputPrefix + stepID + "/output/" + outputKey
}
