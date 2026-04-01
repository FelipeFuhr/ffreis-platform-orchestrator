package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
)

// Status represents the lifecycle state of a single step within a run.
type Status string

const (
	StatusPending        Status = "pending"
	StatusRunning        Status = "running"
	StatusSucceeded      Status = "succeeded"
	StatusFailed         Status = "failed"
	StatusSkipped        Status = "skipped"
	StatusRolledBack     Status = "rolled_back"
	StatusRollbackFailed Status = "rollback_failed"
)

// StepState captures the persisted execution record for one step in one run.
type StepState struct {
	StepID     string            `json:"step_id"`
	Status     Status            `json:"status"`
	StartedAt  time.Time         `json:"started_at,omitempty"`
	FinishedAt time.Time         `json:"finished_at,omitempty"`
	Attempts   int               `json:"attempts"`
	ErrMsg     string            `json:"error,omitempty"`
	Outputs    map[string]string `json:"outputs,omitempty"`
}

// RunMeta stores the top-level run record.
type RunMeta struct {
	RunID     string    `json:"run_id"`
	Project   string    `json:"project"`
	Env       string    `json:"env"`
	StartedAt time.Time `json:"started_at"`
	Status    Status    `json:"status"`
}

// StateStore persists and retrieves step states.
type StateStore struct {
	cfg     configctl.Client
	project string // always platformcfg.PlatformProject
	env     string // always platformcfg.GlobalEnv
}

// NewStateStore constructs a StateStore.
func NewStateStore(cfg configctl.Client) *StateStore {
	return &StateStore{cfg: cfg, project: platformcfg.PlatformProject, env: platformcfg.GlobalEnv}
}

// SaveRunMeta persists a RunMeta record and updates the last_run_id pointer.
func (s *StateStore) SaveRunMeta(ctx context.Context, meta *RunMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal run meta: %w", err)
	}
	metaKey := platformcfg.RunMetaKey(meta.RunID)
	if err := s.cfg.Set(ctx, s.project, s.env, metaKey, string(raw)); err != nil {
		return fmt.Errorf("save run meta: %w", err)
	}
	return s.cfg.Set(ctx, s.project, s.env, platformcfg.KeyLastRunID, meta.RunID)
}

// LoadRunMeta retrieves the RunMeta for the given run ID.
func (s *StateStore) LoadRunMeta(ctx context.Context, runID string) (*RunMeta, error) {
	key := platformcfg.RunMetaKey(runID)
	raw, err := s.cfg.Get(ctx, s.project, s.env, key)
	if err != nil {
		return nil, fmt.Errorf("load run meta: %w", err)
	}
	var meta RunMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, fmt.Errorf("parse run meta: %w", err)
	}
	return &meta, nil
}

// LastRunID returns the ID of the most recent run, or ErrNotFound.
func (s *StateStore) LastRunID(ctx context.Context) (string, error) {
	return s.cfg.Get(ctx, s.project, s.env, platformcfg.KeyLastRunID)
}

// SaveStepState persists a StepState for the given run.
func (s *StateStore) SaveStepState(ctx context.Context, runID string, state *StepState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal step state: %w", err)
	}
	key := platformcfg.StepStateKey(runID, state.StepID)
	if err := s.cfg.Set(ctx, s.project, s.env, key, string(raw)); err != nil {
		return fmt.Errorf("save step state for %q: %w", state.StepID, err)
	}
	return nil
}

// LoadStepState retrieves the StepState for one step in a run.
// Returns nil (no error) if the step state has not been persisted yet.
func (s *StateStore) LoadStepState(ctx context.Context, runID, stepID string) (*StepState, error) {
	key := platformcfg.StepStateKey(runID, stepID)
	raw, err := s.cfg.Get(ctx, s.project, s.env, key)
	if err != nil {
		if configctl.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load step state for %q: %w", stepID, err)
	}
	var state StepState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, fmt.Errorf("parse step state for %q: %w", stepID, err)
	}
	return &state, nil
}

// LoadAllStepStates returns all persisted step states for a run,
// keyed by step ID.
func (s *StateStore) LoadAllStepStates(ctx context.Context, runID string) (map[string]*StepState, error) {
	prefix := platformcfg.RunPrefix(runID)
	all, err := s.cfg.List(ctx, s.project, s.env)
	if err != nil {
		return nil, fmt.Errorf("list step states: %w", err)
	}

	states := make(map[string]*StepState)
	statePrefix := prefix + "/step/"
	stateSuffix := "/state"
	for key, raw := range all {
		if len(key) < len(statePrefix)+len(stateSuffix) {
			continue
		}
		if key[:len(statePrefix)] != statePrefix {
			continue
		}
		if key[len(key)-len(stateSuffix):] != stateSuffix {
			continue
		}
		var st StepState
		if err := json.Unmarshal([]byte(raw), &st); err != nil {
			continue
		}
		states[st.StepID] = &st
	}
	return states, nil
}
