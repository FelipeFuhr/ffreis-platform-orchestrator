package pipeline

import (
	"context"

	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

// ExecutionContext is the dependency bundle passed to every Step's IsDone, Run,
// and Rollback methods. Each step receives a context scoped to its credential class.
type ExecutionContext struct {
	ctx    context.Context
	cfg    configctl.Client
	run    runner.Runner
	awsCfg aws.Config
	log    logger.Logger

	project string
	env     string
	dryRun  bool

	outputs map[string]string
}

// NewExecutionContext constructs an ExecutionContext.
func NewExecutionContext(
	ctx context.Context,
	cfg configctl.Client,
	r runner.Runner,
	awsCfg aws.Config,
	log logger.Logger,
	project, env string,
	dryRun bool,
) *ExecutionContext {
	return &ExecutionContext{
		ctx:     ctx,
		cfg:     cfg,
		run:     r,
		awsCfg:  awsCfg,
		log:     log,
		project: project,
		env:     env,
		dryRun:  dryRun,
		outputs: make(map[string]string),
	}
}

// Config returns the configctl client.
func (c *ExecutionContext) Config() configctl.Client { return c.cfg }

// Runner returns the process runner.
func (c *ExecutionContext) Runner() runner.Runner { return c.run }

// AWSConfig returns the pre-resolved AWS configuration for this step's credential class.
func (c *ExecutionContext) AWSConfig() aws.Config { return c.awsCfg }

// Log returns the structured logger.
func (c *ExecutionContext) Log() logger.Logger { return c.log }

// Project returns the target platform project.
func (c *ExecutionContext) Project() string { return c.project }

// Env returns the target environment.
func (c *ExecutionContext) Env() string { return c.env }

// DryRun reports whether execution should describe but not mutate.
func (c *ExecutionContext) DryRun() bool { return c.dryRun }

// SetOutput records a step output value in memory and logs the key.
// Outputs are persisted to configctl by the engine after the step succeeds.
func (c *ExecutionContext) SetOutput(key, value string) {
	c.log.Info("step output set", zap.String("key", key))
	c.outputs[key] = value
}

// Outputs returns all outputs set during this step's execution.
func (c *ExecutionContext) Outputs() map[string]string {
	result := make(map[string]string, len(c.outputs))
	for k, v := range c.outputs {
		result[k] = v
	}
	return result
}

// Context returns the underlying Go context for cancellation and deadline propagation.
func (c *ExecutionContext) Context() context.Context { return c.ctx }
