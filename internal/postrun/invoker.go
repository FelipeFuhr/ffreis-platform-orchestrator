// Package postrun invokes platform-runner as a subprocess after the pipeline
// completes. There is no import coupling between platform-orchestrator and
// platform-runner — the only contract is the CLI interface of the binary.
package postrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Options configures a Runner invocation.
type Options struct {
	// Binary is the path to the platform-runner executable.
	// Defaults to "platform-runner" (resolved via PATH).
	Binary string

	// Config is passed as --config to every platform-runner subcommand.
	Config string

	// RulesDir is passed to the validate subcommand as --rules-dir.
	RulesDir string

	// TemplateDir is passed to the sync-template subcommand as --template-dir.
	TemplateDir string

	// Workspace is passed as --workspace to every subcommand.
	Workspace string

	// Token is the GitHub token.  It is injected via the GITHUB_TOKEN
	// environment variable, never via a flag or logged.
	Token string

	// DryRun is forwarded as --dry-run to all subcommands.
	DryRun bool
}

// Invoker runs platform-runner subcommands as isolated subprocesses.
type Invoker struct {
	opts Options
}

// New constructs an Invoker.  binary defaults to "platform-runner" if empty.
func New(opts Options) *Invoker {
	if opts.Binary == "" {
		opts.Binary = "platform-runner"
	}
	return &Invoker{opts: opts}
}

// Validate runs: platform-runner validate [--rules-dir …] [--config …]
func (inv *Invoker) Validate(ctx context.Context) error {
	args := inv.baseArgs("validate")
	if inv.opts.RulesDir != "" {
		args = append(args, "--rules-dir", inv.opts.RulesDir)
	}
	return inv.exec(ctx, args)
}

// SyncTemplate runs: platform-runner sync-template --template-dir … [--dry-run]
func (inv *Invoker) SyncTemplate(ctx context.Context) error {
	args := inv.baseArgs("sync-template")
	if inv.opts.TemplateDir != "" {
		args = append(args, "--template-dir", inv.opts.TemplateDir)
	}
	return inv.exec(ctx, args)
}

// PlanAll runs: platform-runner plan-all --dry-run
// apply-all is intentionally not exposed here — it requires explicit human confirmation.
func (inv *Invoker) PlanAll(ctx context.Context) error {
	// plan-all is always run with --dry-run from the orchestrator to produce a
	// preview only.  The operator decides whether to promote to apply.
	args := append(inv.baseArgs("plan-all"), "--dry-run")
	return inv.exec(ctx, args)
}

// baseArgs builds the common argument prefix for every subcommand.
func (inv *Invoker) baseArgs(subcommand string) []string {
	args := []string{subcommand}
	if inv.opts.Config != "" {
		args = append(args, "--config", inv.opts.Config)
	}
	if inv.opts.Workspace != "" {
		args = append(args, "--workspace", inv.opts.Workspace)
	}
	if inv.opts.DryRun {
		args = append(args, "--dry-run")
	}
	return args
}

// exec runs the binary with args.  The process environment contains only
// GITHUB_TOKEN (when set) and HOME — no shell environment is forwarded.
func (inv *Invoker) exec(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, inv.opts.Binary, args...) // #nosec G204 — binary from config, not user input

	env := []string{}
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	if inv.opts.Token != "" {
		env = append(env, "GITHUB_TOKEN="+inv.opts.Token)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Redact token from any error messages before surfacing.
		errMsg := stderr.String()
		if inv.opts.Token != "" {
			errMsg = strings.ReplaceAll(errMsg, inv.opts.Token, "***")
		}
		return fmt.Errorf("platform-runner %s: %w\n%s", args[0], err, errMsg)
	}
	return nil
}
