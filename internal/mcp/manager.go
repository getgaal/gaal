package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"gaal/internal/config"
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
	Err     error
}

// Manager handles MCP server configuration files.
type Manager struct {
	mcps []config.MCPConfig
}

// NewManager creates a new MCP manager.
func NewManager(mcps []config.MCPConfig) *Manager {
	return &Manager{mcps: mcps}
}

// Sync applies every MCP configuration entry.
func (m *Manager) Sync(ctx context.Context) error {
	for _, mc := range m.mcps {
		if err := m.syncOne(ctx, mc); err != nil {
			return fmt.Errorf("mcp %q: %w", mc.Name, err)
		}
	}
	return nil
}

func (m *Manager) syncOne(ctx context.Context, mc config.MCPConfig) error {
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

	return mergeIntoTarget(mc.Target, mc.Name, entry)
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

// mergeIntoTarget reads the target JSON file, upserts the named entry, and writes it back.
func mergeIntoTarget(target, name string, entry serverEntry) error {
	slog.Debug("merging mcp entry into target", "name", name, "target", target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Load existing document (or start fresh).
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(target); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", target, err)
		}
	}

	// Get or initialise the mcpServers key.
	servers := map[string]serverEntry{}
	if existing, ok := raw["mcpServers"]; ok {
		if err := json.Unmarshal(existing, &servers); err != nil {
			return fmt.Errorf("parsing mcpServers: %w", err)
		}
	}

	servers[name] = entry

	updated, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	raw["mcpServers"] = updated

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(target, out, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing config %s: %w", target, err)
	}

	slog.Info("mcp config updated", "name", name, "target", target)
	return nil
}

// Status returns the presence state of every MCP entry.
func (m *Manager) Status(_ context.Context) []Status {
	slog.Debug("checking mcp status", "count", len(m.mcps))
	statuses := make([]Status, 0, len(m.mcps))

	for _, mc := range m.mcps {
		st := Status{Name: mc.Name, Target: mc.Target}

		data, err := os.ReadFile(mc.Target)
		if err != nil {
			if os.IsNotExist(err) {
				statuses = append(statuses, st) // not present
				continue
			}
			st.Err = err
			statuses = append(statuses, st)
			continue
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			st.Err = fmt.Errorf("parsing %s: %w", mc.Target, err)
			statuses = append(statuses, st)
			continue
		}

		if serversRaw, ok := raw["mcpServers"]; ok {
			var servers map[string]serverEntry
			if err := json.Unmarshal(serversRaw, &servers); err == nil {
				_, st.Present = servers[mc.Name]
			}
		}

		statuses = append(statuses, st)
	}

	return statuses
}
