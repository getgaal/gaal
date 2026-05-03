package discover

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gaal/internal/core/agent"
)

// scanMCPs discovers MCP config files by reading each registered agent's
// project_mcp_config_file AND global_mcp_config_file paths directly from
// the filesystem, independent of any gaal.yaml.
//
// Global-scope coverage was missing before #137: the audit / `init`
// import wizard never saw entries living in global config files
// (e.g. ~/.codex/config.toml's [mcp_servers.foo]).
func scanMCPs(ctx context.Context, home, stateDir string) ([]Resource, error) {
	slog.DebugContext(ctx, "scanning MCP config files", "home", home)

	seen := make(map[string]struct{})
	var resources []Resource

	for _, a := range agent.List() {
		if cfgFile, ok := agent.ProjectMCPConfigPath(a.Name, home); ok {
			resources = appendMCPResource(resources, seen, a.Name, cfgFile, ScopeWorkspace, stateDir)
		}
		if cfgFile, ok := agent.GlobalMCPConfigPath(a.Name, home); ok {
			resources = appendMCPResource(resources, seen, a.Name, cfgFile, ScopeGlobal, stateDir)
		}
	}

	return resources, nil
}

// appendMCPResource stats cfgFile and, if it exists and hasn't been seen
// before (some agents share project + global paths), appends a Resource
// for it. Returns the updated slice.
func appendMCPResource(resources []Resource, seen map[string]struct{},
	agentName, cfgFile string, scope Scope, stateDir string,
) []Resource {
	if _, ok := seen[cfgFile]; ok {
		return resources
	}
	seen[cfgFile] = struct{}{}
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		return resources
	}
	return append(resources, Resource{
		Type:  ResourceMCP,
		Scope: scope,
		Path:  cfgFile,
		Name:  agentName,
		Drift: computeMCPDrift(cfgFile, stateDir),
		Meta:  map[string]string{"config_file": cfgFile, "scope": string(scope)},
	})
}

// computeMCPDrift compares the SHA-256 hash of an MCP config file against
// the value stored in the snapshot (Git-inspired fast path: stat first).
func computeMCPDrift(cfgFile, stateDir string) DriftState {
	if stateDir == "" {
		return DriftUnknown
	}
	key := "mcp-" + WorkdirKey(cfgFile)
	snap, err := Load(SnapshotPath(stateDir, key))
	if err != nil || len(snap) == 0 {
		return DriftUnknown
	}

	base := filepath.Base(cfgFile)
	rec, ok := snap[base]
	if !ok {
		return DriftUnknown
	}

	fi, err := os.Stat(cfgFile)
	if err != nil {
		return DriftMissing
	}

	// Fast path: stat matches → assume unchanged.
	if fi.Size() == rec.Size && fi.ModTime().Equal(rec.ModTime) {
		return DriftOK
	}

	// Hash comparison.
	h, err := hashFile(cfgFile)
	if err != nil {
		return DriftUnknown
	}
	if h == rec.Hash {
		// Racy-git: repair snapshot entry.
		snap[base] = FileRecord{Size: fi.Size(), ModTime: fi.ModTime(), Hash: h}
		return DriftOK
	}

	return DriftModified
}
