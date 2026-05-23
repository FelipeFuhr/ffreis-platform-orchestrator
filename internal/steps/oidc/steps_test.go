package oidc

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
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

func TestOIDCSteps_DryRunPaths(t *testing.T) {
	cfg := newMemConfig()
	execCtx := newExecCtx(cfg, true)

	steps := []pipeline.Step{
		NewCreateProviderStep(),
		NewCreateGitHubRoleStep(),
	}
	for _, s := range steps {
		t.Run(s.ID(), func(t *testing.T) {
			if err := s.Run(context.Background(), execCtx); err != nil {
				t.Fatalf("Run() err = %v", err)
			}
		})
	}
}

func TestWriteOutputsStep_RunAndRollback(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), platformProject, globalEnv, githubRoleARNKey, "role-arn")
	_ = cfg.Set(context.Background(), platformProject, globalEnv, oidcProviderKey, "provider-arn")
	execCtx := newExecCtx(cfg, false)

	s := NewWriteOutputsStep()
	if err := s.Run(context.Background(), execCtx); err != nil {
		t.Fatalf("Run() err = %v", err)
	}

	gotRole, _ := cfg.Get(context.Background(), execCtx.Project(), execCtx.Env(), outputKeyGitHubRoleARN)
	if gotRole != "role-arn" {
		t.Fatalf("role arn = %q, want %q", gotRole, "role-arn")
	}

	if err := s.Rollback(context.Background(), execCtx); err != nil {
		t.Fatalf("Rollback() err = %v", err)
	}
	_, err := cfg.Get(context.Background(), execCtx.Project(), execCtx.Env(), outputKeyGitHubRoleARN)
	if err == nil {
		t.Fatal("expected key to be deleted")
	}
}

func TestFetchThumbprint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"issuer":"x"}`))
	}))
	defer server.Close()

	// Ensure the client trusts the test server's cert.
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })

	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	fp, err := fetchThumbprint(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchThumbprint() err = %v", err)
	}
	if fp == "" {
		t.Fatal("expected non-empty thumbprint")
	}
}
