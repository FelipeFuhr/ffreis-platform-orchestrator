package pipeline

import (
	"context"
	"testing"

	platformcfg "github.com/ffreis/platform-orchestrator/internal/config"
)

func TestStateStore_LoadAllStepStatesSkipsInvalid(t *testing.T) {
	store := newMemConfigStore()
	state := NewStateStore(store)
	store.data[store.storeKey(platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.StepStateKey("run-1", "a"))] = `{"step_id":"a","status":"succeeded"}`
	store.data[store.storeKey(platformcfg.PlatformProject, platformcfg.GlobalEnv, platformcfg.StepStateKey("run-1", "b"))] = `invalid-json`

	all, err := state.LoadAllStepStates(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("LoadAllStepStates() error: %v", err)
	}
	if len(all) != 1 || all["a"] == nil {
		t.Fatalf("unexpected states: %+v", all)
	}
}
