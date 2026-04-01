package prompt

import "fmt"

// InputSpec describes a single value the orchestrator needs before execution.
type InputSpec struct {
	// Key is the configctl key used to read and write this value.
	Key string

	// Label is shown to the operator in interactive prompts.
	Label string

	// Default is the value to use if the operator presses Enter without typing.
	// May be empty.
	Default string

	// Sensitive marks the value as a secret; input is masked during prompting
	// and the value is stored under the SECRET# prefix in DynamoDB.
	Sensitive bool

	// Optional indicates the value may be left empty.
	Optional bool

	// Validate is called with the operator's input. Return an error to reject
	// the value and re-prompt. May be nil.
	Validate func(string) error
}

// Verify calls Validate if set.
func (s InputSpec) Verify(value string) error {
	if s.Validate == nil {
		return nil
	}
	if err := s.Validate(value); err != nil {
		return fmt.Errorf("invalid value for %q: %w", s.Key, err)
	}
	return nil
}
