package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"gaal/internal/engine"
	"gaal/internal/telemetry"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current status of repositories, skills and MCP configs",
	Long: `Displays whether each repository is cloned and at the correct version,
which agent skills are installed, and which MCP server entries are present in
their target configuration files.`,
	SilenceUsage: true,
	RunE:         runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(_ *cobra.Command, _ []string) error {
	if resolvedCfg == nil {
		return fmt.Errorf("loading config: %w", resolvedCfgErr)
	}
	cfg := resolvedCfg

	telemetry.Track("status")
	return engine.NewWithOptions(cfg.Config, engineOpts).
		Status(context.Background(), engine.OutputFormat(outputFormat))
}
