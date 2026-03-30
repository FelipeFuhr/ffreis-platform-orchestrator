package prompt

import (
	"context"
	"fmt"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

// BatchPrompter satisfies Prompter in non-interactive mode.
// It reads all values from configctl and fails immediately if any required
// value is missing. Confirmation gates are auto-confirmed and logged.
type BatchPrompter struct {
	cfg     configctl.Client
	project string
	env     string
	logf    func(string, ...any)
}

// NewBatchPrompter constructs a non-interactive prompter.
func NewBatchPrompter(ctx context.Context, cfg configctl.Client, project, env string, logf func(string, ...any)) *BatchPrompter {
	return &BatchPrompter{cfg: cfg, project: project, env: env, logf: logf}
}

func (b *BatchPrompter) Ask(spec InputSpec) (string, error) {
	ctx := context.Background()
	val, err := b.cfg.Get(ctx, b.project, b.env, spec.Key)
	if err != nil {
		if configctl.IsNotFound(err) && spec.Optional {
			return spec.Default, nil
		}
		if configctl.IsNotFound(err) {
			return "", fmt.Errorf("non-interactive mode: required key %q is not set in configctl", spec.Key)
		}
		return "", fmt.Errorf("read key %q: %w", spec.Key, err)
	}
	if err := spec.Verify(val); err != nil {
		return "", fmt.Errorf("stored value for %q is invalid: %w", spec.Key, err)
	}
	return val, nil
}

func (b *BatchPrompter) Confirm(message string) (bool, error) {
	b.logf("non-interactive: auto-confirming: %s", message)
	return true, nil
}

func (b *BatchPrompter) Gate(message, keyword string) error {
	b.logf("non-interactive: auto-passing gate: %s", message)
	return nil
}
