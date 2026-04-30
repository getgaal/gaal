package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/discover"
)

// serverEntry mirrors the MCP server JSON structure used by Claude Desktop,
// VS Code and other compatible clients.
type serverEntry struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// mcpServersDoc is the top-level document shape most MCP clients expect.
type mcpServersDoc struct {
	MCPServers map[string]serverEntry `json:"mcpServers"`
	// Extra fields preserved during round-trip.
	Extra map[string]json.RawMessage `json:"-"`
}

// Status describes one MCP entry.
type Status struct {
	Name    string
	Target  string
	Present bool
	Dirty   bool // true when stored config differs from the configured inline entry
	Err     error
}

// Manager handles MCP server configuration files.
type Manager struct {
	mcps     []config.ConfigMcp
	home     string // user home directory for ~/ expansion and agent path resolution
	stateDir string // gaal state root for snapshot writing
}

// NewManager creates a new MCP Manager.
func NewManager(mcps []config.ConfigMcp, home, stateDir string) *Manager {
	slog.Debug("creating mcp manager", "entries", len(mcps), "home", home)
	return &Manager{mcps: mcps, home: home, stateDir: stateDir}
}

// resolvedMCPs expands each ConfigMcp into one or more concrete entries with
// Target resolved from the agent registry. Entries with an explicit Target
// are kept as-is (backward compat, deprecated). Entries without Target use the
// Agents + Global fields to fan out one entry per agent.
func (m *Manager) resolvedMCPs() []config.ConfigMcp {
	slog.Debug("resolving mcp targets", "count", len(m.mcps))
	var out []config.ConfigMcp
	for _, mc := range m.mcps {
		if mc.Target != "" {
			slog.Warn("mcp: 'target' field is deprecated; use 'agents' and 'global' instead",
				"name", mc.Name, "target", mc.Target)
			out = append(out, mc)
			continue
		}

		if len(mc.Agents) == 0 {
			slog.Warn("mcp: no target and no agents configured, entry skipped", "name", mc.Name)
			continue
		}

		agentNames := mc.Agents
		if len(agentNames) == 1 && agentNames[0] == "*" {
			agentNames = agent.Names()
		}

		for _, agentName := range agentNames {
			var (
				target string
				ok     bool
			)
			if mc.Global {
				target, ok = agent.GlobalMCPConfigPath(agentName, m.home)
			} else {
				target, ok = agent.ProjectMCPConfigPath(agentName, m.home)
			}
			if !ok || target == "" {
				slog.Debug("mcp: agent has no mcp config for this scope, skipping",
					"name", mc.Name, "agent", agentName, "global", mc.Global)
				continue
			}
			resolved := mc
			resolved.Target = target
			slog.Debug("mcp: resolved target", "name", mc.Name, "agent", agentName, "target", target, "global", mc.Global)
			out = append(out, resolved)
		}
	}
	return out
}

// Sync applies every MCP configuration entry.
func (m *Manager) Sync(ctx context.Context) error {
	m.warnLegacyCodexJSON()
	for _, mc := range m.resolvedMCPs() {
		if err := m.syncOne(ctx, mc); err != nil {
			return fmt.Errorf("mcp %q: %w", mc.Name, err)
		}
	}
	return nil
}

// warnLegacyCodexJSON surfaces a one-shot notice when a stale ~/.codex/mcp.json
// file exists. Earlier gaal releases wrote MCP entries there, but the official
// Codex CLI only reads ~/.codex/config.toml — the JSON file was a silent
// no-op. Users who upgraded carry both files; the JSON one can be deleted.
func (m *Manager) warnLegacyCodexJSON() {
	if m.home == "" {
		return
	}
	legacy := filepath.Join(m.home, ".codex", "mcp.json")
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	slog.Warn("mcp: stale ~/.codex/mcp.json detected — codex CLI ignores it; safe to delete",
		"path", legacy,
		"hint", "MCP entries are now written to ~/.codex/config.toml")
}

func (m *Manager) syncOne(ctx context.Context, mc config.ConfigMcp) error {
	slog.DebugContext(ctx, "syncing mcp entry", "name", mc.Name, "target", mc.Target)
	var entry serverEntry

	switch {
	case mc.Inline != nil:
		slog.DebugContext(ctx, "mcp inline definition", "name", mc.Name, "command", mc.Inline.Command)
		entry = serverEntry{
			Command: mc.Inline.Command,
			Args:    mc.Inline.Args,
			Env:     mc.Inline.Env,
		}

	case mc.Source != "":
		slog.DebugContext(ctx, "mcp remote source", "name", mc.Name, "url", mc.Source)
		var err error
		entry, err = fetchRemoteEntry(ctx, mc.Source, mc.Name)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("no source or inline config provided")
	}

	if err := mergeIntoTarget(mc.Target, mc.Name, entry); err != nil {
		return err
	}
	m.writeMCPSnapshot(mc.Target)
	return nil
}

// writeMCPSnapshot records the current state of the target config file so that
// discover.computeMCPDrift can apply the fast path on subsequent status checks.
func (m *Manager) writeMCPSnapshot(target string) {
	if m.stateDir == "" {
		return
	}
	slog.Debug("writing mcp snapshot", "target", target)
	rec, err := discover.Record(target)
	if err != nil {
		slog.Warn("mcp snapshot failed", "target", target, "err", err)
		return
	}
	snap := discover.Snapshot{filepath.Base(target): rec}
	key := "mcp-" + discover.WorkdirKey(target)
	if err := discover.Save(discover.SnapshotPath(m.stateDir, key), snap); err != nil {
		slog.Warn("mcp snapshot save failed", "target", target, "err", err)
	}
}

// fetchRemoteEntry downloads a JSON config file and extracts the entry for name.
// If the remote file is a full mcpServers document the matching key is extracted;
// otherwise the whole document is treated as a single server entry.
func fetchRemoteEntry(ctx context.Context, rawURL, name string) (serverEntry, error) {
	slog.DebugContext(ctx, "fetching remote mcp config", "url", rawURL, "name", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return serverEntry{}, fmt.Errorf("building request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return serverEntry{}, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return serverEntry{}, fmt.Errorf("fetching %s: HTTP %d", rawURL, resp.StatusCode)
	}

	// Try to decode as a full mcpServers document first.
	var doc struct {
		MCPServers map[string]serverEntry `json:"mcpServers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return serverEntry{}, fmt.Errorf("decoding JSON: %w", err)
	}

	if len(doc.MCPServers) > 0 {
		if e, ok := doc.MCPServers[name]; ok {
			return e, nil
		}
		// Return all entries merged? — just take the first entry with "name" key.
		for k, e := range doc.MCPServers {
			slog.Warn("mcp: server name not found in remote, using first entry", "wanted", name, "found", k)
			return e, nil
		}
	}

	return serverEntry{}, fmt.Errorf("no server entry found in %s", rawURL)
}

// mergeIntoTarget reads the target config file (JSON or TOML, picked by
// extension), upserts the named entry, and writes it back. If the target's
// parent directory does not already exist the entry is silently skipped:
// sync never creates agent-owned directories as a side effect. A parent
// that exists but is not a directory is reported as an error (user
// misconfiguration).
func mergeIntoTarget(target, name string, entry serverEntry) error {
	slog.Debug("merging mcp entry into target", "name", name, "target", target)

	parent := filepath.Dir(target)
	info, err := os.Stat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("mcp: skipping entry — target parent directory does not exist",
				"name", name, "target", target, "parent", parent)
			return nil
		}
		return fmt.Errorf("stat %s: %w", parent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", parent)
	}

	codec := codecFor(target)
	servers, err := codec.ReadServers(target)
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]serverEntry{}
	}
	servers[name] = entry

	if err := codec.WriteServers(target, servers); err != nil {
		return fmt.Errorf("writing config %s: %w", target, err)
	}

	slog.Debug("mcp config updated", "name", name, "target", target)
	return nil
}

// Prune removes mcpServers entries from each managed target file whose names
// are no longer declared in the config for that target. Entries added manually
// outside of gaal are also removed — callers should only use --prune on files
// they intend to manage exclusively with gaal.
func (m *Manager) Prune(ctx context.Context) error {
	slog.DebugContext(ctx, "pruning orphan mcp entries")

	// Build expected name set per target path.
	keepPerTarget := make(map[string]map[string]struct{})
	for _, mc := range m.resolvedMCPs() {
		if keepPerTarget[mc.Target] == nil {
			keepPerTarget[mc.Target] = make(map[string]struct{})
		}
		keepPerTarget[mc.Target][mc.Name] = struct{}{}
	}

	for target, keep := range keepPerTarget {
		codec := codecFor(target)
		servers, err := codec.ReadServers(target)
		if err != nil {
			slog.Warn("mcp prune: cannot read target", "target", target, "err", err)
			continue
		}
		if len(servers) == 0 {
			continue
		}

		pruned := false
		for name := range servers {
			if _, ok := keep[name]; !ok {
				slog.Info("pruning orphan mcp entry", "name", name, "target", target)
				delete(servers, name)
				pruned = true
			}
		}
		if !pruned {
			continue
		}

		if err := codec.WriteServers(target, servers); err != nil {
			slog.Warn("mcp prune: write error", "target", target, "err", err)
			continue
		}
		// Refresh snapshot so next status check reflects the pruned state.
		m.writeMCPSnapshot(target)
	}

	return nil
}

// Status returns the presence state of every MCP entry.
func (m *Manager) Status(_ context.Context) []Status {
	resolved := m.resolvedMCPs()
	slog.Debug("checking mcp status", "count", len(resolved))
	statuses := make([]Status, 0, len(resolved))

	for _, mc := range resolved {
		st := Status{Name: mc.Name, Target: mc.Target}

		servers, err := codecFor(mc.Target).ReadServers(mc.Target)
		if err != nil {
			st.Err = err
			statuses = append(statuses, st)
			continue
		}
		if stored, found := servers[mc.Name]; found {
			st.Present = true
			// For inline configs we can detect divergence without I/O.
			if mc.Inline != nil {
				want := serverEntry{
					Command: mc.Inline.Command,
					Args:    mc.Inline.Args,
					Env:     mc.Inline.Env,
				}
				st.Dirty = !serverEntryEqual(stored, want)
			}
		}

		statuses = append(statuses, st)
	}

	return statuses
}

// serverEntryEqual reports whether two serverEntry values are semantically equal.
// Nil and empty slices/maps are treated as equivalent.
func serverEntryEqual(a, b serverEntry) bool {
	if a.Command != b.Command {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for k, v := range a.Env {
		if b.Env[k] != v {
			return false
		}
	}
	return true
}

// LoadServers reads the given MCP config file and returns the full inline
// definition of every server, keyed by name. JSON and TOML config files are
// both supported (extension-based dispatch). Returns nil, nil when the file
// does not exist or has no servers entry.
func LoadServers(configFile string) (map[string]config.ConfigMcpItem, error) {
	slog.Debug("loading mcp servers", "file", configFile)
	servers, err := codecFor(configFile).ReadServers(configFile)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, nil
	}
	out := make(map[string]config.ConfigMcpItem, len(servers))
	for name, s := range servers {
		out[name] = config.ConfigMcpItem{
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
		}
	}
	return out, nil
}

// ListServers reads the given MCP config file and returns a sorted list of
// server names. Returns nil, nil when the file does not exist (the agent is
// simply not installed on this machine).
func ListServers(configFile string) ([]string, error) {
	slog.Debug("listing mcp servers", "file", configFile)
	servers, err := codecFor(configFile).ReadServers(configFile)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
