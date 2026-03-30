package steps

import (
	"fmt"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

// Registry holds all Step implementations, keyed by ID.
type Registry struct {
	steps map[string]pipeline.Step
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{steps: make(map[string]pipeline.Step)}
}

// Register adds a step to the registry. Returns an error on duplicate ID.
func (r *Registry) Register(s pipeline.Step) error {
	if _, exists := r.steps[s.ID()]; exists {
		return fmt.Errorf("duplicate step ID: %q", s.ID())
	}
	r.steps[s.ID()] = s
	return nil
}

// Get returns the step with the given ID and true, or zero value and false if not found.
func (r *Registry) Get(id string) (pipeline.Step, bool) {
	s, ok := r.steps[id]
	return s, ok
}

// All returns all registered steps.
func (r *Registry) All() []pipeline.Step {
	out := make([]pipeline.Step, 0, len(r.steps))
	for _, s := range r.steps {
		out = append(out, s)
	}
	return out
}

// BuildDAG registers all steps into a DAG and validates it.
func (r *Registry) BuildDAG() (*pipeline.DAG, error) {
	dag := pipeline.NewDAG()
	for _, s := range r.steps {
		if err := dag.Add(s); err != nil {
			return nil, err
		}
	}
	if err := dag.Validate(); err != nil {
		return nil, err
	}
	return dag, nil
}
