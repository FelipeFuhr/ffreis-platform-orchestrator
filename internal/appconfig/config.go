package appconfig

import (
	"errors"
	"os"
)

// Config holds all runtime configuration sourced from environment variables.
// CLI flags may override individual fields after Load() returns.
type Config struct {
	// DynamoDB
	TableName string
	// AWS
	AWSRegion string
	// Logging
	LogLevel string
	// Execution
	NonInteractive bool
	ConfirmAll     bool
	DryRun         bool
}

// ErrMissingTable is returned when ORCHESTRATOR_TABLE is unset and no --table flag is provided.
var ErrMissingTable = errors.New("ORCHESTRATOR_TABLE environment variable is required")

// Load resolves Config from environment variables.
func Load() (*Config, error) {
	table := os.Getenv("ORCHESTRATOR_TABLE")
	if table == "" {
		return nil, ErrMissingTable
	}

	region := os.Getenv("AWS_DEFAULT_REGION")
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}

	level := os.Getenv("ORCHESTRATOR_LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	return &Config{
		TableName: table,
		AWSRegion: region,
		LogLevel:  level,
	}, nil
}
