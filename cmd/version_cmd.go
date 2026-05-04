package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print build information",
	// Override parent's PersistentPreRunE so that 'version' does not
	// attempt to load app config, AWS config, or DynamoDB on startup.
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error { return nil },
	Run: func(cmd *cobra.Command, _ []string) {
		out := newCommandOutput(cmd, nil)

		v := strings.TrimSpace(version)
		if v == "" {
			v = "dev"
		}
		c := strings.TrimSpace(commit)
		if c == "" {
			c = "unknown"
		}
		t := strings.TrimSpace(buildTime)
		if t == "" {
			t = "unknown"
		}

		out.Line(v + " (commit=" + c + " built=" + t + ")")
	},
}
