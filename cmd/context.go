package cmd

import "context"

// contextFromCmdContext converts the cobra command context interface to context.Context.
// Cobra commands receive context.Context directly, so this is a type assertion helper.
func contextFromCmdContext(ctx interface{}) context.Context {
	if c, ok := ctx.(context.Context); ok {
		return c
	}
	return context.Background()
}
