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
	"gaal/internal/engine/render"
	"gaal/internal/telemetry"
	"gaal/internal/tools"
)

var (
	service      bool
	interval     time.Duration
	dryRun       bool
	prune        bool
	forceSyncAll bool
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
	syncCmd.Flags().BoolVar(&prune, "prune", false, "remove skills and MCP entries no longer declared in config")
	syncCmd.Flags().BoolVar(&forceSyncAll, "force", false, "install skills into all registered agents even when agent dirs don't exist yet (applies to agents: [\"*\"] wildcard)")
	rootCmd.AddCommand(syncCmd)
}

func runSync(_ *cobra.Command, _ []string) error {
	if dryRun && service {
		return fmt.Errorf("--dry-run and --service are incompatible: a dry-run service loop is meaningless")
	}
	if prune && service {
		return fmt.Errorf("--prune and --service are incompatible: use one-shot mode for destructive operations")
	}

	cfg := resolvedCfg

	warnMissingTools(cfg.Config)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := engineOpts
	opts.Force = forceSyncAll
	eng := engine.NewWithOptions(cfg.Config, opts)

	if dryRun {
		slog.Info("dry-run mode", "config", cfgFile)
		format := engine.OutputFormat(effectiveOutputFormat())
		plan, err := eng.DryRun(ctx, format)
		if err != nil {
			telemetry.TrackError("sync", err)
			return &ExitCodeError{Code: 2, Cause: err}
		}
		telemetry.Track("sync-dry-run")
		if plan.HasErrors {
			return &ExitCodeError{Code: 2}
		}
		if plan.HasChanges {
			return &ExitCodeError{Code: 1}
		}
		return nil
	}

	if service {
		slog.Info("service mode started", "interval", interval, "config", cfgFile)
		telemetry.Track("sync")
		return eng.RunService(ctx, interval)
	}

	slog.Debug("one-shot sync", "config", cfgFile)
	plan, err := eng.Plan(ctx)
	if err != nil {
		telemetry.TrackError("sync", err)
		return err
	}
	if err := checkInterrupted(ctx); err != nil {
		return err
	}

	if err := eng.Hooks().RunPreSync(ctx, plan); err != nil {
		fmt.Fprintln(os.Stderr, "⚠ pre-sync hook failed; aborting sync")
		telemetry.TrackError("sync-hook-pre", err)
		return err
	}
	if err := checkInterrupted(ctx); err != nil {
		return err
	}

	start := time.Now()
	if err := eng.RunOnce(ctx); err != nil {
		telemetry.TrackError("sync", err)
		return err
	}
	if err := checkInterrupted(ctx); err != nil {
		return err
	}
	if prune {
		slog.Debug("pruning orphan resources")
		if err := eng.Prune(ctx); err != nil {
			telemetry.TrackError("sync-prune", err)
			return err
		}
		if err := checkInterrupted(ctx); err != nil {
			return err
		}
	}

	duration := time.Since(start)
	status, err := eng.Collect(ctx)
	if err != nil {
		slog.Debug("post-sync status collection failed; skipping summary", "err", err)
	} else {
		var rerr error
		if effectiveOutputFormat() == "verbose" {
			rerr = render.RenderSyncSummary(os.Stdout, plan, status, duration)
		} else {
			rerr = render.RenderSyncBrief(os.Stdout, plan, status, duration)
		}
		if rerr != nil {
			slog.Debug("rendering sync summary failed", "err", rerr)
		}
	}

	if err := eng.Hooks().RunPostSync(ctx, plan); err != nil {
		fmt.Fprintln(os.Stderr, "⚠ post-sync hook failed")
		telemetry.TrackError("sync-hook-post", err)
		return err
	}

	telemetry.Track("sync")
	telemetry.TrackFirstSync(0)
	return nil
}

// checkInterrupted reports a "sync interrupted" ExitCodeError when the
// signal context has been cancelled (Ctrl-C / SIGTERM). Sync used to
// either claim success after a partial run or surface a misleading
// "context canceled" error from the next step; this short-circuits with
// exit 130 (SIGINT convention) and a clear stderr message instead. #126.
func checkInterrupted(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	fmt.Fprintln(os.Stderr, "⚠ sync interrupted — partial state may exist on disk")
	return &ExitCodeError{Code: 130}
}

// warnMissingTools prints a compact stderr banner listing required tools that
// are missing from PATH. Sync never blocks on missing tools — full attribution
// and exit-code semantics live in `gaal doctor`. The per-line rule: show the
// install hint when set, otherwise show "(required by <source>)".
func warnMissingTools(cfg *config.Config) {
	statuses := tools.Check(tools.Collect(cfg))
	missing := statuses[:0]
	for _, st := range statuses {
		if !st.Found {
			missing = append(missing, st)
		}
	}
	if len(missing) == 0 {
		return
	}

	maxName := 0
	for _, st := range missing {
		if n := len(st.Entry.Tool.Name); n > maxName {
			maxName = n
		}
	}

	fmt.Fprintln(os.Stderr, "⚠ Required tools missing from PATH:")
	for _, st := range missing {
		detail := fmt.Sprintf("(required by %s)", st.Entry.Source)
		if st.Entry.Tool.Hint != "" {
			detail = st.Entry.Tool.Hint
		}
		fmt.Fprintf(os.Stderr, "    %-*s  — %s\n", maxName, st.Entry.Tool.Name, detail)
	}
	fmt.Fprintln(os.Stderr, "  Run `gaal doctor` for details.")
}
