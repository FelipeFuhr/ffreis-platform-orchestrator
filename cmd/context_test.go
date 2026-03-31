package cmd

import (
	"context"
	"testing"
)

type contextKey string

func TestContextFromCmdContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("k"), "v")
	got := contextFromCmdContext(ctx)
	if got != ctx {
		t.Fatal("expected same context to be returned")
	}

	bg := contextFromCmdContext("not-a-context")
	if bg == nil {
		t.Fatal("expected background context")
	}
}
