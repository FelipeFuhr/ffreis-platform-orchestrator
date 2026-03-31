package prompt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
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
	out := make(map[string]string)
	for k, v := range m.data {
		prefix := project + "|" + env + "|"
		if strings.HasPrefix(k, prefix) {
			out[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return out, nil
}

func TestBatchPrompter_Ask_OptionalMissingReturnsDefault(t *testing.T) {
	cfg := newMemConfig()
	b := NewBatchPrompter(nil, cfg, "p", "e", func(string, ...any) {})

	val, err := b.Ask(InputSpec{
		Key:      "k",
		Optional: true,
		Default:  "d",
		Validate: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("Ask() err = %v", err)
	}
	if val != "d" {
		t.Fatalf("Ask() = %q, want %q", val, "d")
	}
}

func TestBatchPrompter_Ask_RequiredMissingErrors(t *testing.T) {
	cfg := newMemConfig()
	b := NewBatchPrompter(context.Background(), cfg, "p", "e", func(string, ...any) {})

	_, err := b.Ask(InputSpec{Key: "k", Optional: false, Validate: func(string) error { return nil }})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "non-interactive mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchPrompter_Ask_VerifyFailureErrors(t *testing.T) {
	cfg := newMemConfig()
	_ = cfg.Set(context.Background(), "p", "e", "k", "bad")
	b := NewBatchPrompter(context.Background(), cfg, "p", "e", func(string, ...any) {})

	_, err := b.Ask(InputSpec{
		Key:      "k",
		Validate: func(string) error { return errors.New("nope") },
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchPrompter_ConfirmAndGate(t *testing.T) {
	cfg := newMemConfig()
	logged := []string{}
	b := NewBatchPrompter(context.Background(), cfg, "p", "e", func(f string, a ...any) {
		logged = append(logged, f)
	})

	ok, err := b.Confirm("hello")
	if err != nil {
		t.Fatalf("Confirm() err = %v", err)
	}
	if !ok {
		t.Fatal("Confirm() = false, want true")
	}
	if err := b.Gate("hello", "YES"); err != nil {
		t.Fatalf("Gate() err = %v", err)
	}
	if len(logged) == 0 {
		t.Fatal("expected log output")
	}
}
