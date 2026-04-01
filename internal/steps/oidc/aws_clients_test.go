package oidc

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestAWSClientFactoriesAndHTTPHelpers(t *testing.T) {
	if newIAMClient(aws.Config{}) == nil {
		t.Fatal("expected iam client")
	}
	if newSTSClient(aws.Config{}) == nil {
		t.Fatal("expected sts client")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	if githubThumbprintFn == nil || httpDo == nil {
		t.Fatal("expected helper funcs to be initialized")
	}
	_ = req
}
