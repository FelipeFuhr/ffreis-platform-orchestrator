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

func (s *CreateProviderStep) IsDone(ctx *pipeline.ExecutionContext) (bool, error) {
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	listOut, err := iamClient.ListOpenIDConnectProviders(ctx.Context(), &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return false, nil
	}
	for _, p := range listOut.OpenIDConnectProviderList {
		if p.Arn == nil {
			continue
		}
		detail, derr := iamClient.GetOpenIDConnectProvider(ctx.Context(), &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: p.Arn,
		})
		if derr == nil && detail.Url != nil && *detail.Url == githubOIDCURL {
			// Check if stored.
			_, storeErr := ctx.Config().Get(ctx.Context(), platformProject, globalEnv, oidcProviderKey)
			return storeErr == nil, nil
		}
	}
	return false, nil
}

func (s *CreateProviderStep) Run(ctx *pipeline.ExecutionContext) error {
	if ctx.DryRun() {
		ctx.Log().Info("[dry-run] would create OIDC provider for " + githubOIDCURL)
		return nil
	}

	iamClient := iam.NewFromConfig(ctx.AWSConfig())

	// Check if provider already exists.
	listOut, err := iamClient.ListOpenIDConnectProviders(ctx.Context(), &iam.ListOpenIDConnectProvidersInput{})
	if err == nil {
		for _, p := range listOut.OpenIDConnectProviderList {
			if p.Arn == nil {
				continue
			}
			detail, derr := iamClient.GetOpenIDConnectProvider(ctx.Context(), &iam.GetOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: p.Arn,
			})
			if derr == nil && detail.Url != nil && *detail.Url == githubOIDCURL {
				return s.storeAndOutput(ctx, *p.Arn)
			}
		}
	}

	// Fetch thumbprint. Use known stable thumbprint as fallback.
	thumbprint, err := fetchThumbprint(ctx.Context(), githubOIDCURL)
	if err != nil {
		// Fallback: use known stable thumbprint (fetched live in production).
		thumbprint = "6938fd4d98bab03faadb97b34396831e3780aea1"
	}

	createOut, err := iamClient.CreateOpenIDConnectProvider(ctx.Context(), &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(githubOIDCURL),
		ClientIDList:   []string{githubAudience},
		ThumbprintList: []string{thumbprint},
		Tags:           []iamtypes.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("platform-orchestrator")}},
	})
	if err != nil {
		return fmt.Errorf("CreateOpenIDConnectProvider: %w", err)
	}

	return s.storeAndOutput(ctx, *createOut.OpenIDConnectProviderArn)
}

func (s *CreateProviderStep) storeAndOutput(ctx *pipeline.ExecutionContext, arn string) error {
	if err := ctx.Config().Set(ctx.Context(), platformProject, globalEnv, oidcProviderKey, arn); err != nil {
		return fmt.Errorf("store oidc_provider_arn: %w", err)
	}
	ctx.SetOutput("oidc_provider_arn", arn)
	return nil
}

func (s *CreateProviderStep) Rollback(ctx *pipeline.ExecutionContext) error {
	arn, err := ctx.Config().Get(ctx.Context(), platformProject, globalEnv, oidcProviderKey)
	if err != nil {
		return nil // nothing to roll back
	}
	iamClient := iam.NewFromConfig(ctx.AWSConfig())
	if _, err := iamClient.DeleteOpenIDConnectProvider(ctx.Context(), &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(arn),
	}); err != nil {
		return fmt.Errorf("DeleteOpenIDConnectProvider: %w", err)
	}
	return ctx.Config().Delete(ctx.Context(), platformProject, globalEnv, oidcProviderKey)
}

// fetchThumbprint fetches the TLS thumbprint from the OIDC provider endpoint.
func fetchThumbprint(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", fmt.Errorf("build request %s: %w", rawURL, err)
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G107
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return "", fmt.Errorf("no TLS peer certificates from %s", rawURL)
	}
	cert := resp.TLS.PeerCertificates[len(resp.TLS.PeerCertificates)-1]
	fp := sha1.Sum(cert.Raw) // #nosec G401 — AWS requires SHA-1 for OIDC thumbprints
	return fmt.Sprintf("%x", fp), nil
}
