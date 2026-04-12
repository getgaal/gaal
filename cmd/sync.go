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

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/telemetry"
)

var (
	service  bool
	interval time.Duration
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronise repositories, skills and MCP configurations",
	Long: `Performs a one-shot synchronisation of all resources defined in the
configuration file: clones or updates repositories, installs or refreshes
agent skills, and upserts MCP server entries.

Use --service to run continuously at a fixed interval.`,
	SilenceUsage: true,
	RunE:         runSync,
}

func init() {
	syncCmd.Flags().BoolVarP(&service, "service", "s", false, "run as a continuous service (daemon mode)")
	syncCmd.Flags().DurationVarP(&interval, "interval", "i", 5*time.Minute, "polling interval in service mode")
	rootCmd.AddCommand(syncCmd)
}

func runSync(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		telemetry.TrackError("sync", err)
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	eng := engine.NewWithOptions(cfg, engineOpts)

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
