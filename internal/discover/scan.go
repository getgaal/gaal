package discover

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultMaxDepth = 4
	defaultTimeout  = 2 * time.Second
)

// ScanMode controls whether Scan uses the fast-path discovery index or
// performs a full filesystem walk.
type ScanMode int

const (
	// ScanModeIndex uses the persisted discovery index for fast reads.
	// Falls back to ScanModeFull when the index is missing or stale.
	ScanModeIndex ScanMode = iota
	// ScanModeFull performs a complete filesystem walk. Used by audit and
	// write operations (init --import-all). No timeout is applied.
	ScanModeFull
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
	// Only applied when Mode == ScanModeIndex.
	Timeout time.Duration
	// Mode selects the scan strategy (default: ScanModeIndex).
	Mode ScanMode
	// CacheRoot is the base cache directory (e.g. ~/.cache/gaal).
	// Required when Mode == ScanModeIndex to locate the discovery index.
	CacheRoot string
}

// applyDefaults fills zero-value fields with their defaults.
func applyDefaults(o ScanOptions) ScanOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = defaultMaxDepth
	}
	if o.Mode == ScanModeIndex && o.Timeout <= 0 {
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
	slog.DebugContext(ctx, "scanning filesystem", "home", home, "workDir", workDir, "mode", opts.Mode)
	opts = applyDefaults(opts)

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

	if opts.Mode == ScanModeIndex && opts.CacheRoot != "" {
		scanCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		idx, err := LoadIndex(IndexPath(opts.CacheRoot))
		if err == nil && idx != nil {
			resources, _ := Validate(scanCtx, idx)
			if len(resources) > 0 {
				slog.DebugContext(ctx, "serving from discovery index", "entries", len(resources))
				add(resources)
				return results, nil
			}
		}
		slog.DebugContext(ctx, "index unavailable or empty, falling back to full scan")
	}

	// ScanModeFull — concurrent, no timeout.
	var wg sync.WaitGroup
	var globalRes, wsRes []Resource
	var globalErr, wsErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		globalRes, globalErr = scanGlobal(ctx, home, workDir, opts.StateDir)
	}()
	if opts.IncludeWorkspace {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wsRes, wsErr = scanWorkspace(ctx, workDir, opts.MaxDepth, opts.StateDir)
		}()
	}
	wg.Wait()

	if globalErr != nil {
		slog.DebugContext(ctx, "global scan error", "err", globalErr)
	}
	add(globalRes)
	if opts.IncludeWorkspace {
		if wsErr != nil {
			slog.DebugContext(ctx, "workspace scan error", "err", wsErr)
		}
		add(wsRes)
	}
	return results, nil
}
