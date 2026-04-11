package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap a gaal.yaml in the current directory",
	Long: `Creates a documented gaal.yaml skeleton in the current directory (or at
the path specified by --config).

All three resource sections (repositories, skills, mcps) are present with
inline field descriptions and usage examples so you can fill them in right away.

Use --force to overwrite an existing file.`,
	SilenceUsage: true,
	RunE:         runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "overwrite an existing configuration file")
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	if err := engine.NewWithOptions(&config.Config{}, engineOpts).Init(cfgFile, forceInit); err != nil {
		return err
	}
	fmt.Printf("Created %s\n\nNext steps:\n  1. Edit %s to describe your repositories, skills and MCP servers.\n  2. Run: gaal sync\n  3. Run: gaal status\n", cfgFile, cfgFile)
	return nil
}
