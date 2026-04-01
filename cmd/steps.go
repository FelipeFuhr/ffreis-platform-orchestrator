package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newStepsCmd(d *deps, gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "steps",
		Short: "List all steps in the platform pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			dag, err := buildPlatformSetupPipeline(d.cfgctl)
			if err != nil {
				return fmt.Errorf("build pipeline: %w", err)
			}
			sorted, err := dag.TopoSort()
			if err != nil {
				return fmt.Errorf("sort pipeline: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "ORDER\tID\tNAME\tCREDENTIAL\tDEPS"); err != nil {
				return err
			}
			for i, step := range sorted {
				deps := strings.Join(step.Deps(), ", ")
				if deps == "" {
					deps = "-"
				}
				if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
					i+1, step.ID(), step.Name(), step.CredentialClass(), deps); err != nil {
					return err
				}
			}
			_ = gf
			return w.Flush()
		},
	}
}
