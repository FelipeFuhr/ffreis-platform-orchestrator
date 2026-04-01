package bootstrap

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type organizationsAPI interface {
	DescribeOrganization(context.Context, *organizations.DescribeOrganizationInput, ...func(*organizations.Options)) (*organizations.DescribeOrganizationOutput, error)
	CreateOrganization(context.Context, *organizations.CreateOrganizationInput, ...func(*organizations.Options)) (*organizations.CreateOrganizationOutput, error)
	ListRoots(context.Context, *organizations.ListRootsInput, ...func(*organizations.Options)) (*organizations.ListRootsOutput, error)
	EnablePolicyType(context.Context, *organizations.EnablePolicyTypeInput, ...func(*organizations.Options)) (*organizations.EnablePolicyTypeOutput, error)
	DisablePolicyType(context.Context, *organizations.DisablePolicyTypeInput, ...func(*organizations.Options)) (*organizations.DisablePolicyTypeOutput, error)
}

type iamAPI interface {
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	AttachRolePolicy(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	DetachRolePolicy(context.Context, *iam.DetachRolePolicyInput, ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
}

type callerIdentityGetter interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

var (
	newOrganizationsClient = func(cfg aws.Config) organizationsAPI { return organizations.NewFromConfig(cfg) }
	newIAMClient           = func(cfg aws.Config) iamAPI { return iam.NewFromConfig(cfg) }
	newSTSClient           = func(cfg aws.Config) callerIdentityGetter { return sts.NewFromConfig(cfg) }

	_ orgtypes.PolicyType
	_ iamtypes.Role
)
