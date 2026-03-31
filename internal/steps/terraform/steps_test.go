package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

type memConfig struct {
	data map[string]string
}

func newMemConfig() *memConfig { return &memConfig{data: make(map[string]string)} }

func (m *memConfig) key(project, env, k string) string { return project + "|" + env + "|" + k }

func (m *memConfig) Get(_ context.Context, project, env, key string) (string, error) {
	v, ok := m.data[m.key(project, env, key)]
	if !ok {
		return "", &configctl.ErrNotFoundError{Key: key}
	}
	return v, nil
}
func (m *memConfig) Set(_ context.Context, project, env, key, value string) error {
	m.data[m.key(project, env, key)] = value
	return nil
}
func (m *memConfig) Delete(_ context.Context, project, env, key string) error {
	delete(m.data, m.key(project, env, key))
	return nil
}
func (m *memConfig) List(_ context.Context, project, env string) (map[string]string, error) {
	return map[string]string{}, nil
}

type staticCredsProvider struct {
	creds aws.Credentials
}

func (p staticCredsProvider) Retrieve(_ context.Context) (aws.Credentials, error) {
	return p.creds, nil
}
func (p staticCredsProvider) IsExpired() bool { return false }

type recordingRunner struct {
	lastCmd  string
	lastArgs []string
	lastOpts runner.ExecOptions

	result runner.ExecResult
	err    error
}

func (r *recordingRunner) Exec(cmd string, args []string, opts runner.ExecOptions) (runner.ExecResult, error) {
	r.lastCmd = cmd
	r.lastArgs = append([]string(nil), args...)
	r.lastOpts = opts
	return r.result, r.err
}

func newExecCtx(t *testing.T, cfg configctl.Client, run runner.Runner, dryRun bool) *pipeline.ExecutionContext {
	t.Helper()
	awsCfg := aws.Config{
		Region:      "us-east-1",
		Credentials: staticCredsProvider{creds: aws.Credentials{AccessKeyID: "a", SecretAccessKey: "b", SessionToken: "c"}},
	}
	return pipeline.NewExecutionContext(pipeline.ExecutionContextOptions{
		Config:    cfg,
		Runner:    run,
		AWSConfig: awsCfg,
		Log:       logger.Nop(),
		Project:   "platform",
		Env:       "dev",
		DryRun:    dryRun,
	})
}

func TestAwsEnvFromConfig(t *testing.T) {
	awsCfg := aws.Config{
		Region:      "us-west-2",
		Credentials: staticCredsProvider{creds: aws.Credentials{AccessKeyID: "a", SecretAccessKey: "b", SessionToken: "c"}},
	}
	env, err := awsEnvFromConfig(context.Background(), awsCfg)
	if err != nil {
		t.Fatalf("awsEnvFromConfig() err = %v", err)
	}
	if env["AWS_ACCESS_KEY_ID"] != "a" || env["AWS_SECRET_ACCESS_KEY"] != "b" || env["AWS_SESSION_TOKEN"] != "c" {
		t.Fatalf("unexpected env: %#v", env)
	}
	if env["AWS_DEFAULT_REGION"] != "us-west-2" {
		t.Fatalf("AWS_DEFAULT_REGION = %q, want %q", env["AWS_DEFAULT_REGION"], "us-west-2")
	}
}

func TestInitStep_DryRun(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, "/tmp/module")
	r := &recordingRunner{}
	execCtx := newExecCtx(t, cfg, r, true)

	err := NewInitStep().Run(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if r.lastCmd != "" {
		t.Fatalf("expected runner not to be called in dry-run")
	}
}

func TestPlanStep_ExitCode2IsChanges(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)

	r := &recordingRunner{
		result: runner.ExecResult{ExitCode: 2},
		err:    errors.New("exit 2"),
	}
	execCtx := newExecCtx(t, cfg, r, false)

	err := NewPlanStep().Run(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if got := execCtx.Outputs()["plan_has_changes"]; got != "true" {
		t.Fatalf("plan_has_changes = %q, want %q", got, "true")
	}
}

func TestPlanStep_RollbackRemovesPlanFile(t *testing.T) {
	cfg := newMemConfig()
	modulePath := t.TempDir()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, modulePath)

	planFile := filepath.Join(modulePath, "org.tfplan")
	if err := os.WriteFile(planFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	execCtx := newExecCtx(t, cfg, &recordingRunner{}, false)
	if err := NewPlanStep().Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() err = %v", err)
	}
	if _, err := os.Stat(planFile); err == nil {
		t.Fatalf("expected plan file to be removed")
	}
}

func TestApplyStep_PlanHasNoChangesSkips(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), "platform", "dev", configKeyOrgModulePath, "/tmp/module")
	_ = cfg.Set(context.Background(), "platform", "dev", "orchestrator/step/terraform_plan_org/output/plan_has_changes", "false")

	r := &recordingRunner{}
	execCtx := newExecCtx(t, cfg, r, false)

	err := NewApplyStep().Run(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if got := execCtx.Outputs()["applied"]; got != "false" {
		t.Fatalf("applied = %q, want %q", got, "false")
	}
	if r.lastCmd != "" {
		t.Fatalf("expected runner not to be called when no changes")
	}
}
