package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestNewAWSResolver(t *testing.T) {
	resolver := NewAWSResolver("us-east-1", "run-1", nil)
	if resolver.region != "us-east-1" || resolver.runID != "run-1" {
		t.Fatalf("unexpected resolver: %+v", resolver)
	}
}

func TestResolve_UnknownClass(t *testing.T) {
	resolver := NewAWSResolver("us-east-1", "run-1", nil)
	_, err := resolver.Resolve(context.Background(), Class(99))
	if err == nil || err.Error() != "unknown credential class: unknown" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolve_RootLoadError(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(context.Context, ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("boom")
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	resolver := NewAWSResolver("us-east-1", "run-1", nil)
	_, err := resolver.Resolve(context.Background(), ClassRoot)
	if err == nil || err.Error() != "load root credentials: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssumeRole_BaseLoadError(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(context.Context, ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("base")
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	resolver := NewAWSResolver("us-east-1", "run-1", nil)
	_, err := resolver.assumeRole(context.Background(), "arn:aws:iam::123:role/test", "session", nil)
	if err == nil || err.Error() != "load base credentials for role assumption: base" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssumeRole_RoleLoadError(t *testing.T) {
	origLoad := loadDefaultAWSConfig
	origSTS := newSTSClient
	origAssume := newAssumeRoleProvider
	callCount := 0

	loadDefaultAWSConfig = func(context.Context, ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		callCount++
		if callCount == 1 {
			return aws.Config{}, nil
		}
		return aws.Config{}, errors.New("role")
	}
	newSTSClient = func(cfg aws.Config, _ ...func(*sts.Options)) *sts.Client { return &sts.Client{} }
	newAssumeRoleProvider = func(client stscreds.AssumeRoleAPIClient, roleARN string, optFns ...func(*stscreds.AssumeRoleOptions)) *stscreds.AssumeRoleProvider {
		return &stscreds.AssumeRoleProvider{}
	}
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSTSClient = origSTS
		newAssumeRoleProvider = origAssume
	})

	resolver := NewAWSResolver("us-east-1", "run-1", nil)
	_, err := resolver.assumeRole(context.Background(), "arn:aws:iam::123:role/test", "session", nil)
	if err == nil || err.Error() != "load role credentials: role" {
		t.Fatalf("unexpected error: %v", err)
	}
}
