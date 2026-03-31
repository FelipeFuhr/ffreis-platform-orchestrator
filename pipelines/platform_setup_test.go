package pipelines

import (
	"context"
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

type memConfig struct{}

func (m *memConfig) Get(context.Context, string, string, string) (string, error) {
	return "", &configctl.ErrNotFoundError{Key: "x"}
}
func (m *memConfig) Set(context.Context, string, string, string, string) error { return nil }
func (m *memConfig) Delete(context.Context, string, string, string) error      { return nil }
func (m *memConfig) List(context.Context, string, string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestPlatformSetupPipeline_BuildsDAG(t *testing.T) {
	dag, err := PlatformSetupPipeline(&memConfig{})
	if err != nil {
		t.Fatalf("PlatformSetupPipeline() err = %v", err)
	}
	if dag == nil {
		t.Fatal("expected non-nil DAG")
	}
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort() err = %v", err)
	}
	if len(sorted) == 0 {
		t.Fatal("expected steps")
	}
}
