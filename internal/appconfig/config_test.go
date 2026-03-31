package appconfig

import (
	"testing"
)

func TestLoad_MissingTable(t *testing.T) {
	t.Setenv("ORCHESTRATOR_TABLE", "")
	_, err := Load()
	if err != ErrMissingTable {
		t.Fatalf("Load() err = %v, want %v", err, ErrMissingTable)
	}
}

func TestLoad_UsesRegionAndDefaultLevel(t *testing.T) {
	t.Setenv("ORCHESTRATOR_TABLE", "tbl")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("ORCHESTRATOR_LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if cfg.TableName != "tbl" {
		t.Fatalf("TableName = %q, want %q", cfg.TableName, "tbl")
	}
	if cfg.AWSRegion != "us-east-1" {
		t.Fatalf("AWSRegion = %q, want %q", cfg.AWSRegion, "us-east-1")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_PrefersAWSDefaultRegion(t *testing.T) {
	t.Setenv("ORCHESTRATOR_TABLE", "tbl")
	t.Setenv("AWS_DEFAULT_REGION", "sa-east-1")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("ORCHESTRATOR_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if cfg.AWSRegion != "sa-east-1" {
		t.Fatalf("AWSRegion = %q, want %q", cfg.AWSRegion, "sa-east-1")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
