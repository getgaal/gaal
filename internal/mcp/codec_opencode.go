package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/getgaal/gaal/internal/core/io/secfile"
)

// opencodeCodec is opencode's format: opencode.json/opencode.jsonc use a
// top-level "mcp" key (not "mcpServers"), and each entry is discriminated
// by type: "local" (command as a []string, "environment" not "env") or
// "remote" (url/headers, same shape as jsonCodec). Schema:
// https://opencode.ai/config.json. See agent-capabilities.md.
const opencodeServersKey = "mcp"

type opencodeCodec struct{}

func (opencodeCodec) ReadServers(path string) (map[string]serverEntry, error) {
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
	serversRaw, ok := raw[opencodeServersKey]
	if !ok {
		return nil, nil
	}
	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &rawEntries); err != nil {
		return nil, fmt.Errorf("parsing %s in %s: %w", opencodeServersKey, path, err)
	}
	servers := make(map[string]serverEntry, len(rawEntries))
	for name, raw := range rawEntries {
		var oe opencodeEntry
		if err := json.Unmarshal(raw, &oe); err != nil {
			return nil, fmt.Errorf("parsing %s.%s in %s: %w", opencodeServersKey, name, path, err)
		}
		servers[name] = oe.toServerEntry()
	}
	return servers, nil
}

func (opencodeCodec) WriteServers(path string, servers map[string]serverEntry) error {
	keys, raw, err := readOrderedJSON(path)
	if err != nil {
		return fmt.Errorf("parsing existing config %s: %w", path, err)
	}

	entries := make(map[string]opencodeEntry, len(servers))
	for name, e := range servers {
		entries[name] = opencodeEntryFrom(e)
	}
	serversBytes, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if _, ok := raw[opencodeServersKey]; !ok {
		keys = append(keys, opencodeServersKey)
	}
	raw[opencodeServersKey] = serversBytes

	out, err := writeOrderedJSON(keys, raw, "  ")
	if err != nil {
		return err
	}
	return secfile.Write(path, out)
}

// opencodeEntry mirrors a single opencode.json "mcp" table entry.
// https://opencode.ai/config.json
type opencodeEntry struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
}

// toServerEntry converts an opencode-shaped entry into the shared
// serverEntry used by every other codec (command+args split, env not
// environment).
func (oe opencodeEntry) toServerEntry() serverEntry {
	slog.Debug("decoding opencode mcp entry", "type", oe.Type)
	e := serverEntry{Type: oe.Type, URL: oe.URL, Headers: oe.Headers, Env: oe.Environment}
	if len(oe.Command) > 0 {
		e.Command = oe.Command[0]
		e.Args = oe.Command[1:]
	}
	return e
}

// opencodeEntryFrom produces the inverse of toServerEntry. serverEntry has
// no local/remote discriminator; type is inferred from which fields are set
// (URL present → remote, Command present → local), matching opencode's own
// two entry kinds.
func opencodeEntryFrom(e serverEntry) opencodeEntry {
	slog.Debug("encoding opencode mcp entry", "command", e.Command != "", "url", e.URL != "")
	oe := opencodeEntry{Headers: e.Headers, Environment: e.Env}
	switch {
	case e.URL != "":
		oe.Type = "remote"
		oe.URL = e.URL
	case e.Command != "":
		oe.Type = "local"
		oe.Command = append([]string{e.Command}, e.Args...)
	default:
		oe.Type = e.Type
	}
	return oe
}
