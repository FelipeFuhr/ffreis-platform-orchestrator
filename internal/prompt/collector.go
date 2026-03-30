package prompt

import (
	"context"
	"fmt"

	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

// Collector runs the pre-execution collection phase: it loads existing config,
// identifies missing inputs, prompts for them, then writes confirmed values.
type Collector struct {
	cfg      configctl.Client
	prompter Prompter
	project  string
	env      string
}

// NewCollector constructs a Collector.
func NewCollector(cfg configctl.Client, prompter Prompter, project, env string) *Collector {
	return &Collector{cfg: cfg, prompter: prompter, project: project, env: env}
}

// CollectResult holds the values gathered during the collection phase.
type CollectResult struct {
	// Values maps InputSpec.Key to the confirmed value.
	Values map[string]string
	// Skipped lists keys that already had valid values in configctl.
	Skipped []string
	// Collected lists keys that were prompted and written.
	Collected []string
}

// Collect iterates specs, prompts for missing values, and writes them to configctl.
// It returns a CollectResult so callers can log what was gathered.
func (c *Collector) Collect(ctx context.Context, specs []InputSpec) (*CollectResult, error) {
	result := &CollectResult{
		Values: make(map[string]string, len(specs)),
	}

	for _, spec := range dedupeSpecs(specs) {
		if err := c.collectOne(ctx, result, spec); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func dedupeSpecs(specs []InputSpec) []InputSpec {
	seen := make(map[string]bool)
	var deduped []InputSpec
	for _, s := range specs {
		if seen[s.Key] {
			continue
		}
		seen[s.Key] = true
		deduped = append(deduped, s)
	}
	return deduped
}

func (c *Collector) collectOne(ctx context.Context, result *CollectResult, spec InputSpec) error {
	existing, err := c.loadExisting(ctx, spec.Key)
	if err != nil {
		return err
	}

	if existing != "" && spec.Verify(existing) == nil {
		result.Values[spec.Key] = existing
		result.Skipped = append(result.Skipped, spec.Key)
		return nil
	}

	spec = applyDefaultFromExisting(spec, existing)

	value, err := c.prompter.Ask(spec)
	if err != nil {
		return fmt.Errorf("prompt for %q: %w", spec.Key, err)
	}

	if value == "" && spec.Optional {
		result.Values[spec.Key] = ""
		return nil
	}

	if err := c.cfg.Set(ctx, c.project, c.env, spec.Key, value); err != nil {
		return fmt.Errorf("write %q to configctl: %w", spec.Key, err)
	}

	result.Values[spec.Key] = value
	result.Collected = append(result.Collected, spec.Key)
	return nil
}

func (c *Collector) loadExisting(ctx context.Context, key string) (string, error) {
	existing, err := c.cfg.Get(ctx, c.project, c.env, key)
	if err == nil {
		return existing, nil
	}
	if configctl.IsNotFound(err) {
		return "", nil
	}
	return "", fmt.Errorf("load key %q: %w", key, err)
}

func applyDefaultFromExisting(spec InputSpec, existing string) InputSpec {
	if existing != "" && spec.Default == "" {
		spec.Default = existing
	}
	return spec
}
