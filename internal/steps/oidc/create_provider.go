package oidc

import (
	"context"
	"crypto/sha1" // #nosec G505
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

const (
	githubOIDCURL   = "https://token.actions.githubusercontent.com"
	oidcProviderKey = platformcfg.KeyOIDCProviderARN
	githubAudience  = "sts.amazonaws.com"
)

// CreateProviderStep creates the IAM OIDC provider for GitHub Actions.
type CreateProviderStep struct{}

func NewCreateProviderStep() *CreateProviderStep { return &CreateProviderStep{} }

func (s *CreateProviderStep) ID() string   { return "create_oidc_provider" }
func (s *CreateProviderStep) Name() string { return "Create IAM OIDC provider (GitHub Actions)" }
func (s *CreateProviderStep) Deps() []string {
	return []string{"terraform_apply_org"}
}
func (s *CreateProviderStep) CredentialClass() credential.Class  { return credential.ClassAdmin }
func (s *CreateProviderStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *CreateProviderStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }

func (s *CreateProviderStep) IsDone(ctx context.Context, execCtx *pipeline.ExecutionContext) (bool, error) {
	iamClient := newIAMClient(execCtx.AWSConfig())
	listOut, err := iamClient.ListOpenIDConnectProviders(ctx, &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return false, nil
	}
	for _, p := range listOut.OpenIDConnectProviderList {
		if p.Arn == nil {
			continue
		}
		detail, derr := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: p.Arn,
		})
		if derr == nil && detail.Url != nil && *detail.Url == githubOIDCURL {
			// Check if stored.
			_, storeErr := execCtx.Config().Get(ctx, platformProject, globalEnv, oidcProviderKey)
			return storeErr == nil, nil
		}
	}
	return false, nil
}

func (s *CreateProviderStep) Run(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	if execCtx.DryRun() {
		execCtx.Log().Info("[dry-run] would create OIDC provider for " + githubOIDCURL)
		return nil
	}

	iamClient := newIAMClient(execCtx.AWSConfig())

	// Check if provider already exists.
	listOut, err := iamClient.ListOpenIDConnectProviders(ctx, &iam.ListOpenIDConnectProvidersInput{})
	if err == nil {
		for _, p := range listOut.OpenIDConnectProviderList {
			if p.Arn == nil {
				continue
			}
			detail, derr := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: p.Arn,
			})
			if derr == nil && detail.Url != nil && *detail.Url == githubOIDCURL {
				return s.storeAndOutput(ctx, execCtx, *p.Arn)
			}
		}
	}

	// Fetch thumbprint. Use known stable thumbprint as fallback.
	thumbprint, err := githubThumbprintFn(ctx, githubOIDCURL)
	if err != nil {
		// Fallback: use known stable thumbprint (fetched live in production).
		thumbprint = "6938fd4d98bab03faadb97b34396831e3780aea1"
	}

	createOut, err := iamClient.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(githubOIDCURL),
		ClientIDList:   []string{githubAudience},
		ThumbprintList: []string{thumbprint},
		Tags:           []iamtypes.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("platform-orchestrator")}},
	})
	if err != nil {
		return fmt.Errorf("CreateOpenIDConnectProvider: %w", err)
	}

	return s.storeAndOutput(ctx, execCtx, *createOut.OpenIDConnectProviderArn)
}

func (s *CreateProviderStep) storeAndOutput(ctx context.Context, execCtx *pipeline.ExecutionContext, arn string) error {
	if err := execCtx.Config().Set(ctx, platformProject, globalEnv, oidcProviderKey, arn); err != nil {
		return fmt.Errorf("store oidc_provider_arn: %w", err)
	}
	execCtx.SetOutput("oidc_provider_arn", arn)
	return nil
}

func (s *CreateProviderStep) Rollback(ctx context.Context, execCtx *pipeline.ExecutionContext) error {
	arn, err := execCtx.Config().Get(ctx, platformProject, globalEnv, oidcProviderKey)
	if err != nil {
		return nil // nothing to roll back
	}
	iamClient := newIAMClient(execCtx.AWSConfig())
	if _, err := iamClient.DeleteOpenIDConnectProvider(ctx, &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(arn),
	}); err != nil {
		return fmt.Errorf("DeleteOpenIDConnectProvider: %w", err)
	}
	return execCtx.Config().Delete(ctx, platformProject, globalEnv, oidcProviderKey)
}

// fetchThumbprint fetches the TLS thumbprint from the OIDC provider endpoint.
func fetchThumbprint(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", fmt.Errorf("build request %s: %w", rawURL, err)
	}
	resp, err := httpDo(req) // #nosec G107
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return "", fmt.Errorf("no TLS peer certificates from %s", rawURL)
	}
	cert := resp.TLS.PeerCertificates[len(resp.TLS.PeerCertificates)-1]
	// AWS requires SHA-1 for OIDC provider thumbprints.
	// This hashes the DER bytes of a *public* X.509 certificate from the TLS
	// handshake to produce an AWS-compatible thumbprint (a fingerprint), not to
	// protect secrets (password hashing/signing/MAC/etc).
	// BEGIN-SONAR-IGNORE-OIDC-THUMBPRINT
	// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	fp := sha1.Sum(cert.Raw) // #nosec G401 -- required for AWS OIDC thumbprints // NOSONAR
	// END-SONAR-IGNORE-OIDC-THUMBPRINT
	return fmt.Sprintf("%x", fp), nil
}
