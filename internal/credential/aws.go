package credential

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

// AWSResolver resolves AWS credentials for each privilege class.
// Admin and operator ARNs are read lazily from configctl so they are
// available even when the resolver is constructed before bootstrap runs.
type AWSResolver struct {
	region string
	runID  string
	cfg    configctl.Client
}

var (
	loadDefaultAWSConfig  = awscfg.LoadDefaultConfig
	newSTSClient          = sts.NewFromConfig
	newAssumeRoleProvider = stscreds.NewAssumeRoleProvider
)

// NewAWSResolver constructs an AWSResolver.
func NewAWSResolver(region, runID string, cfg configctl.Client) *AWSResolver {
	return &AWSResolver{region: region, runID: runID, cfg: cfg}
}

// Resolve returns an aws.Config for the given credential class.
func (r *AWSResolver) Resolve(ctx context.Context, class Class) (aws.Config, error) {
	opts := []func(*awscfg.LoadOptions) error{}
	if r.region != "" {
		opts = append(opts, awscfg.WithRegion(r.region))
	}

	switch class {
	case ClassRoot:
		cfg, err := loadDefaultAWSConfig(ctx, opts...)
		if err != nil {
			return aws.Config{}, fmt.Errorf("load root credentials: %w", err)
		}
		return cfg, nil

	case ClassAdmin:
		arn, err := r.cfg.Get(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN)
		if err != nil {
			return aws.Config{}, fmt.Errorf("admin_role_arn not found in configctl: run bootstrap steps first: %w", err)
		}
		return r.assumeRole(ctx, arn, "platform-orchestrator-admin-"+r.runID, opts)

	case ClassOperator:
		arn, err := r.cfg.Get(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyOperatorRoleARN)
		if err != nil {
			return aws.Config{}, fmt.Errorf("operator_role_arn not found in configctl: %w", err)
		}
		return r.assumeRole(ctx, arn, "platform-orchestrator-operator-"+r.runID, opts)

	default:
		return aws.Config{}, fmt.Errorf("unknown credential class: %v", class)
	}
}

func (r *AWSResolver) assumeRole(ctx context.Context, roleARN, sessionName string, opts []func(*awscfg.LoadOptions) error) (aws.Config, error) {
	baseCfg, err := loadDefaultAWSConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load base credentials for role assumption: %w", err)
	}
	stsClient := newSTSClient(baseCfg)
	provider := newAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = sessionName
		o.Duration = 3600
	})
	roleCfg, err := loadDefaultAWSConfig(ctx,
		append(opts, awscfg.WithCredentialsProvider(aws.NewCredentialsCache(provider)))...,
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("load role credentials: %w", err)
	}
	return roleCfg, nil
}
