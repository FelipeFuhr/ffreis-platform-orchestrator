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
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

func TestBootstrapStepMetadata(t *testing.T) {
	create := NewCreateOrg(newMemConfig())
	if len(create.Deps()) != 0 || create.CredentialClass() != credential.ClassRoot || len(create.RequiredInputs()) != 0 || create.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected create org metadata")
	}

	enable := NewEnableSCP(newMemConfig())
	if len(enable.Deps()) != 1 || enable.CredentialClass() != credential.ClassRoot || len(enable.RequiredInputs()) != 0 || enable.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected enable scp metadata")
	}

	admin := NewCreateAdminRole(newMemConfig())
	if len(admin.Deps()) != 1 || admin.CredentialClass() != credential.ClassRoot || len(admin.RequiredInputs()) == 0 || admin.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected create admin role metadata")
	}

	verify := NewVerifyRootStep()
	if len(verify.Deps()) != 0 || verify.CredentialClass() != credential.ClassRoot || len(verify.RequiredInputs()) != 0 || verify.RetryPolicy() != pipeline.NoRetry {
		t.Fatal("unexpected verify root metadata")
	}
}

func TestCreateOrg_RunAlreadyExistsBranch(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	orgClient := &fakeOrgClient{
		createErr:   &orgtypes.AlreadyInOrganizationException{},
		describeOut: &organizations.DescribeOrganizationOutput{Organization: &orgtypes.Organization{Id: aws.String("o-999")}},
		rootsOut:    &organizations.ListRootsOutput{Roots: []orgtypes.Root{{Id: aws.String("r-root")}}},
	}
	orig := newOrganizationsClient
	newOrganizationsClient = func(aws.Config) organizationsAPI { return orgClient }
	t.Cleanup(func() { newOrganizationsClient = orig })

	if err := NewCreateOrg(cfg).Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

func TestEnableSCP_RunMissingRootAndCreateAdminRoleExisting(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)

	if err := NewEnableSCP(cfg).Run(context.Background(), execCtx); err == nil {
		t.Fatal("expected missing root id error")
	}

	_ = cfg.Set(context.Background(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyBootstrapRootOUID, "r-root")
	iamClient := &fakeIAMClient{
		createRoleErr: &iamtypes.EntityAlreadyExistsException{},
		getRoleOut:    &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:existing")}},
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

	if err := NewCreateAdminRole(cfg).Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() existing role error: %v", err)
	}
}

func TestCreateAdminRole_RollbackError(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	_ = cfg.Set(context.Background(), platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN, "arn:aws:iam::123:role/platform-admin")

	origIAM := newIAMClient
	newIAMClient = func(aws.Config) iamAPI {
		return &fakeIAMClient{detachErr: errors.New("detach"), deleteRoleErr: errors.New("delete")}
	}
	t.Cleanup(func() { newIAMClient = origIAM })

	if err := NewCreateAdminRole(cfg).Rollback(context.Background(), execCtx); err == nil {
		t.Fatal("expected rollback error")
	}
}
