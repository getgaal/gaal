package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"gaal/internal/secfile"
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
	// Decode the existing file preserving top-level key order so the
	// rewrite doesn't churn the user's tracked dotfile (e.g. ~/.claude.json
	// holds session state, MRU lists, projects alongside mcpServers; map
	// iteration would scramble those keys on every sync). #122.
	keys, raw, err := readOrderedJSON(path)
	if err != nil {
		return fmt.Errorf("parsing existing config %s: %w", path, err)
	}

	serversBytes, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	if _, ok := raw["mcpServers"]; !ok {
		keys = append(keys, "mcpServers")
	}
	raw["mcpServers"] = serversBytes

	out, err := writeOrderedJSON(keys, raw, "  ")
	if err != nil {
		return err
	}
	return secfile.Write(path, out)
}

// readOrderedJSON parses a JSON object and returns its top-level keys in
// document order plus a name → raw value map. A missing or empty file
// yields an empty key list and empty map, not an error.
func readOrderedJSON(path string) ([]string, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]json.RawMessage{}, nil
		}
		return nil, nil, err
	}
	if len(data) == 0 {
		return nil, map[string]json.RawMessage{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, fmt.Errorf("expected JSON object at root, got %v", tok)
	}
	keys := []string{}
	values := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected string key, got %T", kt)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		values[key] = raw
	}
	return keys, values, nil
}

// writeOrderedJSON re-emits a JSON object in the supplied key order with
// per-level indentation (typically two spaces). Each value is run through
// json.Indent so nested objects/arrays land at the right depth.
func writeOrderedJSON(keys []string, values map[string]json.RawMessage, indent string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range keys {
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.WriteString(indent)
		buf.Write(kb)
		buf.WriteString(": ")
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, values[k], indent, indent); err != nil {
			return nil, fmt.Errorf("re-indenting %s: %w", k, err)
		}
		buf.Write(pretty.Bytes())
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
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
	return secfile.Write(path, out)
}

// decodeTOMLEntry converts a parsed TOML table into a serverEntry, normalising
// the args slice (TOML decodes arrays as []any) and env table types.
func decodeTOMLEntry(t map[string]any) serverEntry {
	slog.Debug("decoding toml mcp entry", "fields", len(t))
	var e serverEntry
	if v, ok := t["type"].(string); ok {
		e.Type = v
	}
	if v, ok := t["command"].(string); ok {
		e.Command = v
	}
	if v, ok := t["url"].(string); ok {
		e.URL = v
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
	if rawHeaders, ok := t["headers"].(map[string]any); ok && len(rawHeaders) > 0 {
		e.Headers = decodeStringMap(rawHeaders)
	}
	if rawHeaders, ok := t["http_headers"].(map[string]any); ok && len(rawHeaders) > 0 {
		e.HTTPHeaders = decodeStringMap(rawHeaders)
	}
	if rawHeaders, ok := t["env_http_headers"].(map[string]any); ok && len(rawHeaders) > 0 {
		e.EnvHTTPHeaders = decodeStringMap(rawHeaders)
	}
	return e
}

// encodeTOMLEntry produces the inverse of decodeTOMLEntry, omitting empty
// fields so the rendered TOML stays minimal.
func encodeTOMLEntry(e serverEntry) map[string]any {
	slog.Debug("encoding toml mcp entry", "type", e.Type, "command", e.Command != "", "url", e.URL != "")
	out := map[string]any{}
	if e.Type != "" {
		out["type"] = e.Type
	}
	if e.Command != "" {
		out["command"] = e.Command
	}
	if e.URL != "" {
		out["url"] = e.URL
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
	if len(e.Headers) > 0 {
		out["headers"] = encodeStringMap(e.Headers)
	}
	if len(e.HTTPHeaders) > 0 {
		out["http_headers"] = encodeStringMap(e.HTTPHeaders)
	}
	if len(e.EnvHTTPHeaders) > 0 {
		out["env_http_headers"] = encodeStringMap(e.EnvHTTPHeaders)
	}
	return out
}

func decodeStringMap(raw map[string]any) map[string]string {
	slog.Debug("decoding string map", "count", len(raw))
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func encodeStringMap(in map[string]string) map[string]any {
	slog.Debug("encoding string map", "count", len(in))
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
