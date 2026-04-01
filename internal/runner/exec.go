package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecRunner implements Runner using os/exec with no shell expansion.
type ExecRunner struct{}

// NewExecRunner constructs an ExecRunner.
func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// safeBaselineEnv contains the environment variable names that are always
// forwarded from the parent process to give subprocesses a functional runtime
// (plugin caches, temp dirs, PATH resolution) without leaking credentials.
var safeBaselineEnv = []string{"HOME", "PATH", "TMPDIR"}

// Exec runs the command with the given args and options.
// The subprocess environment is seeded with a safe baseline (HOME, PATH,
// TMPDIR) and then overlaid with opts.Env; no other parent env vars are
// forwarded to prevent credential leakage.
func (r *ExecRunner) Exec(command string, args []string, opts ExecOptions) (ExecResult, error) {
	// exec.Command does not invoke a shell; callers must pass args as discrete strings.
	//nolint:gosec // command execution is the purpose of this adapter; callers pass discrete args and no shell is involved
	cmd := exec.CommandContext(context.Background(), command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command #nosec G204 — caller controls args; no shell expansion

	// Seed with a safe baseline so tools like Terraform can locate plugin
	// caches, temp directories, and binaries in PATH.
	// opts.Env overlays the baseline; duplicate keys favor opts.Env values.
	merged := make(map[string]string, len(safeBaselineEnv)+len(opts.Env))
	for _, k := range safeBaselineEnv {
		if v := os.Getenv(k); v != "" {
			merged[k] = v
		}
	}
	for k, v := range opts.Env {
		merged[k] = v
	}
	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	result := ExecResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: 0,
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			// Non-zero exit is a domain error, not a Go error.
			return result, fmt.Errorf("command %q exited %d: %s", command, result.ExitCode, result.Stderr)
		}
		return result, fmt.Errorf("execute %q: %w", command, runErr)
	}

	return result, nil
}
