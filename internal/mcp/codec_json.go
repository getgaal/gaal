package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/getgaal/gaal/internal/core/io/secfile"
)

// jsonCodec is the default JSON-based MCP config format: a top-level
// "mcpServers" object, used by Claude Desktop, VS Code, Cursor, and every
// other agent that doesn't have its own distinct shape (see codec_toml.go,
// codec_opencode.go for the exceptions).
type jsonCodec struct{}

func (jsonCodec) ReadServers(path string) (map[string]serverEntry, error) {
	data, err := readFileOrEmpty(path)
	if err != nil {
		return nil, err
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
