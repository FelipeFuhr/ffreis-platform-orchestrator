package credential_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	testconstants "github.com/ffreis/platform-orchestrator/internal/test"
)

const (
	testRegion = testconstants.RegionUSEast1
	testRunID  = "test-run"
)

// --- in-memory configctl mock ---

type mockCfg struct {
	mu       sync.Mutex
	data     map[string]string
	getCalls int
}

func newMockCfg() *mockCfg {
	return &mockCfg{data: make(map[string]string)}
}

func (m *mockCfg) key(project, env, k string) string { return project + "|" + env + "|" + k }

func (m *mockCfg) Get(ctx context.Context, project, env, k string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	v, ok := m.data[m.key(project, env, k)]
	if !ok {
		return "", &configctl.ErrNotFoundError{Key: k}
	}
	return v, nil
}

func (m *mockCfg) Set(ctx context.Context, project, env, k, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.key(project, env, k)] = value
	return nil
}

func (m *mockCfg) Delete(ctx context.Context, project, env, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, m.key(project, env, k))
	return nil
}

func (m *mockCfg) List(ctx context.Context, project, env string) (map[string]string, error) {
	return nil, nil
}

// --- tests ---

// TestResolveAdminBeforeBootstrap: orchestrator/admin_role_arn absent →
// Resolve(ClassAdmin) returns error containing "not found".
func TestResolveAdminBeforeBootstrap(t *testing.T) {
	store := newMockCfg()
	resolver := credential.NewAWSResolver(context.Background(), testRegion, testRunID, store)

	_, err := resolver.Resolve(credential.ClassAdmin)
	if err == nil {
		t.Fatal("expected error when admin_role_arn is not set, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected error to contain 'not found', got: %v", err)
	}
}

// TestResolveAdminAfterBootstrap: write orchestrator/admin_role_arn to mock store →
// Resolve(ClassAdmin) attempts STS AssumeRole (expected to fail in unit test),
// but the error is NOT "not found" (it's an STS error instead).
func TestResolveAdminAfterBootstrap(t *testing.T) {
	store := newMockCfg()
	ctx := context.Background()

	// Simulate bootstrap having written the admin role ARN.
	if err := store.Set(ctx, platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.KeyAdminRoleARN, testconstants.AdminRoleARNPlatformAdmin); err != nil {
		t.Fatalf("Set: %v", err)
	}

	resolver := credential.NewAWSResolver(ctx, testRegion, testRunID, store)

	_, err := resolver.Resolve(credential.ClassAdmin)
	// In a unit test without real AWS credentials, this will fail at the
	// credential-loading or STS AssumeRole stage — but NOT with "not found".
	// The important thing is that we got past the configctl lookup.
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			t.Errorf("error should NOT be 'not found' when ARN is set; got: %v", err)
		}
		// STS error or credential error is acceptable here.
		t.Logf("got expected non-notfound error: %v", err)
		return
	}
	// If no error, credentials resolved successfully (unlikely in unit test but OK).
	t.Log("credentials resolved successfully (no real AWS call expected)")
}

// TestResolveRootDoesNotUseStore: Resolve(ClassRoot) does not read from the
// mock configctl client at all (call count stays 0).
func TestResolveRootDoesNotUseStore(t *testing.T) {
	store := newMockCfg()
	resolver := credential.NewAWSResolver(context.Background(), testRegion, testRunID, store)

	// Root resolution may fail (no real AWS env), but the important thing is
	// that it never calls the configctl store.
	_, _ = resolver.Resolve(credential.ClassRoot)

	store.mu.Lock()
	calls := store.getCalls
	store.mu.Unlock()

	if calls != 0 {
		t.Errorf("expected 0 configctl Get calls for ClassRoot, got %d", calls)
	}
}
