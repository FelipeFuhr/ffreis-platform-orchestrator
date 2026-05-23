package runner

import "context"

// Runner executes external processes without shell expansion.
// All implementations must pass arguments as discrete strings, never
// through shell interpolation, to prevent injection.
type Runner interface {
	// Exec runs command with args in workdir, injecting env into the process
	// environment (on top of an empty base — not inherited from the shell).
	// stdout and stderr are returned as strings on completion.
	//
	// The ctx parameter is wired into exec.CommandContext so that SIGINT /
	// SIGTERM at the parent process kills the child immediately. Long-running
	// subprocesses (terraform plan/apply) MUST receive a cancellable context;
	// passing context.Background here re-introduces the orphan-process bug.
	Exec(ctx context.Context, command string, args []string, opts ExecOptions) (ExecResult, error)
}

// ExecOptions configures a single execution.
type ExecOptions struct {
	// WorkDir is the working directory for the process.
	// Defaults to the current working directory if empty.
	WorkDir string

	// Env holds additional environment variables injected into the process.
	// These are the ONLY environment variables the process sees; the shell
	// environment is NOT inherited to prevent credential leakage.
	Env map[string]string
}

// ExecResult holds the output of a completed process.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}
