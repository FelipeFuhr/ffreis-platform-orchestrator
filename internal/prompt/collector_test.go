package prompt_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// --- in-memory config store ---

type memStore struct {
	mu     sync.Mutex
	data   map[string]string
	gets   int // count of Get calls
	getErr error
	setErr error
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string]string)}
}

func (m *memStore) key(project, env, k string) string { return project + "|" + env + "|" + k }

func (m *memStore) Get(ctx context.Context, project, env, k string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	if m.getErr != nil {
		return "", m.getErr
	}
	v, ok := m.data[m.key(project, env, k)]
	if !ok {
		return "", &configctl.ErrNotFoundError{Key: k}
	}
	return v, nil
}

func (m *memStore) Set(ctx context.Context, project, env, k, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.data[m.key(project, env, k)] = value
	return nil
}

func (m *memStore) Delete(ctx context.Context, project, env, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, m.key(project, env, k))
	return nil
}

func (m *memStore) List(ctx context.Context, project, env string) (map[string]string, error) {
	return nil, nil
}

// --- mock prompter ---

type mockPrompter struct {
	asks   int
	answer string
	askErr error
}

func (p *mockPrompter) Ask(_ context.Context, _ prompt.InputSpec) (string, error) {
	p.asks++
	if p.askErr != nil {
		return "", p.askErr
	}
	return p.answer, nil
}

func (p *mockPrompter) Confirm(_ context.Context, _ string) (bool, error) { return true, nil }
func (p *mockPrompter) Gate(_ context.Context, _, _ string) error         { return nil }

// --- tests ---

// TestCollectExistingValues: all values pre-loaded in mock config, no prompts issued,
// all returned in result.Skipped.
func TestCollectExistingValues(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()
	if err := store.Set(ctx, "proj", "env", "key/a", "val-a"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(ctx, "proj", "env", "key/b", "val-b"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	pr := &mockPrompter{}
	collector := prompt.NewCollector(store, pr, "proj", "env")

	specs := []prompt.InputSpec{
		{Key: "key/a", Label: "A"},
		{Key: "key/b", Label: "B"},
	}

	result, err := collector.Collect(ctx, specs)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if pr.asks != 0 {
		t.Errorf("expected 0 prompts, got %d", pr.asks)
	}
	if len(result.Skipped) != 2 {
		t.Errorf("expected 2 skipped, got %d: %v", len(result.Skipped), result.Skipped)
	}
	if result.Values["key/a"] != "val-a" {
		t.Errorf("expected val-a, got %q", result.Values["key/a"])
	}
	if result.Values["key/b"] != "val-b" {
		t.Errorf("expected val-b, got %q", result.Values["key/b"])
	}
}

// TestCollectMissingValues: one value missing, mock prompter called once,
// value written to config.
func TestCollectMissingValues(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	// Only key/a is pre-loaded; key/b is missing.
	if err := store.Set(ctx, "proj", "env", "key/a", "val-a"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	pr := &mockPrompter{answer: "prompted-value"}
	collector := prompt.NewCollector(store, pr, "proj", "env")

	specs := []prompt.InputSpec{
		{Key: "key/a", Label: "A"},
		{Key: "key/b", Label: "B"},
	}

	result, err := collector.Collect(ctx, specs)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if pr.asks != 1 {
		t.Errorf("expected 1 prompt, got %d", pr.asks)
	}
	if result.Values["key/b"] != "prompted-value" {
		t.Errorf("expected prompted-value for key/b, got %q", result.Values["key/b"])
	}
	if len(result.Collected) != 1 || result.Collected[0] != "key/b" {
		t.Errorf("expected key/b in Collected, got %v", result.Collected)
	}

	// Value should have been written back to config.
	got, err := store.Get(ctx, "proj", "env", "key/b")
	if err != nil {
		t.Fatalf("Get after collect: %v", err)
	}
	if got != "prompted-value" {
		t.Errorf("expected config to have 'prompted-value', got %q", got)
	}
}

// TestCollectInvalidStoredValue: stored value fails validation,
// prompter is called to re-prompt.
func TestCollectInvalidStoredValue(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()

	// Store an invalid value.
	if err := store.Set(ctx, "proj", "env", "validated/key", "bad-value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	pr := &mockPrompter{answer: "good-value"}
	collector := prompt.NewCollector(store, pr, "proj", "env")

	specs := []prompt.InputSpec{
		{
			Key:   "validated/key",
			Label: "Validated",
			Validate: func(v string) error {
				if v == "bad-value" {
					return fmt.Errorf("value is invalid")
				}
				return nil
			},
		},
	}

	result, err := collector.Collect(ctx, specs)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Prompter should have been called once (re-prompt for invalid stored value).
	if pr.asks != 1 {
		t.Errorf("expected 1 prompt for invalid stored value, got %d", pr.asks)
	}
	if result.Values["validated/key"] != "good-value" {
		t.Errorf("expected good-value, got %q", result.Values["validated/key"])
	}
}

func TestCollectOptionalEmptyAndConfigErrors(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	pr := &mockPrompter{answer: ""}
	collector := prompt.NewCollector(store, pr, "proj", "env")

	result, err := collector.Collect(ctx, []prompt.InputSpec{{Key: "optional/key", Optional: true}})
	if err != nil {
		t.Fatalf("Collect optional empty: %v", err)
	}
	if result.Values["optional/key"] != "" {
		t.Fatalf("expected empty optional value, got %q", result.Values["optional/key"])
	}

	store.setErr = fmt.Errorf("set failed")
	pr.answer = "value"
	_, err = collector.Collect(ctx, []prompt.InputSpec{{Key: "write/key"}})
	if err == nil || err.Error() == "" {
		t.Fatal("expected write error")
	}
}

func TestCollectLoadExistingError(t *testing.T) {
	store := newMemStore()
	store.getErr = fmt.Errorf("boom")
	collector := prompt.NewCollector(store, &mockPrompter{answer: "value"}, "proj", "env")

	if _, err := collector.Collect(context.Background(), []prompt.InputSpec{{Key: "k"}}); err == nil {
		t.Fatal("expected load error")
	}
}
