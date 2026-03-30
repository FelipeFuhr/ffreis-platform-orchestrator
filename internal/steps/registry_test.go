package steps_test

import (
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
	"github.com/ffreis/platform-orchestrator/internal/steps"
)

// minimalStep is the smallest valid Step implementation for testing the registry.
type minimalStep struct {
	id   string
	deps []string
}

func (s *minimalStep) ID() string                                          { return s.id }
func (s *minimalStep) Name() string                                        { return s.id }
func (s *minimalStep) Deps() []string                                      { return s.deps }
func (s *minimalStep) CredentialClass() credential.Class                   { return credential.ClassRoot }
func (s *minimalStep) RequiredInputs() []prompt.InputSpec                  { return nil }
func (s *minimalStep) RetryPolicy() pipeline.RetryPolicy                   { return pipeline.NoRetry }
func (s *minimalStep) IsDone(ctx *pipeline.ExecutionContext) (bool, error) { return false, nil }
func (s *minimalStep) Run(ctx *pipeline.ExecutionContext) error            { return nil }
func (s *minimalStep) Rollback(ctx *pipeline.ExecutionContext) error {
	return pipeline.ErrRollbackNotSupported(s.id)
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := steps.NewRegistry()
	s := &minimalStep{id: "alpha"}
	if err := reg.Register(s); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := reg.Get("alpha")
	if !ok {
		t.Fatal("Get returned false for registered step")
	}
	if got.ID() != "alpha" {
		t.Errorf("expected ID alpha, got %q", got.ID())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := steps.NewRegistry()
	_, ok := reg.Get("missing")
	if ok {
		t.Fatal("expected false for missing step, got true")
	}
}

func TestRegistry_Register_DuplicateReturnsError(t *testing.T) {
	reg := steps.NewRegistry()
	if err := reg.Register(&minimalStep{id: "dup"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register(&minimalStep{id: "dup"}); err == nil {
		t.Error("expected error on duplicate registration, got nil")
	}
}

func TestRegistry_All(t *testing.T) {
	reg := steps.NewRegistry()
	for _, id := range []string{"x", "y", "z"} {
		if err := reg.Register(&minimalStep{id: id}); err != nil {
			t.Fatalf("Register %q: %v", id, err)
		}
	}
	all := reg.All()
	if len(all) != 3 {
		t.Errorf("expected 3 steps, got %d", len(all))
	}
}

func TestRegistry_BuildDAG_ValidGraph(t *testing.T) {
	reg := steps.NewRegistry()
	if err := reg.Register(&minimalStep{id: "a"}); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	if err := reg.Register(&minimalStep{id: "b", deps: []string{"a"}}); err != nil {
		t.Fatalf("Register b: %v", err)
	}

	dag, err := reg.BuildDAG()
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort: %v", err)
	}
	if len(sorted) != 2 {
		t.Errorf("expected 2 steps, got %d", len(sorted))
	}
	if sorted[0].ID() != "a" || sorted[1].ID() != "b" {
		t.Errorf("unexpected order: %v, %v", sorted[0].ID(), sorted[1].ID())
	}
}

func TestRegistry_BuildDAG_UnknownDependencyReturnsError(t *testing.T) {
	reg := steps.NewRegistry()
	if err := reg.Register(&minimalStep{id: "orphan", deps: []string{"does_not_exist"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := reg.BuildDAG()
	if err == nil {
		t.Fatal("expected error for unknown dependency, got nil")
	}
}

func TestRegistry_BuildDAG_CycleReturnsError(t *testing.T) {
	reg := steps.NewRegistry()
	if err := reg.Register(&minimalStep{id: "x", deps: []string{"y"}}); err != nil {
		t.Fatalf("Register x: %v", err)
	}
	if err := reg.Register(&minimalStep{id: "y", deps: []string{"x"}}); err != nil {
		t.Fatalf("Register y: %v", err)
	}

	_, err := reg.BuildDAG()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}
