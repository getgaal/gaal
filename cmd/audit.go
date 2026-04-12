package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/telemetry"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Discover all skills and MCP servers installed on this machine",
	Long: `Scans well-known directories for every registered AI coding agent and lists
all SKILL.md files and MCP server entries found, regardless of the local
gaal.yaml configuration.

Three source types are reported:
  project        – skills found in project-relative directories (cwd)
  global         – skills found in user-home directories (~/)
  package-manager – skills installed by the agent's own extension manager

The command never modifies any file. It is safe to run at any time.`,
	SilenceUsage: true,
	RunE:         runAudit,
}

func init() {
	rootCmd.AddCommand(auditCmd)
}

func runAudit(_ *cobra.Command, _ []string) error {
	telemetry.Track("audit")
	// Audit does not require a gaal.yaml — pass an empty config.
	return engine.NewWithOptions(&config.Config{}, engineOpts).
		Audit(context.Background(), engine.OutputFormat(outputFormat))
}
