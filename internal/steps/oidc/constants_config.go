package oidc

import platformcfg "github.com/ffreis/platform-orchestrator/internal/config"

const (
	platformProject = platformcfg.PlatformProject
	globalEnv       = platformcfg.GlobalEnv

	configKeyOIDCGitHubOrg  = "oidc/github_org"
	configKeyOIDCGitHubRepo = "oidc/github_repo"
	configKeyOIDCRoleName   = "oidc/role_name"

	defaultGitHubRoleName = "github-actions-deploy"
)
