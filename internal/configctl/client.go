package configctl

import "context"

// Client provides configuration read/write operations backed by DynamoDB.
// The schema follows the same PK/SK conventions as platform-configctl so
// that stored values are inspectable with that CLI.
type Client interface {
	// Get retrieves a plaintext config value. Returns ErrNotFound if absent.
	Get(ctx context.Context, project, env, key string) (string, error)

	// Set writes a plaintext config value. Idempotent.
	Set(ctx context.Context, project, env, key, value string) error

	// Delete removes a config key. No-op if absent.
	Delete(ctx context.Context, project, env, key string) error

	// List returns all config keys and values for the project+env.
	List(ctx context.Context, project, env string) (map[string]string, error)
}

// ErrNotFound is returned by Get when the requested key does not exist.
type ErrNotFoundError struct{ Key string }

func (e *ErrNotFoundError) Error() string { return "key not found: " + e.Key }

// ErrNotFound is the sentinel value for not-found checks.
var ErrNotFound = &ErrNotFoundError{}

// IsNotFound returns true if err is a not-found error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ErrNotFoundError)
	return ok
}
