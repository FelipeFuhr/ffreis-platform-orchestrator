package cmd

import (
	"context"
	"testing"
)

func TestContextFromCmdContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "k", "v")
	got := contextFromCmdContext(ctx)
	if got != ctx {
		t.Fatal("expected same context to be returned")
	}

	bg := contextFromCmdContext("not-a-context")
	if bg == nil {
		t.Fatal("expected background context")
	}
}
