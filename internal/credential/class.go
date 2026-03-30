package credential

// Class represents the AWS privilege level required to execute a step.
type Class int

const (
	// ClassRoot uses the root account credentials (env vars or [root] profile).
	// Root credentials are never stored and are required only for bootstrap steps.
	ClassRoot Class = iota

	// ClassAdmin uses an IAM admin role assumed via STS AssumeRole.
	ClassAdmin

	// ClassOperator uses a restricted IAM operator role assumed via STS AssumeRole.
	ClassOperator
)

func (c Class) String() string {
	switch c {
	case ClassRoot:
		return "root"
	case ClassAdmin:
		return "admin"
	case ClassOperator:
		return "operator"
	default:
		return "unknown"
	}
}
