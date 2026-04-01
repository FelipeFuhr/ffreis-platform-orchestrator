package pipeline

import (
	"context"

	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

// Step is the unit of work in a pipeline.
// Each implementation is responsible for a single, idempotent platform action.
// All methods must be safe to call multiple times.
type Step interface {
	// ID returns the unique, stable identifier for this step.
	// Use snake_case (e.g. "create_admin_role").
	ID() string

	// Name returns a human-readable display label.
	Name() string

	// Deps returns the IDs of steps that must have Status==Succeeded before
	// this step may execute.
	Deps() []string

	// CredentialClass returns the AWS privilege level required.
	CredentialClass() credential.Class

	// RequiredInputs lists the InputSpecs this step needs from the collection phase.
	RequiredInputs() []prompt.InputSpec

	// RetryPolicy returns the retry configuration for this step.
	RetryPolicy() RetryPolicy

	// IsDone reports whether the step's effect already exists in the world.
	// When true the engine marks the step Skipped without calling Run.
	// IsDone must be read-only; it must not modify external state.
	IsDone(ctx context.Context, execCtx *ExecutionContext) (bool, error)

	// Run executes the step. It must be idempotent: if called on an already-
	// complete step it must succeed without creating duplicate resources.
	Run(ctx context.Context, execCtx *ExecutionContext) error

	// Rollback undoes the effects of Run. Return ErrRollbackNotSupported if
	// the step cannot be reversed. Rollback is never called automatically.
	Rollback(ctx context.Context, execCtx *ExecutionContext) error
}

// RetryPolicy configures the retry behaviour for a single step.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts (including the first).
	// 0 means "try once, never retry".
	MaxAttempts int

	// Backoff controls the wait between retries.
	Backoff BackoffStrategy
}

// BackoffStrategy controls inter-retry delay.
type BackoffStrategy int

const (
	BackoffFixed       BackoffStrategy = iota // constant 5s between retries
	BackoffExponential                        // doubles: 1s, 2s, 4s, 8s...
)

// NoRetry is a RetryPolicy that never retries.
var NoRetry = RetryPolicy{MaxAttempts: 1}

// RetryThrice retries up to three times with exponential backoff.
var RetryThrice = RetryPolicy{MaxAttempts: 3, Backoff: BackoffExponential}

// ErrRollbackNotSupported is returned by Rollback when the step cannot be reversed.
type ErrRollbackNotSupportedError struct{ StepID string }

func (e *ErrRollbackNotSupportedError) Error() string {
	return "rollback not supported for step: " + e.StepID
}

// ErrRollbackNotSupported constructs the rollback-not-supported error.
func ErrRollbackNotSupported(id string) error {
	return &ErrRollbackNotSupportedError{StepID: id}
}

// IsRollbackNotSupported returns true if err is an ErrRollbackNotSupportedError.
func IsRollbackNotSupported(err error) bool {
	_, ok := err.(*ErrRollbackNotSupportedError)
	return ok
}
