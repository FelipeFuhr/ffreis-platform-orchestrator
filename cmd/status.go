package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-cli/pkg/ui"
)

const statusTimeFormat = "2006-01-02T15:04:05"

func newStatusCmd(d *deps, gf *globalFlags) *cobra.Command {
	var runID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the step states for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := newCommandOutput(cmd, d.ui)
			state := newStateStore(d.cfgctl)

			resolvedRunID, err := resolveStatusRunID(ctx, state, runID)
			if err != nil {
				return err
			}

			d.log.Info("loading status", "run_id", resolvedRunID)

			states, err := state.LoadAllStepStates(ctx, resolvedRunID)
			if err != nil {
				return fmt.Errorf("load step states: %w", err)
			}

			out.Header("Platform Orchestrator Status", "run "+resolvedRunID)
			if d.ui != nil {
				out.Summary("Steps", countPart("count", len(states)))
			}

			if err := writeStatusTable(out, states, d.ui); err != nil {
				return err
			}
			_ = gf
			return nil
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

func writeStatusTable(out *commandOutput, states map[string]*pipeline.StepState, presenter *ui.Presenter) error {
	keys := make([]string, 0, len(states))
	for stepID := range states {
		keys = append(keys, stepID)
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(keys))
	for _, stepID := range keys {
		ss := states[stepID]
		rows = append(rows, []string{
			ss.StepID,
			stepStatusBadge(presenter, ss.Status),
			strconv.Itoa(ss.Attempts),
			formatStatusTime(ss.StartedAt),
			formatStatusTime(ss.FinishedAt),
		})
	}
	return out.Table([]string{"STEP", "STATUS", "ATTEMPTS", "STARTED", "FINISHED"}, rows)
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(statusTimeFormat)
}
