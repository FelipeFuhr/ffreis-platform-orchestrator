package pipeline

import (
	"context"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/internal/runner"
	testconstants "github.com/ffreis/platform-orchestrator/internal/test"
)

const (
	testProject = "test-project"
	testEnv     = "test-env"
)

// --- in-memory configctl mock ---

type memConfigStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemConfigStore() *memConfigStore {
	return &memConfigStore{data: make(map[string]string)}
}

func (m *memConfigStore) storeKey(project, env, k string) string {
	return project + "|" + env + "|" + k
}

func (m *memConfigStore) Get(_ context.Context, project, env, k string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[m.storeKey(project, env, k)]
	if !ok {
		return "", &configctl.ErrNotFoundError{Key: k}
	}
	return v, nil
}

func (m *memConfigStore) Set(_ context.Context, project, env, k, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.storeKey(project, env, k)] = value
	return nil
}

func (m *memConfigStore) Delete(_ context.Context, project, env, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, m.storeKey(project, env, k))
	return nil
}

func (m *memConfigStore) List(_ context.Context, project, env string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prefix := project + "|" + env + "|"
	result := make(map[string]string)
	for k, v := range m.data {
		if strings.HasPrefix(k, prefix) {
			result[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return result, nil
}

// --- mock credential resolver that tracks classes resolved ---

type classTrackingResolver struct {
	mu       sync.Mutex
	resolved []credential.Class
}

func newClassTrackingResolver() *classTrackingResolver {
	return &classTrackingResolver{}
}

func (r *classTrackingResolver) Resolve(_ context.Context, class credential.Class) (aws.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved = append(r.resolved, class)
	return aws.Config{}, nil
}

// --- mock runner ---

type mockRunner struct{}

func (r *mockRunner) Exec(_ string, _ []string, _ runner.ExecOptions) (runner.ExecResult, error) {
	return runner.ExecResult{}, nil
}

// --- stub step ---

type stubStep struct {
	id     string
	deps   []string
	class  credential.Class
	runErr error
	runCnt int
	done   bool
}

func (s *stubStep) ID() string                                                  { return s.id }
func (s *stubStep) Name() string                                                { return s.id }
func (s *stubStep) Deps() []string                                              { return s.deps }
func (s *stubStep) CredentialClass() credential.Class                           { return s.class }
func (s *stubStep) RequiredInputs() []prompt.InputSpec                          { return nil }
func (s *stubStep) RetryPolicy() RetryPolicy                                    { return NoRetry }
func (s *stubStep) IsDone(_ context.Context, _ *ExecutionContext) (bool, error) { return s.done, nil }
func (s *stubStep) Run(_ context.Context, _ *ExecutionContext) error            { s.runCnt++; return s.runErr }
func (s *stubStep) Rollback(_ context.Context, _ *ExecutionContext) error {
	return ErrRollbackNotSupported(s.id)
}

// stubWriteARNStep writes orchestrator/admin_role_arn to the store during Run
// to simulate a bootstrap step that makes the ARN available to subsequent steps.
type stubWriteARNStep struct {
	id    string
	class credential.Class
	store configctl.Client
}

func (s *stubWriteARNStep) ID() string                         { return s.id }
func (s *stubWriteARNStep) Name() string                       { return s.id }
func (s *stubWriteARNStep) Deps() []string                     { return nil }
func (s *stubWriteARNStep) CredentialClass() credential.Class  { return s.class }
func (s *stubWriteARNStep) RequiredInputs() []prompt.InputSpec { return nil }
func (s *stubWriteARNStep) RetryPolicy() RetryPolicy           { return NoRetry }
func (s *stubWriteARNStep) IsDone(_ context.Context, _ *ExecutionContext) (bool, error) {
	return false, nil
}
func (s *stubWriteARNStep) Run(ctx context.Context, _ *ExecutionContext) error {
	return s.store.Set(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN, testconstants.AdminRoleARNPlatformAdmin)
}
func (s *stubWriteARNStep) Rollback(_ context.Context, _ *ExecutionContext) error {
	return ErrRollbackNotSupported(s.id)
}

// --- helpers ---

func buildTestEngine(dag *DAG, store *memConfigStore) *Engine {
	state := NewStateStore(store)
	resolver := newClassTrackingResolver()
	r := &mockRunner{}
	log := logger.Nop()
	return NewEngine(EngineOptions{
		DAG:      dag,
		State:    state,
		Resolver: resolver,
		Config:   store,
		Runner:   r,
		Log:      log,
		Project:  testProject,
		Env:      testEnv,
		DryRun:   false,
	})
}

func buildTestEngineWithResolver(dag *DAG, store *memConfigStore, resolver credential.Resolver) *Engine {
	state := NewStateStore(store)
	r := &mockRunner{}
	log := logger.Nop()
	return NewEngine(EngineOptions{
		DAG:      dag,
		State:    state,
		Resolver: resolver,
		Config:   store,
		Runner:   r,
		Log:      log,
		Project:  testProject,
		Env:      testEnv,
		DryRun:   false,
	})
}
