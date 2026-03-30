package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/pipelines"
)

func newResumeCmd(d *deps, gf *globalFlags) *cobra.Command {
	var runID string

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume a previously failed run",
		Long: `resume loads the state of a prior run and re-executes only the steps
that have not yet succeeded. Steps that already succeeded are skipped without
re-running, preserving idempotency.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireProjectEnv(gf); err != nil {
				return err
			}
			ctx := cmd.Context()

			// Resolve run ID.
			if runID == "" {
				state := pipeline.NewStateStore(d.cfgctl)
				lastID, err := state.LastRunID(ctx)
				if err != nil {
					return fmt.Errorf("no run ID provided and no previous run found: %w", err)
				}
				runID = lastID
			}

			d.log.Info("resuming run", zap.String("run_id", runID))

			dag, err := pipelines.PlatformSetupPipeline(d.cfgctl)
			if err != nil {
				return fmt.Errorf("build pipeline: %w", err)
			}

			// Collection phase: only prompt for values still missing.
			if err := collectInputs(ctx, d, gf, dag); err != nil {
				return fmt.Errorf("input collection: %w", err)
			}

			eng := buildEngine(ctx, d, gf, dag, runID)
			return eng.Resume(ctx, runID)
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "ID of the run to resume (defaults to last run)")
	return cmd
}
