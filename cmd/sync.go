package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"gaal/internal/engine"
	"gaal/internal/telemetry"
)

var (
	service  bool
	interval time.Duration
	dryRun   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronise repositories, skills and MCP configurations",
	Long: `Performs a one-shot synchronisation of all resources defined in the
configuration file: clones or updates repositories, installs or refreshes
agent skills, and upserts MCP server entries.

Use --service to run continuously at a fixed interval.
Use --dry-run to preview what sync would do without writing anything.`,
	SilenceUsage: true,
	RunE:         runSync,
}

func init() {
	syncCmd.Flags().BoolVarP(&service, "service", "s", false, "run as a continuous service (daemon mode)")
	syncCmd.Flags().DurationVarP(&interval, "interval", "i", 5*time.Minute, "polling interval in service mode")
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview what sync would do without writing anything")
	rootCmd.AddCommand(syncCmd)
}

func runSync(_ *cobra.Command, _ []string) error {
	if dryRun && service {
		return fmt.Errorf("--dry-run and --service are incompatible: a dry-run service loop is meaningless")
	}

	cfg := resolvedCfg

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	eng := engine.NewWithOptions(cfg.Config, engineOpts)

	if dryRun {
		slog.Info("dry-run mode", "config", cfgFile)
		format := engine.OutputFormat(outputFormat)
		plan, err := eng.DryRun(ctx, format)
		if err != nil {
			telemetry.TrackError("sync", err)
			os.Exit(2)
		}
		telemetry.Track("sync-dry-run")
		if plan.HasErrors {
			os.Exit(2)
		}
		if plan.HasChanges {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if service {
		slog.Info("service mode started", "interval", interval, "config", cfgFile)
		telemetry.Track("sync")
		return eng.RunService(ctx, interval)
	}

	slog.Info("one-shot sync", "config", cfgFile)
	if err := eng.RunOnce(ctx); err != nil {
		telemetry.TrackError("sync", err)
		return err
	}
	telemetry.Track("sync")
	telemetry.TrackFirstSync(0)
	return nil
}
