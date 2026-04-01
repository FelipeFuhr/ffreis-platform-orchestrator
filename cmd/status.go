package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

const statusTimeFormat = "2006-01-02T15:04:05"

func newStatusCmd(d *deps, gf *globalFlags) *cobra.Command {
	var runID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the step states for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			state := newStateStore(d.cfgctl)

			resolvedRunID, err := resolveStatusRunID(ctx, state, runID)
			if err != nil {
				return err
			}

			d.log.Info("loading status", zap.String("run_id", resolvedRunID))

			states, err := state.LoadAllStepStates(ctx, resolvedRunID)
			if err != nil {
				return fmt.Errorf("load step states: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if err := writeStatusTable(w, states); err != nil {
				return err
			}
			_ = gf
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID (defaults to last run)")
	return cmd
}

type lastRunIDGetter interface {
	LastRunID(ctx context.Context) (string, error)
}

func resolveStatusRunID(ctx context.Context, state lastRunIDGetter, runID string) (string, error) {
	if runID != "" {
		return runID, nil
	}

	lastID, err := state.LastRunID(ctx)
	if err != nil {
		return "", fmt.Errorf("no run ID provided and no previous run found: %w", err)
	}
	return lastID, nil
}

func writeStatusTable(w *tabwriter.Writer, states map[string]*pipeline.StepState) error {
	if _, err := fmt.Fprintln(w, "STEP\tSTATUS\tATTEMPTS\tSTARTED\tFINISHED"); err != nil {
		return err
	}
	keys := make([]string, 0, len(states))
	for stepID := range states {
		keys = append(keys, stepID)
	}
	sort.Strings(keys)

	for _, stepID := range keys {
		ss := states[stepID]
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			ss.StepID, ss.Status, ss.Attempts, formatStatusTime(ss.StartedAt), formatStatusTime(ss.FinishedAt)); err != nil {
			return err
		}
	}
	return nil
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(statusTimeFormat)
}
