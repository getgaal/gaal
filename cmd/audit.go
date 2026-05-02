package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/engine/render"
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
	SilenceUsage: true, Annotations: map[string]string{"config": "optional"}, RunE: runAudit,
}

func init() {
	rootCmd.AddCommand(auditCmd)
}

func runAudit(_ *cobra.Command, _ []string) error {
	telemetry.Track("audit")
	// Audit does not require a gaal.yaml — pass an empty config.
	format := effectiveOutputFormat()
	if err := engine.NewWithOptions(&config.Config{}, engineOpts).
		Audit(context.Background(), engine.OutputFormat(format)); err != nil {
		return err
	}
	if format == string(render.FormatText) || format == "" {
		render.WriteTip(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())))
	}
	return nil
}
