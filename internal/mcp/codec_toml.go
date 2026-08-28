package mcp

import (
	"fmt"
	"log/slog"
	"os"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/getgaal/gaal/internal/core/io/secfile"
)

// tomlCodec is codex's format: a "mcp_servers" table inside config.toml
// alongside unrelated keys (model, sandbox, [analytics]…) that must
// survive a round-trip.
const tomlServersKey = "mcp_servers"

type tomlCodec struct{}

func (tomlCodec) ReadServers(path string) (map[string]serverEntry, error) {
	data, err := readFileOrEmpty(path)
	if err != nil {
		return nil, err
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
