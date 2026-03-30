package pipeline

import (
	"strings"
	"testing"
)

// FuzzDAGTopoSort exercises the DAG's topological sort with fuzz-generated
// step graphs encoded as a compact adjacency string.
//
// Input encoding (space-separated tokens):
//
//	"id:dep1,dep2" — step with given ID and comma-separated dependency IDs
//	"id"           — step with no dependencies
//
// Example: "a b:a c:a,b" produces the diamond a→{b,c}→∅ graph.
//
// Invariants verified:
//   - Add/Validate/TopoSort must never panic
//   - If TopoSort succeeds, each step must appear exactly once in the output
//   - If TopoSort succeeds, all dependencies appear before their dependents
//   - If a cycle exists, TopoSort must return an error (not silently succeed)
func FuzzDAGTopoSort(f *testing.F) {
	// Linear chain: a → b → c
	f.Add("a b:a c:b")
	// Diamond: a → {b,c} → d
	f.Add("a b:a c:a d:b,c")
	// Isolated nodes
	f.Add("x y z")
	// Self-loop (cycle)
	f.Add("a:a")
	// Mutual cycle
	f.Add("a:b b:a")
	// Long chain
	f.Add("s1 s2:s1 s3:s2 s4:s3 s5:s4")
	// Duplicate IDs
	f.Add("a a")
	// Unknown dependency
	f.Add("a:nonexistent")
	// Empty input
	f.Add("")
	// Single node
	f.Add("only")

	f.Fuzz(func(t *testing.T, input string) {
		dag := NewDAG()

		steps := parseAdjacencyEncoding(input)

		// Add all steps; duplicate IDs are expected to fail gracefully.
		for _, s := range steps {
			_ = dag.Add(&testStep{id: s.id, deps: s.deps})
		}

		// Validate and sort — must not panic.
		sorted, err := dag.TopoSort()
		if err != nil {
			// Cycle or other error: acceptable outcome.
			return
		}

		assertTopoSortInvariants(t, dag, sorted)
	})
}

type parsedStep struct {
	id   string
	deps []string
}

func parseAdjacencyEncoding(input string) []parsedStep {
	var steps []parsedStep
	seen := make(map[string]bool)

	for _, token := range strings.Fields(input) {
		id, deps := parseToken(token)
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		steps = append(steps, parsedStep{id: id, deps: deps})
	}

	return steps
}

func parseToken(token string) (string, []string) {
	parts := strings.SplitN(token, ":", 2)
	id := parts[0]
	if id == "" {
		return "", nil
	}
	if len(parts) != 2 || parts[1] == "" {
		return id, nil
	}
	return id, strings.Split(parts[1], ",")
}

func assertTopoSortInvariants(t *testing.T, dag *DAG, sorted []Step) {
	t.Helper()

	if len(sorted) != len(dag.Steps()) {
		t.Fatalf("TopoSort returned %d steps, DAG has %d", len(sorted), len(dag.Steps()))
	}

	positions := stepPositions(t, sorted)
	assertDependenciesBeforeDependents(t, dag, positions)
}

func stepPositions(t *testing.T, sorted []Step) map[string]int {
	t.Helper()

	positions := make(map[string]int, len(sorted))
	for i, step := range sorted {
		if _, dup := positions[step.ID()]; dup {
			t.Fatalf("step %q appears more than once in sorted output", step.ID())
		}
		positions[step.ID()] = i
	}
	return positions
}

func assertDependenciesBeforeDependents(t *testing.T, dag *DAG, positions map[string]int) {
	t.Helper()

	for id, deps := range dag.deps {
		for _, dep := range deps {
			if positions[dep] >= positions[id] {
				t.Fatalf("dependency ordering violated: dep %q (pos %d) must precede %q (pos %d)",
					dep, positions[dep], id, positions[id])
			}
		}
	}
}
