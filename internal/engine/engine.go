package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gaal/internal/config"
	"gaal/internal/engine/ops"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Re-exported types from the render sub-package for backward compatibility.
// All cmd/ and test code can continue to use the engine.* names unchanged.
type (
	OutputFormat    = render.OutputFormat
	StatusCode      = render.StatusCode
	StatusReport    = render.StatusReport
	RepoEntry       = render.RepoEntry
	SkillEntry      = render.SkillEntry
	MCPEntry        = render.MCPEntry
	AgentEntry      = render.AgentEntry
	AgentDetail     = render.AgentDetail
	AgentPath       = render.AgentPath
	AuditReport     = render.AuditReport
	AuditSkillEntry = render.AuditSkillEntry
	AuditMCPEntry   = render.AuditMCPEntry
	PlanReport      = render.PlanReport
)

// Re-exported constants from the render sub-package.
const (
	FormatText  OutputFormat = render.FormatText
	FormatTable OutputFormat = render.FormatTable
	FormatJSON  OutputFormat = render.FormatJSON

	StatusOK        StatusCode = render.StatusOK
	StatusDirty     StatusCode = render.StatusDirty
	StatusNotCloned StatusCode = render.StatusNotCloned
	StatusPartial   StatusCode = render.StatusPartial
	StatusPresent   StatusCode = render.StatusPresent
	StatusAbsent    StatusCode = render.StatusAbsent
	StatusUnmanaged StatusCode = render.StatusUnmanaged
	StatusError     StatusCode = render.StatusError
)

// Options allows overriding runtime directories (useful for sandbox/test runs).
type Options struct {
	// WorkDir overrides the current working directory used for project-relative
	// skill install paths. Defaults to os.Getwd().
	WorkDir string
	// StateDir overrides the directory used to persist snapshot indexes.
	// Defaults to <cacheRoot>/gaal/state.
	StateDir string
	// Force skips the agent-installation check for wildcard agent lists so
	// that skills are installed into every registered agent regardless of
	// whether the agent's config directory is already present on the machine.
	Force bool
}

// Engine orchestrates repository, skill and MCP synchronisation.
type Engine struct {
	cfg       *config.Config
	repos     *repo.Manager
	skills    *skill.Manager
	mcps      *mcp.Manager
	home      string
	workDir   string
	cacheRoot string
	stateDir  string
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
	//
	//	Linux   : $XDG_CACHE_HOME or ~/.cache
	//	macOS   : ~/Library/Caches
	//	Windows : %LocalAppData%
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = filepath.Join(home, ".cache")
	}
	cacheDir := filepath.Join(cacheRoot, "gaal", "skills")

	stateDir := filepath.Join(cacheRoot, "gaal", "state")
	if opts.StateDir != "" {
		stateDir = opts.StateDir
	}

	slog.Debug("engine initialised", "home", home, "workDir", workDir, "cacheDir", cacheDir, "stateDir", stateDir)

	return &Engine{
		cfg:       cfg,
		repos:     repo.NewManager(cfg.Repositories, stateDir),
		skills:    skill.NewManager(cfg.Skills, cacheDir, home, workDir, stateDir, opts.Force),
		mcps:      mcp.NewManager(cfg.MCPs, stateDir),
		home:      home,
		workDir:   workDir,
		cacheRoot: cacheRoot,
		stateDir:  stateDir,
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

// Prune removes skills and MCP entries that are on disk but no longer declared
// in the configuration. It is intended to be called after RunOnce (sources are
// already cached so no network access occurs). Repositories are never pruned
// automatically — deletion of source trees requires explicit user action.
func (e *Engine) Prune(ctx context.Context) error {
	slog.DebugContext(ctx, "pruning orphan resources")
	var errs []error
	if err := e.skills.Prune(ctx); err != nil {
		errs = append(errs, fmt.Errorf("skills prune: %w", err))
	}
	if err := e.mcps.Prune(ctx); err != nil {
		errs = append(errs, fmt.Errorf("mcps prune: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("prune errors: %v", errs)
	}
	return nil
}

// Collect gathers the current status of all resources without side effects.
func (e *Engine) Collect(ctx context.Context) (*render.StatusReport, error) {
	return ops.Collect(ctx, e.repos, e.skills, e.mcps, e.home, e.workDir, e.stateDir)
}

// DryRun computes what sync would do and renders the plan to os.Stdout.
// It returns the plan so the caller can inspect HasChanges / HasErrors for
// exit code logic.
func (e *Engine) DryRun(ctx context.Context, format OutputFormat) (*render.PlanReport, error) {
	return ops.RenderPlan(ctx, e.repos, e.skills, e.mcps, e.home, e.workDir, e.stateDir, format)
}

// Status collects the current resource state and renders it to os.Stdout.
func (e *Engine) Status(ctx context.Context, format OutputFormat) error {
	return ops.Status(ctx, e.repos, e.skills, e.mcps, e.home, e.workDir, e.stateDir, format)
}

// Audit discovers all skills and MCP servers installed on the machine and
// renders the result to stdout using the requested format.
func (e *Engine) Audit(ctx context.Context, format OutputFormat) error {
	return ops.Audit(ctx, e.home, e.workDir, format)
}

// Info renders a detailed view for the given package type to stdout.
func (e *Engine) Info(ctx context.Context, pkg, filter string, format OutputFormat) error {
	return ops.Info(ctx, e.repos, e.skills, e.mcps, e.cfg, e.home, e.workDir, e.stateDir, pkg, filter, format)
}

// ListAgents returns all registered agents with installed-detection.
func (e *Engine) ListAgents() ([]render.AgentEntry, error) {
	return ops.ListAgents(e.home, e.workDir)
}

// AgentDetail returns the full detail view for a single agent.
func (e *Engine) AgentDetail(name string) (*render.AgentDetail, error) {
	return ops.AgentDetail(e.home, e.workDir, name)
}

// Init writes the documented gaal.yaml skeleton to dest.
func (e *Engine) Init(dest string, force bool) error {
	return ops.Init(dest, force)
}

// BuildImportCandidates scans the machine for installed skills and MCP servers
// filtered by the given scope. The result is intended for the init wizard.
func (e *Engine) BuildImportCandidates(ctx context.Context, scope ops.Scope) (ops.Candidates, error) {
	return ops.BuildImportCandidates(ctx, scope, e.home, e.workDir, e.cacheRoot)
}

// InitFromPlan writes a gaal.yaml populated from the user-selected plan.
func (e *Engine) InitFromPlan(dest string, plan ops.Plan, force bool) error {
	return ops.InitFromPlan(dest, plan, force)
}

// Migrate validates the configuration and returns a summary of what would be
// migrated to a Community Edition instance. The actual migration is not yet
// implemented — this stub reserves the CLI surface.
func (e *Engine) Migrate(target, url string, dryRun bool) (*ops.MigrateResult, error) {
	return ops.Migrate(e.cfg, target, url, dryRun)
}

// Doctor runs sanity checks and returns a diagnostic report.
func (e *Engine) Doctor(opts ops.DoctorOptions) *ops.DoctorReport {
	return ops.RunDoctor(e.cfg, opts)
}
