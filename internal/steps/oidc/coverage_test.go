package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

type fakeIAMClient struct {
	listProvidersOut  *iam.ListOpenIDConnectProvidersOutput
	listProvidersErr  error
	getProviderOut    *iam.GetOpenIDConnectProviderOutput
	getProviderErr    error
	createProviderOut *iam.CreateOpenIDConnectProviderOutput
	createProviderErr error
	deleteProviderErr error
	getRoleOut        *iam.GetRoleOutput
	getRoleErr        error
	createRoleOut     *iam.CreateRoleOutput
	createRoleErr     error
	deleteRoleErr     error
}

func (f *fakeIAMClient) ListOpenIDConnectProviders(context.Context, *iam.ListOpenIDConnectProvidersInput, ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	return f.listProvidersOut, f.listProvidersErr
}
func (f *fakeIAMClient) GetOpenIDConnectProvider(context.Context, *iam.GetOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	return f.getProviderOut, f.getProviderErr
}
func (f *fakeIAMClient) CreateOpenIDConnectProvider(context.Context, *iam.CreateOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error) {
	return f.createProviderOut, f.createProviderErr
}
func (f *fakeIAMClient) DeleteOpenIDConnectProvider(context.Context, *iam.DeleteOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.DeleteOpenIDConnectProviderOutput, error) {
	return &iam.DeleteOpenIDConnectProviderOutput{}, f.deleteProviderErr
}
func (f *fakeIAMClient) GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	return f.getRoleOut, f.getRoleErr
}
func (f *fakeIAMClient) CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	return f.createRoleOut, f.createRoleErr
}
func (f *fakeIAMClient) DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
	return &iam.DeleteRoleOutput{}, f.deleteRoleErr
}

type fakeSTSClient struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f *fakeSTSClient) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f.out, f.err
}

func TestCreateProviderStep_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	step := NewCreateProviderStep()
	if step.ID() == "" || step.Name() == "" || step.CredentialClass().String() == "" || step.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected metadata")
	}

	iamClient := &fakeIAMClient{
		listProvidersOut: &iam.ListOpenIDConnectProvidersOutput{
			OpenIDConnectProviderList: []iamtypes.OpenIDConnectProviderListEntry{{Arn: aws.String("arn:provider")}},
		},
		getProviderOut:    &iam.GetOpenIDConnectProviderOutput{Url: aws.String(githubOIDCURL)},
		createProviderOut: &iam.CreateOpenIDConnectProviderOutput{OpenIDConnectProviderArn: aws.String("arn:provider")},
	}
	origIAM := newIAMClient
	origThumbprint := githubThumbprintFn
	newIAMClient = func(aws.Config) iamAPI { return iamClient }
	githubThumbprintFn = func(context.Context, string) (string, error) { return "thumbprint", nil }
	t.Cleanup(func() {
		newIAMClient = origIAM
		githubThumbprintFn = origThumbprint
	})

	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if execCtx.Outputs()["oidc_provider_arn"] != "arn:provider" {
		t.Fatalf("unexpected outputs: %+v", execCtx.Outputs())
	}
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}

func TestCreateGitHubRoleStep_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), platformProject, globalEnv, oidcProviderKey, "arn:provider")
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOIDCGitHubOrg, "acme")
	execCtx := newExecCtx(cfg, false)
	step := NewCreateGitHubRoleStep()

	iamClient := &fakeIAMClient{
		getRoleOut:    &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:role")}},
		createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:role")}},
	}
	stsClient := &fakeSTSClient{out: &sts.GetCallerIdentityOutput{Account: aws.String("123")}}
	origIAM := newIAMClient
	origSTS := newSTSClient
	newIAMClient = func(aws.Config) iamAPI { return iamClient }
	newSTSClient = func(aws.Config) callerIdentityGetter { return stsClient }
	t.Cleanup(func() {
		newIAMClient = origIAM
		newSTSClient = origSTS
	})

	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if execCtx.Outputs()["github_actions_role_arn"] != "arn:role" {
		t.Fatalf("unexpected outputs: %+v", execCtx.Outputs())
	}
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}

func TestCreateProviderStep_FallbackThumbprintAndFetchErrors(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	iamClient := &fakeIAMClient{
		listProvidersOut:  &iam.ListOpenIDConnectProvidersOutput{},
		createProviderOut: &iam.CreateOpenIDConnectProviderOutput{OpenIDConnectProviderArn: aws.String("arn:provider")},
	}
	origIAM := newIAMClient
	origThumbprint := githubThumbprintFn
	newIAMClient = func(aws.Config) iamAPI { return iamClient }
	githubThumbprintFn = func(context.Context, string) (string, error) { return "", errors.New("fetch") }
	t.Cleanup(func() {
		newIAMClient = origIAM
		githubThumbprintFn = origThumbprint
	})

	if err := NewCreateProviderStep().Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if _, err := fetchThumbprint(context.Background(), "://bad-url"); err == nil {
		t.Fatal("expected request build error")
	}

	noTLS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	defer noTLS.Close()
	if _, err := fetchThumbprint(context.Background(), noTLS.URL); err == nil {
		t.Fatal("expected missing tls cert error")
	}
}

func TestFetchThumbprintTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	origHTTPDo := httpDo
	client := server.Client()
	httpDo = func(req *http.Request) (*http.Response, error) { return client.Do(req) }
	t.Cleanup(func() { httpDo = origHTTPDo })

	if fp, err := fetchThumbprint(context.Background(), server.URL); err != nil || fp == "" {
		t.Fatalf("fetchThumbprint() = %q, %v", fp, err)
	}
}
