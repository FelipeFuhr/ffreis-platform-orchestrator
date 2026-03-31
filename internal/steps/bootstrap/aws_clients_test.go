package bootstrap

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestAWSClientFactories(t *testing.T) {
	if newOrganizationsClient(aws.Config{}) == nil {
		t.Fatal("expected organizations client")
	}
	if newIAMClient(aws.Config{}) == nil {
		t.Fatal("expected iam client")
	}
	if newSTSClient(aws.Config{}) == nil {
		t.Fatal("expected sts client")
	}
}
