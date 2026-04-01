package terraform

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// awsEnvFromConfig extracts AWS credentials from an aws.Config and returns
// them as environment variables safe to inject into a child process.
// The process environment must NOT be inherited from the shell.
func awsEnvFromConfig(ctx context.Context, cfg aws.Config) (map[string]string, error) {
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieve credentials: %w", err)
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
	}
	if creds.SessionToken != "" {
		env["AWS_SESSION_TOKEN"] = creds.SessionToken
	}
	if cfg.Region != "" {
		env["AWS_DEFAULT_REGION"] = cfg.Region
	}
	return env, nil
}
