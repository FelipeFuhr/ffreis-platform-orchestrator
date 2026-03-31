package runner

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ExecRunner implements Runner using os/exec with no shell expansion.
type ExecRunner struct{}

// NewExecRunner constructs an ExecRunner.
func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// Exec runs the command with the given args and options.
// The process environment is built from opts.Env only; the parent shell
// environment is not inherited.
func (r *ExecRunner) Exec(command string, args []string, opts ExecOptions) (ExecResult, error) {
	// exec.Command does not invoke a shell; callers must pass args as discrete strings.
	cmd := exec.Command(command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command #nosec G204 — caller controls args; no shell expansion

	// Build a clean environment from the provided map.
	// Shell environment is intentionally not forwarded.
	env := make([]string, 0, len(opts.Env))
	for k, v := range opts.Env {
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
