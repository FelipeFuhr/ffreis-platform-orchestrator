package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

type fakeOrgClient struct {
	describeOut *organizations.DescribeOrganizationOutput
	describeErr error
	createOut   *organizations.CreateOrganizationOutput
	createErr   error
	rootsOut    *organizations.ListRootsOutput
	rootsErr    error
	enableErr   error
	disableErr  error
}

func (f *fakeOrgClient) DescribeOrganization(context.Context, *organizations.DescribeOrganizationInput, ...func(*organizations.Options)) (*organizations.DescribeOrganizationOutput, error) {
	return f.describeOut, f.describeErr
}
func (f *fakeOrgClient) CreateOrganization(context.Context, *organizations.CreateOrganizationInput, ...func(*organizations.Options)) (*organizations.CreateOrganizationOutput, error) {
	return f.createOut, f.createErr
}
func (f *fakeOrgClient) ListRoots(context.Context, *organizations.ListRootsInput, ...func(*organizations.Options)) (*organizations.ListRootsOutput, error) {
	return f.rootsOut, f.rootsErr
}
func (f *fakeOrgClient) EnablePolicyType(context.Context, *organizations.EnablePolicyTypeInput, ...func(*organizations.Options)) (*organizations.EnablePolicyTypeOutput, error) {
	return &organizations.EnablePolicyTypeOutput{}, f.enableErr
}
func (f *fakeOrgClient) DisablePolicyType(context.Context, *organizations.DisablePolicyTypeInput, ...func(*organizations.Options)) (*organizations.DisablePolicyTypeOutput, error) {
	return &organizations.DisablePolicyTypeOutput{}, f.disableErr
}

type fakeIAMClient struct {
	getRoleOut    *iam.GetRoleOutput
	getRoleErr    error
	createRoleOut *iam.CreateRoleOutput
	createRoleErr error
	attachErr     error
	detachErr     error
	deleteRoleErr error
}

func (f *fakeIAMClient) GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	return f.getRoleOut, f.getRoleErr
}
func (f *fakeIAMClient) CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	return f.createRoleOut, f.createRoleErr
}
func (f *fakeIAMClient) AttachRolePolicy(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	return &iam.AttachRolePolicyOutput{}, f.attachErr
}
func (f *fakeIAMClient) DetachRolePolicy(context.Context, *iam.DetachRolePolicyInput, ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	return &iam.DetachRolePolicyOutput{}, f.detachErr
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

func TestCreateOrg_MetadataAndRun(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	step := NewCreateOrg(cfg)
	if step.ID() == "" || step.Name() == "" || step.CredentialClass().String() == "" || step.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected step metadata")
	}
	if err := step.Rollback(context.Background(), execCtx); err == nil {
		t.Fatal("expected rollback not supported")
	}

	orgClient := &fakeOrgClient{
		describeOut: &organizations.DescribeOrganizationOutput{Organization: &orgtypes.Organization{Id: aws.String("o-123")}},
		createOut:   &organizations.CreateOrganizationOutput{Organization: &orgtypes.Organization{Id: aws.String("o-123")}},
		rootsOut:    &organizations.ListRootsOutput{Roots: []orgtypes.Root{{Id: aws.String("r-root")}}},
	}
	origOrg := newOrganizationsClient
	newOrganizationsClient = func(aws.Config) organizationsAPI { return orgClient }
	t.Cleanup(func() { newOrganizationsClient = origOrg })

	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if got := execCtx.Outputs()["org_id"]; got != "o-123" {
		t.Fatalf("unexpected org_id: %q", got)
	}
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
}

func TestEnableSCP_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapRootOUID, "r-root")
	execCtx := newExecCtx(cfg, false)

	orgClient := &fakeOrgClient{
		rootsOut: &organizations.ListRootsOutput{Roots: []orgtypes.Root{{
			PolicyTypes: []orgtypes.PolicyTypeSummary{{
				Type: orgtypes.PolicyTypeServiceControlPolicy, Status: orgtypes.PolicyTypeStatusEnabled,
			}},
		}}},
		enableErr:  &orgtypes.PolicyTypeAlreadyEnabledException{},
		disableErr: nil,
	}
	origOrg := newOrganizationsClient
	newOrganizationsClient = func(aws.Config) organizationsAPI { return orgClient }
	t.Cleanup(func() { newOrganizationsClient = origOrg })

	step := NewEnableSCP(cfg)
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}

func TestVerifyRoot_RunBranches(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	step := NewVerifyRootStep()

	origSTS := newSTSClient
	t.Cleanup(func() { newSTSClient = origSTS })

	newSTSClient = func(aws.Config) stsAPI { return &fakeSTSClient{err: errors.New("sts")} }
	if err := step.Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected sts error")
	}

	newSTSClient = func(aws.Config) stsAPI { return &fakeSTSClient{out: &sts.GetCallerIdentityOutput{}} }
	if err := step.Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected missing account error")
	}

	newSTSClient = func(aws.Config) stsAPI {
		return &fakeSTSClient{out: &sts.GetCallerIdentityOutput{Account: aws.String("123")}}
	}
	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if execCtx.Outputs()["account_id"] != "123" {
		t.Fatalf("unexpected outputs: %+v", execCtx.Outputs())
	}
}

func TestCreateAdminRole_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	step := NewCreateAdminRole(cfg)

	iamClient := &fakeIAMClient{
		createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123:role/platform-admin")}},
	}
	stsClient := &fakeSTSClient{out: &sts.GetCallerIdentityOutput{Account: aws.String("123456789012")}}
	origIAM := newIAMClient
	origSTS := newSTSClient
	newIAMClient = func(aws.Config) iamAPI { return iamClient }
	newSTSClient = func(aws.Config) stsAPI { return stsClient }
	t.Cleanup(func() {
		newIAMClient = origIAM
		newSTSClient = origSTS
	})

	if err := step.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	done, err := step.IsDone(context.Background(), execCtx)
	if err != nil || !done {
		t.Fatalf("IsDone() = %v, %v", done, err)
	}
	if execCtx.Outputs()["admin_role_arn"] == "" {
		t.Fatal("expected output to be set")
	}
	if err := step.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
}
