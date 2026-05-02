package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"gaal/internal/engine"
	"gaal/internal/engine/render"
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
	cfg := resolvedCfg

	telemetry.Track("status")
	format := effectiveOutputFormat()
	if err := engine.NewWithOptions(cfg.Config, engineOpts).
		Status(context.Background(), engine.OutputFormat(format)); err != nil {
		return err
	}
	if format == string(render.FormatText) || format == "" {
		render.WriteTip(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())))
	}
	return nil
}
