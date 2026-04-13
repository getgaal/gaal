package discover

import (
	"context"
	"log/slog"
	"time"
)

const (
	defaultMaxDepth = 4
	defaultTimeout  = 2 * time.Second
)

// ScanOptions controls the behaviour of Scan.
type ScanOptions struct {
	// MaxDepth limits how deep the workspace walk descends (default: 4).
	MaxDepth int
	// IncludeWorkspace enables the depth-limited workspace FS walk (default: true).
	IncludeWorkspace bool
	// StateDir is the root of the gaal state cache (e.g. ~/.cache/gaal/state).
	// When empty, drift detection is skipped and resources get DriftUnknown.
	StateDir string
	// Timeout caps the total scan duration (default: 2s). A timed-out scan
	// returns whatever results were gathered before the deadline.
	Timeout time.Duration
}

// applyDefaults fills zero-value fields with their defaults.
func applyDefaults(o ScanOptions) ScanOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = defaultMaxDepth
	}
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	return o
}

// Scan discovers resources present on the filesystem and enriches each entry
// with a DriftState derived from the persisted snapshot index.
//
// Two orthogonal domains are scanned:
//   - Predictable (global/user) paths derived from the agent registry: skills and MCPs.
//   - Workspace paths found by a depth-limited FS walk of workDir: skills and repos.
//
// Duplicate paths are silently deduplicated; the first attribution wins.
func Scan(ctx context.Context, home, workDir string, opts ScanOptions) ([]Resource, error) {
	slog.DebugContext(ctx, "scanning filesystem", "home", home, "workDir", workDir)
	opts = applyDefaults(opts)

	scanCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	seen := make(map[string]struct{})
	var results []Resource

	add := func(resources []Resource) {
		for _, r := range resources {
			if r.Path == "" {
				continue
			}
			if _, ok := seen[r.Path]; ok {
				continue
			}
			seen[r.Path] = struct{}{}
			results = append(results, r)
		}
	}

	global, err := scanGlobal(scanCtx, home, workDir, opts.StateDir)
	if err != nil {
		slog.DebugContext(scanCtx, "global scan error", "err", err)
	}
	add(global)

	mcps, err := scanMCPs(scanCtx, home, opts.StateDir)
	if err != nil {
		slog.DebugContext(scanCtx, "mcp scan error", "err", err)
	}
	add(mcps)

	if opts.IncludeWorkspace {
		ws, err := scanWorkspace(scanCtx, workDir, opts.MaxDepth, opts.StateDir)
		if err != nil {
			slog.DebugContext(scanCtx, "workspace scan error", "err", err)
		}
		add(ws)
	}

	return results, nil
}
