package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
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
			out := newCommandOutput(cmd, d.ui)

			// Resolve run ID.
			if runID == "" {
				state := newStateStore(d.cfgctl)
				lastID, err := state.LastRunID(ctx)
				if err != nil {
					return fmt.Errorf("no run ID provided and no previous run found: %w", err)
				}
				runID = lastID
			}

			out.Header("Platform Orchestrator Resume", runSummary(runID, gf.project, gf.env))
			d.log.Info("resuming run", "run_id", runID)

			dag, err := buildPlatformSetupPipeline(d.cfgctl)
			if err != nil {
				return fmt.Errorf("build pipeline: %w", err)
			}

			// Collection phase: only prompt for values still missing.
			if err := collectInputs(ctx, out, d, gf, dag); err != nil {
				return fmt.Errorf("input collection: %w", err)
			}

			eng := buildEngine(ctx, d, gf, dag, runID, cmd.ErrOrStderr())
			return eng.Resume(ctx, runID)
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "ID of the run to resume (defaults to last run)")
	return cmd
}
