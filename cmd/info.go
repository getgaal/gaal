package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
)

var infoCmd = &cobra.Command{
	Use:   "info <repo|skill|mcp> [name]",
	Short: "Show detailed spec and state for a package type",
	Long: `Displays a full information card for every entry of the given package type, combining the configuration spec with the current runtime state.

Optionally pass a name/source to filter results (case-insensitive substring).

Package types:
  repo   — repository URL, type, configured version vs current HEAD, dirty flag
  skill  — source, target agents, scope, selection filter, per-agent installation tree
  mcp    — server name, target file, inline definition, merge flag, dirty detection

Examples:
  gaal info skill
  gaal info skill vercel-labs/agent-skills
  gaal info repo workspace/myrepo
  gaal info mcp claude`,
	SilenceUsage: true,
	Args:         cobra.RangeArgs(1, 2),
	ValidArgs:    []string{"repo", "skill", "mcp"},
	RunE:         runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func runInfo(_ *cobra.Command, args []string) error {
	pkg := args[0]
	filter := ""
	if len(args) == 2 {
		filter = args[1]
	}

	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	return engine.NewWithOptions(cfg, engineOpts).
		Info(context.Background(), pkg, filter)
}
