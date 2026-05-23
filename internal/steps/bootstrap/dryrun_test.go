package bootstrap

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ffreis/platform-orchestrator/internal/config"
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

type nopRunner struct{}

func (n *nopRunner) Exec(_ context.Context, _ string, _ []string, _ runner.ExecOptions) (runner.ExecResult, error) {
	return runner.ExecResult{}, nil
}

func newExecCtx(cfg configctl.Client, dryRun bool) *pipeline.ExecutionContext {
	return pipeline.NewExecutionContext(pipeline.ExecutionContextOptions{
		Config:    cfg,
		Runner:    &nopRunner{},
		AWSConfig: aws.Config{},
		Log:       logger.Nop(),
		Project:   "platform",
		Env:       "dev",
		DryRun:    dryRun,
	})
}

func TestBootstrapSteps_DryRunPaths(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, true)

	steps := []pipeline.Step{
		NewCreateOrg(cfg),
		NewEnableSCP(cfg),
		NewCreateAdminRole(cfg),
	}

	for _, s := range steps {
		t.Run(s.ID(), func(t *testing.T) {
			if err := s.Run(context.Background(), execCtx); err != nil {
				t.Fatalf("Run() err = %v", err)
			}
		})
	}
}

func TestVerifyRootStep_IsDoneAndRollback(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)

	s := NewVerifyRootStep()
	if s.ID() == "" {
		t.Fatal("expected non-empty ID")
	}
	if s.Name() == "" {
		t.Fatal("expected non-empty Name")
	}

	done, err := s.IsDone(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("IsDone() err = %v", err)
	}
	if done {
		t.Fatal("expected not done before key exists")
	}

	_ = cfg.Set(context.Background(), config.PlatformProject, config.GlobalEnv, config.KeyAccountID, "123")
	done, err = s.IsDone(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("IsDone() err = %v", err)
	}
	if !done {
		t.Fatal("expected done after key exists")
	}

	if err := s.Rollback(context.Background(), execCtx); err == nil {
		// RollbackNotSupported error is expected.
		t.Fatal("expected rollback error")
	}
}

func TestEnableSCP_RollbackNoRootKeyIsNoop(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, false)
	if err := NewEnableSCP(cfg).Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() err = %v", err)
	}
}
