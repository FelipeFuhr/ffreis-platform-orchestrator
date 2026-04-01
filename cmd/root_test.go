package cmd

import (
	"strings"
	"testing"
)

func TestRequireProjectEnv(t *testing.T) {
	gf := &globalFlags{}
	if err := requireProjectEnv(gf); err == nil {
		t.Fatal("expected error")
	}
	gf.project = "p"
	if err := requireProjectEnv(gf); err == nil {
		t.Fatal("expected error")
	}
	gf.env = "dev"
	if err := requireProjectEnv(gf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRunID_Format(t *testing.T) {
	id := newRunID()
	if len(id) != 12 {
		t.Fatalf("len(newRunID()) = %d, want 12", len(id))
	}
	if strings.ContainsAny(id, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("expected lowercase/nums only, got %q", id)
	}
}

func TestBuildRoot_HasSubcommandsAndFlags(t *testing.T) {
	root := buildRoot()
	if root.Use != "platform-orchestrator" {
		t.Fatalf("Use = %q, want %q", root.Use, "platform-orchestrator")
	}
	if root.PersistentFlags().Lookup("env") == nil {
		t.Fatal("expected --env flag")
	}
	if root.Commands() == nil || len(root.Commands()) == 0 {
		t.Fatal("expected subcommands")
	}
}
