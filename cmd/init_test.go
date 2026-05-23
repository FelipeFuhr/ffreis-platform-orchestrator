package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ffreis/platform-orchestrator/internal/appconfig"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/internal/runner"
	"github.com/ffreis/platform-orchestrator/internal/steps"
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
	result := make(map[string]string)
	prefix := m.key(project, env, "")
	for key, value := range m.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			result[key[len(prefix):]] = value
		}
	}
	return result, nil
}

type stubStep struct {
	id    string
	specs []prompt.InputSpec
}

func (s *stubStep) ID() string                         { return s.id }
func (s *stubStep) Name() string                       { return s.id }
func (s *stubStep) Deps() []string                     { return nil }
func (s *stubStep) CredentialClass() credential.Class  { return credential.ClassOperator }
func (s *stubStep) RequiredInputs() []prompt.InputSpec { return s.specs }
func (s *stubStep) RetryPolicy() pipeline.RetryPolicy  { return pipeline.NoRetry }
func (s *stubStep) IsDone(context.Context, *pipeline.ExecutionContext) (bool, error) {
	return false, nil
}
func (s *stubStep) Run(context.Context, *pipeline.ExecutionContext) error { return nil }
func (s *stubStep) Rollback(context.Context, *pipeline.ExecutionContext) error {
	return nil
}

type nopRunner struct{}

func (n *nopRunner) Exec(context.Context, string, []string, runner.ExecOptions) (runner.ExecResult, error) {
	return runner.ExecResult{}, nil
}

func TestCollectInputs_NonInteractiveReadsFromConfig(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), "platform", "dev", "input/key", "value")

	reg := steps.NewRegistry()
	if err := reg.Register(&stubStep{id: "s1", specs: []prompt.InputSpec{{Key: "input/key", Label: "X"}}}); err != nil {
		t.Fatalf("Register() err = %v", err)
	}
	dag, err := reg.BuildDAG()
	if err != nil {
		t.Fatalf("BuildDAG() err = %v", err)
	}

	d := &deps{
		cfg:    &appconfig.Config{NonInteractive: true},
		log:    logger.Nop(),
		cfgctl: cfg,
	}
	gf := &globalFlags{project: "platform", env: "dev"}

	if err := collectInputs(context.Background(), nil, d, gf, dag); err != nil {
		t.Fatalf("collectInputs() err = %v", err)
	}
}

func TestBuildEngine_DoesNotPanic(t *testing.T) {
	d := &deps{
		cfg:    &appconfig.Config{AWSRegion: "us-east-1"},
		log:    logger.Nop(),
		cfgctl: newMemConfig(),
	}
	gf := &globalFlags{project: "platform", env: "dev", dryRun: true}

	eng := buildEngine(context.Background(), d, gf, pipeline.NewDAG(), "runid", io.Discard)
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}

	// Minimal smoke check that we can create an execution context.
	_ = pipeline.NewExecutionContext(pipeline.ExecutionContextOptions{
		Config:    d.cfgctl,
		Runner:    &nopRunner{},
		AWSConfig: aws.Config{},
		Log:       d.log,
		Project:   gf.project,
		Env:       gf.env,
		DryRun:    true,
	})
}
