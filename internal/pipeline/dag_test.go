package pipeline

import (
	"testing"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// testStep is a minimal Step implementation used only in tests.
type testStep struct {
	id    string
	name  string
	deps  []string
	class credential.Class
}

func (s *testStep) ID() string                                 { return s.id }
func (s *testStep) Name() string                               { return s.name }
func (s *testStep) Deps() []string                             { return s.deps }
func (s *testStep) CredentialClass() credential.Class          { return s.class }
func (s *testStep) RequiredInputs() []prompt.InputSpec         { return nil }
func (s *testStep) RetryPolicy() RetryPolicy                   { return NoRetry }
func (s *testStep) IsDone(ctx *ExecutionContext) (bool, error) { return false, nil }
func (s *testStep) Run(ctx *ExecutionContext) error            { return nil }
func (s *testStep) Rollback(ctx *ExecutionContext) error       { return ErrRollbackNotSupported(s.id) }

func TestDAG_TopoSort_LinearChain(t *testing.T) {
	// a → b → c
	dag := NewDAG()
	steps := []*testStep{
		{id: "a", deps: nil},
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"b"}},
	}
	for _, s := range steps {
		if err := dag.Add(s); err != nil {
			t.Fatalf("Add(%q): %v", s.id, err)
		}
	}
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(sorted))
	}
	// Verify order: a must come before b, b before c.
	pos := make(map[string]int)
	for i, step := range sorted {
		pos[step.ID()] = i
	}
	if pos["a"] >= pos["b"] {
		t.Errorf("expected a before b, got a=%d b=%d", pos["a"], pos["b"])
	}
	if pos["b"] >= pos["c"] {
		t.Errorf("expected b before c, got b=%d c=%d", pos["b"], pos["c"])
	}
}

func TestDAG_TopoSort_Diamond(t *testing.T) {
	//   a
	//  / \
	// b   c
	//  \ /
	//   d
	dag := NewDAG()
	steps := []*testStep{
		{id: "a", deps: nil},
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"a"}},
		{id: "d", deps: []string{"b", "c"}},
	}
	for _, s := range steps {
		if err := dag.Add(s); err != nil {
			t.Fatalf("Add(%q): %v", s.id, err)
		}
	}
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort: %v", err)
	}
	if len(sorted) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(sorted))
	}
	pos := make(map[string]int)
	for i, step := range sorted {
		pos[step.ID()] = i
	}
	if pos["a"] >= pos["b"] || pos["a"] >= pos["c"] {
		t.Errorf("a must come before b and c")
	}
	if pos["b"] >= pos["d"] || pos["c"] >= pos["d"] {
		t.Errorf("b and c must come before d")
	}
}

func TestDAG_TopoSort_CycleDetection(t *testing.T) {
	dag := NewDAG()
	// x → y → x (cycle)
	steps := []*testStep{
		{id: "x", deps: []string{"y"}},
		{id: "y", deps: []string{"x"}},
	}
	for _, s := range steps {
		if err := dag.Add(s); err != nil {
			t.Fatalf("Add(%q): %v", s.id, err)
		}
	}
	_, err := dag.TopoSort()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestDAG_Validate_UnknownDependency(t *testing.T) {
	dag := NewDAG()
	if err := dag.Add(&testStep{id: "a", deps: []string{"nonexistent"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := dag.Validate(); err == nil {
		t.Fatal("expected unknown dependency error, got nil")
	}
}

func TestDAG_Add_DuplicateID(t *testing.T) {
	dag := NewDAG()
	s := &testStep{id: "dup"}
	if err := dag.Add(s); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := dag.Add(s); err == nil {
		t.Fatal("expected error on duplicate ID, got nil")
	}
}

func TestDAG_TopoSort_EmptyGraph(t *testing.T) {
	dag := NewDAG()
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort on empty DAG: %v", err)
	}
	if len(sorted) != 0 {
		t.Fatalf("expected 0 steps, got %d", len(sorted))
	}
}

func TestDAG_TopoSort_IsDeterministic(t *testing.T) {
	// Steps with no dependencies should sort lexicographically.
	dag := NewDAG()
	for _, id := range []string{"z", "a", "m", "b"} {
		if err := dag.Add(&testStep{id: id}); err != nil {
			t.Fatalf("Add(%q): %v", id, err)
		}
	}
	sorted, err := dag.TopoSort()
	if err != nil {
		t.Fatalf("TopoSort: %v", err)
	}
	expected := []string{"a", "b", "m", "z"}
	for i, step := range sorted {
		if step.ID() != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], step.ID())
		}
	}
}
