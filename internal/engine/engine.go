package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gaal/internal/config"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Options allows overriding runtime directories (useful for sandbox/test runs).
type Options struct {
	// WorkDir overrides the current working directory used for project-relative
	// skill install paths. Defaults to os.Getwd().
	WorkDir string
}

// Engine orchestrates repository, skill and MCP synchronisation.
type Engine struct {
	cfg    *config.Config
	repos  *repo.Manager
	skills *skill.Manager
	mcps   *mcp.Manager
}

// New creates an Engine from the given configuration using default directories.
func New(cfg *config.Config) *Engine {
	return NewWithOptions(cfg, Options{})
}

// NewWithOptions creates an Engine, applying directory overrides from opts.
func NewWithOptions(cfg *config.Config, opts Options) *Engine {
	home, _ := os.UserHomeDir()
	workDir, _ := os.Getwd()
	if opts.WorkDir != "" {
		workDir = opts.WorkDir
	}
	// os.UserCacheDir returns the OS-appropriate cache root:
	//   Linux   : $XDG_CACHE_HOME or ~/.cache
	//   macOS   : ~/Library/Caches
	//   Windows : %LocalAppData%
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = filepath.Join(home, ".cache")
	}
	cacheDir := filepath.Join(cacheRoot, "gaal", "skills")

	slog.Debug("engine initialised", "home", home, "workDir", workDir, "cacheDir", cacheDir)

	return &Engine{
		cfg:    cfg,
		repos:  repo.NewManager(cfg.Repositories),
		skills: skill.NewManager(cfg.Skills, cacheDir, home, workDir),
		mcps:   mcp.NewManager(cfg.MCPs),
	}
}

// RunOnce performs a single synchronisation pass.
func (e *Engine) RunOnce(ctx context.Context) error {
	slog.Info("sync started")

	var errs []error

	if len(e.cfg.Repositories) > 0 {
		slog.Info("syncing repositories", "count", len(e.cfg.Repositories))
		if err := e.repos.Sync(ctx); err != nil {
			errs = append(errs, fmt.Errorf("repositories: %w", err))
		}
	}

	if len(e.cfg.Skills) > 0 {
		slog.Info("syncing skills", "count", len(e.cfg.Skills))
		if err := e.skills.Sync(ctx); err != nil {
			errs = append(errs, fmt.Errorf("skills: %w", err))
		}
	}

	if len(e.cfg.MCPs) > 0 {
		slog.Info("syncing MCP configs", "count", len(e.cfg.MCPs))
		if err := e.mcps.Sync(ctx); err != nil {
			errs = append(errs, fmt.Errorf("mcps: %w", err))
		}
	}

	if len(errs) > 0 {
		slog.Error("sync completed with errors", "errors", len(errs))
		return fmt.Errorf("sync errors: %v", errs)
	}

	slog.Info("sync completed successfully")
	return nil
}

// RunService runs synchronisation in a loop until the context is cancelled.
func (e *Engine) RunService(ctx context.Context, interval time.Duration) error {
	slog.Info("service mode started", "interval", interval)

	// Run immediately on startup.
	if err := e.RunOnce(ctx); err != nil {
		slog.Error("initial sync failed", "err", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("service stopping")
			return nil
		case t := <-ticker.C:
			slog.Info("periodic sync triggered", "time", t)
			if err := e.RunOnce(ctx); err != nil {
				slog.Error("periodic sync failed", "err", err)
			}
		}
	}
}

// Status prints a human-readable status report to stdout.
func (e *Engine) Status(ctx context.Context) error {
	fmt.Println("=== Repositories ===")
	for _, st := range e.repos.Status(ctx) {
		if st.Err != nil {
			fmt.Printf("  %-40s ERROR: %v\n", st.Path, st.Err)
			continue
		}
		if !st.Cloned {
			fmt.Printf("  %-40s [not cloned]  %s @ %s\n", st.Path, st.Type, st.URL)
		} else {
			fmt.Printf("  %-40s [ok]          current=%-15s want=%s\n",
				st.Path, st.Current, orDefault(st.Version, "default"))
		}
	}

	fmt.Println()
	fmt.Println("=== Skills ===")
	for _, st := range e.skills.Status(ctx) {
		if st.Err != nil {
			fmt.Printf("  %-30s %-20s ERROR: %v\n", st.Source, st.AgentName, st.Err)
			continue
		}
		fmt.Printf("  %-30s %-20s installed=%v  missing=%v\n",
			st.Source, st.AgentName, st.Installed, st.Missing)
	}

	fmt.Println()
	fmt.Println("=== MCP Configs ===")
	for _, st := range e.mcps.Status(ctx) {
		if st.Err != nil {
			fmt.Printf("  %-25s ERROR: %v\n", st.Name, st.Err)
			continue
		}
		present := "absent"
		if st.Present {
			present = "present"
		}
		fmt.Printf("  %-25s %-10s  %s\n", st.Name, present, st.Target)
	}

	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
