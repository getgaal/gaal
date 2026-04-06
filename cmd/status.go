package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
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
	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	return engine.NewWithOptions(cfg, engineOpts).Status(context.Background())
}
