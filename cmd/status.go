package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newStatusCmd(d *deps, gf *globalFlags) *cobra.Command {
	var runID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the step states for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			state := newStateStore(d.cfgctl)

			if runID == "" {
				lastID, err := state.LastRunID(ctx)
				if err != nil {
					return fmt.Errorf("no run ID provided and no previous run found: %w", err)
				}
				runID = lastID
			}

			d.log.Info("loading status", zap.String("run_id", runID))

			states, err := state.LoadAllStepStates(ctx, runID)
			if err != nil {
				return fmt.Errorf("load step states: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "STEP\tSTATUS\tATTEMPTS\tSTARTED\tFINISHED"); err != nil {
				return err
			}
			for _, ss := range states {
				started := "-"
				if !ss.StartedAt.IsZero() {
					started = ss.StartedAt.Format("2006-01-02T15:04:05")
				}
				finished := "-"
				if !ss.FinishedAt.IsZero() {
					finished = ss.FinishedAt.Format("2006-01-02T15:04:05")
				}
				if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
					ss.StepID, ss.Status, ss.Attempts, started, finished); err != nil {
					return err
				}
			}
			_ = gf
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (defaults to last run)")
	return cmd
}
