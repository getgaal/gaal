package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/discover"
	"gaal/internal/httpx"
	"gaal/internal/urlx"
)

// serverEntry mirrors the MCP server JSON structure used by Claude Desktop,
// VS Code and other compatible clients.
type serverEntry struct {
	Type           string            `json:"type,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	HTTPHeaders    map[string]string `json:"http_headers,omitempty"`
	EnvHTTPHeaders map[string]string `json:"env_http_headers,omitempty"`
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
	warnOnce sync.Once
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
	m.warnOnce.Do(m.emitConfigWarnings)
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

// emitConfigWarnings surfaces issues that depend on the user's MCP config
// but aren't tied to a single sync entry. Runs at most once per Manager
// (gated by warnOnce) so the messages don't repeat across resolvedMCPs
// calls from Sync, Status, and Prune.
//
// Per-agent behavioural constraints (unsupported platforms, project/global
// MCP scope missing) come from [agent.CollectWarnings] — no hard-coded
// agent names live here.
func (m *Manager) emitConfigWarnings() {
	m.emitBehaviorWarnings()
}

// emitBehaviorWarnings groups the configured MCP entries by scope and
// asks the agent registry for the behaviour mismatches that apply.
//
// Entries using the deprecated `target:` field are skipped here — they
// bypass agent resolution entirely (a separate deprecation warning fires
// inside resolvedMCPs). Entries with neither `target:` nor `agents:` are
// also skipped.
func (m *Manager) emitBehaviorWarnings() {
	var projectAgents, globalAgents []string
	for _, mc := range m.mcps {
		if mc.Target != "" || len(mc.Agents) == 0 {
			continue
		}
		if mc.Global {
			globalAgents = append(globalAgents, mc.Agents...)
		} else {
			projectAgents = append(projectAgents, mc.Agents...)
		}
	}
	warnings := agent.CollectWarnings(runtime.GOOS,
		agent.Group{Scope: agent.ScopeMCPProject, Agents: projectAgents},
		agent.Group{Scope: agent.ScopeMCPGlobal, Agents: globalAgents},
	)
	for _, w := range warnings {
		slog.Warn("mcp: "+w.Msg,
			"agent", w.Agent, "scope", w.Scope, "code", w.Code, "hint", w.Hint)
	}
}

// Sync applies every MCP configuration entry. A failure on one entry does not
// abort the rest — every entry is attempted, and the per-entry errors are
// joined via errors.Join so callers can inspect each underlying cause via
// errors.As / errors.Is.
func (m *Manager) Sync(ctx context.Context) error {
	var errs []error
	for _, mc := range m.resolvedMCPs() {
		if err := m.syncOne(ctx, mc); err != nil {
			errs = append(errs, fmt.Errorf("mcp %q: %w", mc.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) syncOne(ctx context.Context, mc config.ConfigMcp) error {
	slog.DebugContext(ctx, "syncing mcp entry", "name", mc.Name, "target", mc.Target)
	var entry serverEntry

	switch {
	case mc.Inline != nil:
		slog.DebugContext(ctx, "mcp inline definition", "name", mc.Name, "type", mc.Inline.Type, "command", mc.Inline.Command, "url", urlx.Redact(mc.Inline.URL))
		entry = inlineServerEntry(mc.Inline, strings.EqualFold(extOf(mc.Target), ".toml"))

	case mc.Source != "":
		slog.DebugContext(ctx, "mcp remote source", "name", mc.Name, "url", urlx.Redact(mc.Source))
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
	if err := urlx.ValidateRemoteFetchURL(rawURL); err != nil {
		return serverEntry{}, err
	}
	safeURL := urlx.Redact(rawURL)
	slog.DebugContext(ctx, "fetching remote mcp config", "url", safeURL, "name", name)
	req, err := httpx.NewRequest(ctx, http.MethodGet, rawURL)
	if err != nil {
		return serverEntry{}, fmt.Errorf("building request: %w", err)
	}

	resp, err := httpx.Client().Do(req)
	if err != nil {
		return serverEntry{}, fmt.Errorf("fetching %s: %w", safeURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return serverEntry{}, fmt.Errorf("fetching %s: HTTP %d", safeURL, resp.StatusCode)
	}

	// Cap the JSON body so a hostile or runaway server cannot OOM the
	// process. 16 MiB is well above any realistic mcpServers document.
	body := io.LimitReader(resp.Body, httpx.MaxBodyBytes+1)

	// Try to decode as a full mcpServers document first.
	var doc struct {
		MCPServers map[string]serverEntry `json:"mcpServers"`
	}
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
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

	return serverEntry{}, fmt.Errorf("no server entry found in %s", safeURL)
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

// Prune removes mcpServers entries from each managed target file whose
// names are no longer declared in the config for that target.
//
// Per #142, this is gated by an opt-in flag: a target is only pruned
// when at least one ConfigMcp entry pointing at it sets `prune: true`.
// Without that gate `--prune` could silently wipe MCP entries the user
// had added by hand to a shared file (e.g. claude-code's ~/.claude.json,
// codex's ~/.codex/config.toml).
func (m *Manager) Prune(ctx context.Context) error {
	slog.DebugContext(ctx, "pruning orphan mcp entries")

	// Build expected name set + per-target prune-opt-in.
	keepPerTarget := make(map[string]map[string]struct{})
	pruneTarget := make(map[string]bool)
	for _, mc := range m.resolvedMCPs() {
		if keepPerTarget[mc.Target] == nil {
			keepPerTarget[mc.Target] = make(map[string]struct{})
		}
		keepPerTarget[mc.Target][mc.Name] = struct{}{}
		if mc.Prune {
			pruneTarget[mc.Target] = true
		}
	}

	for target, keep := range keepPerTarget {
		if !pruneTarget[target] {
			slog.Debug("mcp prune: target not opted in (set prune: true on at least one entry)",
				"target", target)
			continue
		}
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
				want := inlineServerEntry(mc.Inline, strings.EqualFold(extOf(mc.Target), ".toml"))
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
	slog.Debug("comparing mcp server entries")
	if a.Type != b.Type {
		return false
	}
	if a.Command != b.Command {
		return false
	}
	if a.URL != b.URL {
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
	return stringMapEqual(a.Headers, b.Headers) &&
		stringMapEqual(a.HTTPHeaders, b.HTTPHeaders) &&
		stringMapEqual(a.EnvHTTPHeaders, b.EnvHTTPHeaders)
}

func inlineServerEntry(item *config.ConfigMcpItem, tomlTarget bool) serverEntry {
	slog.Debug("building inline mcp server entry", "type", item.Type, "toml", tomlTarget, "url", urlx.Redact(item.URL))
	entry := serverEntry{
		Command: item.Command,
		Args:    item.Args,
		Env:     item.Env,
		URL:     item.URL,
	}
	typ := item.Type
	if typ == "" {
		if item.URL != "" {
			typ = "http"
		} else {
			typ = "stdio"
		}
	}
	if !tomlTarget && typ != "stdio" {
		entry.Type = typ
	}
	if len(item.Headers) == 0 {
		return entry
	}
	if tomlTarget {
		entry.HTTPHeaders = map[string]string{}
		entry.EnvHTTPHeaders = map[string]string{}
		for name, header := range item.Headers {
			if header.Env != "" {
				entry.EnvHTTPHeaders[name] = header.Env
				continue
			}
			entry.HTTPHeaders[name] = header.Value
		}
		if len(entry.HTTPHeaders) == 0 {
			entry.HTTPHeaders = nil
		}
		if len(entry.EnvHTTPHeaders) == 0 {
			entry.EnvHTTPHeaders = nil
		}
		return entry
	}
	entry.Headers = map[string]string{}
	for name, header := range item.Headers {
		if header.Env != "" {
			entry.Headers[name] = "${" + header.Env + "}"
			continue
		}
		entry.Headers[name] = header.Value
	}
	return entry
}

func stringMapEqual(a, b map[string]string) bool {
	slog.Debug("comparing string maps", "left", len(a), "right", len(b))
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
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
		headers := make(map[string]config.ConfigMcpHeader, len(s.Headers)+len(s.HTTPHeaders)+len(s.EnvHTTPHeaders))
		for k, v := range s.Headers {
			headers[k] = config.ConfigMcpHeader{Value: v}
		}
		for k, v := range s.HTTPHeaders {
			headers[k] = config.ConfigMcpHeader{Value: v}
		}
		for k, v := range s.EnvHTTPHeaders {
			headers[k] = config.ConfigMcpHeader{Env: v}
		}
		if len(headers) == 0 {
			headers = nil
		}
		out[name] = config.ConfigMcpItem{
			Type:    s.Type,
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			URL:     s.URL,
			Headers: headers,
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
