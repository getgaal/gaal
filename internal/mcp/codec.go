package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// mcpCodec abstracts reading and writing the MCP-server table inside an
// agent's config file. JSON-based agents use the "mcpServers" object at the
// document root; codex uses a TOML "mcp_servers" table inside config.toml
// alongside unrelated keys (model, sandbox, [analytics]…) that must survive
// a round-trip; opencode uses a "mcp" table with its own per-entry shape.
// One implementation per format lives in its own file: codec_json.go,
// codec_toml.go, codec_opencode.go (mirrors internal/core/vcs's
// one-backend-per-file convention).
type mcpCodec interface {
	// ReadServers returns the current servers map. A missing file or a
	// missing servers table both return (nil, nil); only parse errors are
	// surfaced.
	ReadServers(path string) (map[string]serverEntry, error)

	// WriteServers replaces the servers table with the given map while
	// preserving every other top-level key that already exists in path.
	WriteServers(path string, servers map[string]serverEntry) error
}

// codecFor picks a codec based on the target's file name. Unknown names
// default to jsonCodec's "mcpServers" shape, matching every JSON-based MCP
// client (Claude Desktop, VS Code, Cursor…). opencode.json/opencode.jsonc
// use a distinct top-level key ("mcp", not "mcpServers") and a distinct
// per-entry shape (type: "local"/"remote", command as an array, environment
// not env) — see agent-capabilities.md and https://opencode.ai/config.json.
func codecFor(path string) mcpCodec {
	if strings.EqualFold(extOf(path), ".toml") {
		return tomlCodec{}
	}
	if isOpencodeConfig(path) {
		return opencodeCodec{}
	}
	return jsonCodec{}
}

// isOpencodeConfig reports whether path's base name is opencode's own
// config file, by name rather than extension since both opencode.json and
// opencode's Claude-compatible-looking neighbors otherwise share ".json".
func isOpencodeConfig(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "opencode.json" || base == "opencode.jsonc"
}

func extOf(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i:]
	}
	return ""
}

// readOrderedJSON parses a JSON object and returns its top-level keys in
// document order plus a name → raw value map. A missing or empty file
// yields an empty key list and empty map, not an error. Shared by
// jsonCodec and opencodeCodec — both need to preserve unrelated top-level
// keys (session state, provider config, …) on write. #122.
func readOrderedJSON(path string) ([]string, map[string]json.RawMessage, error) {
	data, err := readFileOrEmpty(path)
	if err != nil {
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

// readFileOrEmpty reads path, treating a missing file as a zero-length
// result rather than an error — every codec's ReadServers wants "no file
// yet" and "empty file" to behave identically (nil servers map, no error).
func readFileOrEmpty(path string) ([]byte, error) {
	slog.Debug("reading mcp config file", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
