package credential

import "github.com/aws/aws-sdk-go-v2/aws"

// Resolver resolves an aws.Config for a given credential Class.
// Implementations must be safe for concurrent use; credentials are resolved
// fresh per call and are not cached beyond the caller's scope.
type Resolver interface {
	Resolve(class Class) (aws.Config, error)
}
