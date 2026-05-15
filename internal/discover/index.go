package discover

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// IndexEntry represents a single discovered resource cached in the discovery index.
type IndexEntry struct {
	Path        string       `json:"path"`
	Type        ResourceType `json:"type"`
	Scope       Scope        `json:"scope"`
	Name        string       `json:"name"`
	Agent       string       `json:"agent"`
	ParentMtime time.Time    `json:"parent_mtime"`
}

// DiscoveryIndex is the top-level structure persisted to disk.
type DiscoveryIndex struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Entries     []IndexEntry `json:"entries"`
}

// IndexPath returns the absolute path to the discovery index file.
func IndexPath(cacheRoot string) string {
	return filepath.Join(cacheRoot, "gaal", "discovery-index.json")
}

// LoadIndex reads the discovery index from path.
// Returns (nil, nil) when the file does not exist.
func LoadIndex(path string) (*DiscoveryIndex, error) {
	slog.Debug("loading discovery index", "path", path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var idx DiscoveryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// SaveIndex writes idx to path atomically using a temp file + os.Rename.
func SaveIndex(path string, idx *DiscoveryIndex) error {
	slog.Debug("saving discovery index", "path", path, "entries", len(idx.Entries))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RebuildIndex runs a full scan and persists the result as a DiscoveryIndex.
func RebuildIndex(ctx context.Context, home, workDir, stateDir, cacheRoot string) error {
	slog.DebugContext(ctx, "rebuilding discovery index", "home", home, "workDir", workDir)
	resources, err := Scan(ctx, home, workDir, ScanOptions{
		Mode:             ScanModeFull,
		IncludeWorkspace: true,
		StateDir:         stateDir,
	})
	if err != nil {
		slog.DebugContext(ctx, "scan error during index rebuild", "err", err)
		// non-fatal: persist whatever was collected
	}

	entries := make([]IndexEntry, 0, len(resources))
	for _, r := range resources {
		parent := filepath.Dir(r.Path)
		fi, statErr := os.Stat(parent)
		if statErr != nil {
			slog.DebugContext(ctx, "stat failed for index entry parent", "path", parent, "err", statErr)
			continue
		}
		entries = append(entries, IndexEntry{
			Path:        r.Path,
			Type:        r.Type,
			Scope:       r.Scope,
			Name:        r.Name,
			Agent:       r.Meta["agent"],
			ParentMtime: fi.ModTime(),
		})
	}

	idx := &DiscoveryIndex{
		GeneratedAt: time.Now(),
		Entries:     entries,
	}
	idxPath := IndexPath(cacheRoot)
	if err := SaveIndex(idxPath, idx); err != nil {
		return err
	}
	slog.DebugContext(ctx, "discovery index rebuilt", "entries", len(entries), "path", idxPath)
	return nil
}

// Validate checks each IndexEntry against the current filesystem state.
// For each entry whose parent directory mtime is unchanged, it converts the
// entry back to a Resource. Entries with a changed parent mtime trigger a
// partial re-walk of that directory.
// Returns the slice of valid resources and whether all entries were valid.
func Validate(ctx context.Context, idx *DiscoveryIndex) ([]Resource, bool) {
	slog.DebugContext(ctx, "validating discovery index", "entries", len(idx.Entries))
	allValid := true
	seen := make(map[string]struct{})
	var results []Resource

	add := func(r Resource) {
		if _, ok := seen[r.Path]; ok {
			return
		}
		seen[r.Path] = struct{}{}
		results = append(results, r)
	}

	for _, e := range idx.Entries {
		parent := filepath.Dir(e.Path)
		fi, err := os.Stat(parent)
		if err != nil {
			allValid = false
			continue
		}
		if fi.ModTime().Equal(e.ParentMtime) {
			// Parent directory unchanged — trust the index entry.
			add(Resource{
				Type:  e.Type,
				Scope: e.Scope,
				Path:  e.Path,
				Name:  e.Name,
				Drift: DriftUnknown,
				Meta:  map[string]string{"agent": e.Agent},
			})
			continue
		}
		// Parent mtime changed — partial re-walk for this directory.
		allValid = false
		slog.DebugContext(ctx, "partial re-walk triggered", "parent", parent, "agent", e.Agent)
		switch e.Type {
		case ResourceSkill:
			freshSeenLocal := make(map[string]struct{})
			for _, r := range skillsFromDir(ctx, parent, e.Scope, e.Agent, "", "", freshSeenLocal) {
				add(r)
			}
		case ResourceMCP:
			if _, statErr := os.Stat(e.Path); statErr == nil {
				add(Resource{
					Type:  ResourceMCP,
					Scope: e.Scope,
					Path:  e.Path,
					Name:  e.Name,
					Drift: DriftUnknown,
					Meta:  map[string]string{"agent": e.Agent},
				})
			}
		}
	}
	return results, allValid
}
