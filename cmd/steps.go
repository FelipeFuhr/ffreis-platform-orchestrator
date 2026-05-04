package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ffreis/platform-orchestrator/internal/pipeline"
)

func newStepsCmd(d *deps, gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "steps",
		Short: "List all steps in the platform pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newCommandOutput(cmd, d.ui)
			dag, err := buildPlatformSetupPipeline(d.cfgctl)
			if err != nil {
				return fmt.Errorf("build pipeline: %w", err)
			}
			sorted, err := dag.TopoSort()
			if err != nil {
				return fmt.Errorf("sort pipeline: %w", err)
			}

			out.Header("Platform Orchestrator Steps", strconv.Itoa(len(sorted))+" pipeline step(s)")
			_ = gf
			return writeStepsTable(out, sorted)
		},
	}
}

func writeStepsTable(out *commandOutput, steps []pipeline.Step) error {
	rows := make([][]string, 0, len(steps))
	for i, step := range steps {
		deps := strings.Join(step.Deps(), ", ")
		if deps == "" {
			deps = "-"
		}
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			step.ID(),
			step.Name(),
			step.CredentialClass().String(),
			deps,
		})
	}
	return out.Table([]string{"ORDER", "ID", "NAME", "CREDENTIAL", "DEPS"}, rows)
}
