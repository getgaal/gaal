package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// mcpCodec abstracts reading and writing the MCP-server table inside an
// agent's config file. JSON-based agents use the "mcpServers" object at the
// document root; codex uses a TOML "mcp_servers" table inside config.toml
// alongside unrelated keys (model, sandbox, [analytics]…) that must survive
// a round-trip.
type mcpCodec interface {
	// ReadServers returns the current servers map. A missing file or a
	// missing servers table both return (nil, nil); only parse errors are
	// surfaced.
	ReadServers(path string) (map[string]serverEntry, error)

	// WriteServers replaces the servers table with the given map while
	// preserving every other top-level key that already exists in path.
	WriteServers(path string, servers map[string]serverEntry) error
}

// codecFor picks a codec based on the target's file extension. Unknown
// extensions default to JSON, matching every JSON-based MCP client (Claude
// Desktop, VS Code, Cursor…).
func codecFor(path string) mcpCodec {
	if strings.EqualFold(extOf(path), ".toml") {
		return tomlCodec{}
	}
	return jsonCodec{}
}

func extOf(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i:]
	}
	return ""
}

// ── JSON ────────────────────────────────────────────────────────────────────

type jsonCodec struct{}

func (jsonCodec) ReadServers(path string) (map[string]serverEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	serversRaw, ok := raw["mcpServers"]
	if !ok {
		return nil, nil
	}
	var servers map[string]serverEntry
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return nil, fmt.Errorf("parsing mcpServers in %s: %w", path, err)
	}
	return servers, nil
}

func (jsonCodec) WriteServers(path string, servers map[string]serverEntry) error {
	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", path, err)
		}
	}

	updated, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	raw["mcpServers"] = updated

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644) //nolint:gosec
}

// ── TOML ────────────────────────────────────────────────────────────────────

const tomlServersKey = "mcp_servers"

type tomlCodec struct{}

func (tomlCodec) ReadServers(path string) (map[string]serverEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	doc := map[string]any{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	rawTable, ok := doc[tomlServersKey].(map[string]any)
	if !ok {
		return nil, nil
	}

	servers := make(map[string]serverEntry, len(rawTable))
	for name, v := range rawTable {
		entry, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parsing %s: %s.%s is not a table", path, tomlServersKey, name)
		}
		servers[name] = decodeTOMLEntry(entry)
	}
	return servers, nil
}

func (tomlCodec) WriteServers(path string, servers map[string]serverEntry) error {
	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", path, err)
		}
	}

	if len(servers) == 0 {
		delete(doc, tomlServersKey)
	} else {
		table := make(map[string]any, len(servers))
		for name, e := range servers {
			table[name] = encodeTOMLEntry(e)
		}
		doc[tomlServersKey] = table
	}

	out, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return os.WriteFile(path, out, 0o644) //nolint:gosec
}

// decodeTOMLEntry converts a parsed TOML table into a serverEntry, normalising
// the args slice (TOML decodes arrays as []any) and env table types.
func decodeTOMLEntry(t map[string]any) serverEntry {
	var e serverEntry
	if v, ok := t["command"].(string); ok {
		e.Command = v
	}
	if rawArgs, ok := t["args"].([]any); ok {
		e.Args = make([]string, 0, len(rawArgs))
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				e.Args = append(e.Args, s)
			}
		}
	}
	if rawEnv, ok := t["env"].(map[string]any); ok && len(rawEnv) > 0 {
		e.Env = make(map[string]string, len(rawEnv))
		for k, v := range rawEnv {
			if s, ok := v.(string); ok {
				e.Env[k] = s
			}
		}
	}
	return e
}

// encodeTOMLEntry produces the inverse of decodeTOMLEntry, omitting empty
// fields so the rendered TOML stays minimal.
func encodeTOMLEntry(e serverEntry) map[string]any {
	out := map[string]any{}
	if e.Command != "" {
		out["command"] = e.Command
	}
	if len(e.Args) > 0 {
		args := make([]any, len(e.Args))
		for i, a := range e.Args {
			args[i] = a
		}
		out["args"] = args
	}
	if len(e.Env) > 0 {
		env := make(map[string]any, len(e.Env))
		for k, v := range e.Env {
			env[k] = v
		}
		out["env"] = env
	}
	return out
}
