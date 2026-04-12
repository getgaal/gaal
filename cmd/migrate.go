package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
)

var (
	migrateTarget string
	migrateDryRun bool
	migrateYes    bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate --to community <url>",
	Short: "Migrate this Lite configuration to a gaal Community Edition instance",
	Long: `Migrate this Lite configuration to a gaal Community Edition instance.

Reads the local gaal configuration, validates it, and pushes it to the
specified Community URL as versioned configs.

Community Edition is not yet publicly available. Running this command
today validates your configuration and confirms it is ready to migrate.
Subscribe to the announcement list at https://getgaal.com to be notified
when migration becomes available.

Examples:
  gaal migrate --to community https://community.example.com
  gaal migrate --to community https://community.example.com --dry-run
  gaal migrate --to community https://community.example.com --yes`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runMigrate,
}

func init() {
	migrateCmd.Flags().StringVar(&migrateTarget, "to", "", `migration target (currently only "community" is supported)`)
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "validate everything but do not perform the migration")
	migrateCmd.Flags().BoolVar(&migrateYes, "yes", false, "skip interactive confirmation")
	_ = migrateCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	eng := engine.NewWithOptions(cfg, engineOpts)

	result, err := eng.Migrate(migrateTarget, args[0], migrateDryRun)
	if err != nil {
		return err
	}

	if migrateDryRun {
		fmt.Println("[dry-run] No changes will be made.")
		fmt.Println()
	}

	fmt.Printf("Would migrate %d repositories, %d skills, %d MCP servers to %s\n",
		result.Repositories, result.Skills, result.MCPs, result.URL)
	fmt.Println()
	fmt.Println("gaal Community Edition is not yet available. Your configuration is valid and")
	fmt.Println("ready to migrate when Community ships. Join the announcement list:")
	fmt.Println("https://getgaal.com")

	return nil
}
