package pipeline

import (
	"fmt"
	"sort"
)

// DAG builds a directed acyclic graph of Steps and performs topological sorting.
type DAG struct {
	steps map[string]Step
	// adjacency list: step ID → list of dependency IDs
	deps map[string][]string
}

// NewDAG constructs an empty DAG.
func NewDAG() *DAG {
	return &DAG{
		steps: make(map[string]Step),
		deps:  make(map[string][]string),
	}
}

// Add registers a step. Returns an error if a step with the same ID is
// already registered.
func (d *DAG) Add(s Step) error {
	if _, exists := d.steps[s.ID()]; exists {
		return fmt.Errorf("duplicate step ID: %q", s.ID())
	}
	d.steps[s.ID()] = s
	d.deps[s.ID()] = s.Deps()
	return nil
}

// Validate checks that all declared dependency IDs exist and that the graph
// is acyclic.
func (d *DAG) Validate() error {
	for id, deps := range d.deps {
		for _, dep := range deps {
			if _, ok := d.steps[dep]; !ok {
				return fmt.Errorf("step %q declares unknown dependency %q", id, dep)
			}
		}
	}
	_, err := d.TopoSort()
	return err
}

// TopoSort returns the steps in a topological order (dependencies before
// dependents). Returns ErrCycle if the graph contains a cycle.
func (d *DAG) TopoSort() ([]Step, error) {
	// Kahn's algorithm.
	inDegree := d.computeInDegree()
	dependents := d.computeDependents()

	// Queue starts with steps that have no dependencies.
	queue := zeroInDegreeQueue(inDegree)

	var sorted []Step
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, d.steps[id])

		next := dependents[id]
		sortStrings(next)
		for _, dep := range next {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sortStrings(queue)
			}
		}
	}

	if len(sorted) != len(d.steps) {
		return nil, fmt.Errorf("cycle detected in step dependency graph")
	}
	return sorted, nil
}

// Steps returns all registered steps as a map (ID → Step).
func (d *DAG) Steps() map[string]Step { return d.steps }

func sortStrings(s []string) {
	sort.Strings(s)
}

func (d *DAG) computeInDegree() map[string]int {
	inDegree := make(map[string]int, len(d.steps))
	for id := range d.steps {
		inDegree[id] = len(d.deps[id])
	}
	return inDegree
}

func (d *DAG) computeDependents() map[string][]string {
	// dependents maps: step ID -> list of steps that depend on it.
	dependents := make(map[string][]string)
	for id, deps := range d.deps {
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], id)
		}
	}
	return dependents
}

func zeroInDegreeQueue(inDegree map[string]int) []string {
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sortStrings(queue)
	return queue
}
