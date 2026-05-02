//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// Scope describes whether a resource is expected at user-global or
// workspace-local scope. Mirrors the engine's notion but kept private so
// the tests are not coupled to the engine package.
type Scope int

const (
	ScopeProject Scope = iota
	ScopeGlobal
)

func (s Scope) String() string {
	if s == ScopeGlobal {
		return "global"
	}
	return "project"
}

// agentSkillDir returns the absolute path to the directory that gaal
// installs <agent>'s skills into for a given (env, scope, agent).
//
// The mapping mirrors internal/core/agent/agents.yaml. The e2e suite
// hard-codes only the agents it actually exercises (claude-code, codex,
// generic) — adding a new one is one line here.
func agentSkillDir(env *testEnv, agent string, scope Scope) (string, bool) {
	switch agent {
	case "claude-code":
		if scope == ScopeGlobal {
			return path.Join(env.home, ".claude", "skills"), true
		}
		return path.Join(env.workdir, ".claude", "skills"), true
	case "codex":
		if scope == ScopeGlobal {
			return path.Join(env.home, ".codex", "skills"), true
		}
		// codex has no project skills dir per agents.yaml — falls through to
		// the generic project convention.
		return path.Join(env.workdir, ".agents", "skills"), true
	case "generic":
		if scope == ScopeGlobal {
			return path.Join(env.home, ".agents", "skills"), true
		}
		return path.Join(env.workdir, ".agents", "skills"), true
	}
	return "", false
}

// agentMCPConfigPath returns the absolute path to the MCP config file
// gaal writes for a given (env, scope, agent). Returns ok=false when the
// agent does not have a config file at the requested scope.
func agentMCPConfigPath(env *testEnv, agent string, scope Scope) (string, bool) {
	switch agent {
	case "claude-code":
		if scope == ScopeGlobal {
			return path.Join(env.home, ".claude.json"), true
		}
		return path.Join(env.workdir, ".mcp.json"), true
	case "codex":
		if scope == ScopeGlobal {
			return path.Join(env.home, ".codex", "config.toml"), true
		}
		// codex has no project-scope MCP config — mirror the registry.
		return "", false
	}
	return "", false
}

// AssertSkillInstalled fails the test when SKILL.md for skillName is not
// found under <agent>'s skill directory at the given scope.
func AssertSkillInstalled(t *testing.T, env *testEnv, agent string, scope Scope, skillName string) {
	t.Helper()
	dir, ok := agentSkillDir(env, agent, scope)
	if !ok {
		t.Fatalf("no skill directory mapping for agent %q (scope=%s)", agent, scope)
	}
	target := path.Join(dir, skillName, "SKILL.md")
	if !env.c.FileExists(t, target) {
		listing := env.c.ListDir(t, dir)
		t.Fatalf("skill %q missing for agent=%s scope=%s\n  expected: %s\n  contents of %s: %v",
			skillName, agent, scope, target, dir, listing)
	}
}

// AssertSkillAbsent is the inverse of AssertSkillInstalled.
func AssertSkillAbsent(t *testing.T, env *testEnv, agent string, scope Scope, skillName string) {
	t.Helper()
	dir, ok := agentSkillDir(env, agent, scope)
	if !ok {
		t.Fatalf("no skill directory mapping for agent %q (scope=%s)", agent, scope)
	}
	target := path.Join(dir, skillName, "SKILL.md")
	if env.c.FileExists(t, target) {
		t.Fatalf("skill %q unexpectedly present for agent=%s scope=%s at %s",
			skillName, agent, scope, target)
	}
}

// AssertFileExists fails when p does not exist in the container.
func AssertFileExists(t *testing.T, env *testEnv, p string) {
	t.Helper()
	if !env.c.FileExists(t, p) {
		t.Fatalf("expected file to exist: %s", p)
	}
}

// AssertFileAbsent fails when p exists in the container.
func AssertFileAbsent(t *testing.T, env *testEnv, p string) {
	t.Helper()
	if env.c.FileExists(t, p) {
		t.Fatalf("expected file to be absent: %s", p)
	}
}

// MCPExpect describes an MCP server entry that gaal is expected to have
// upserted into an agent's config file. Empty fields are not asserted on.
type MCPExpect struct {
	Command string
	Args    []string
	Env     map[string]string
}

// AssertMCPEntry parses the agent's MCP config file (JSON or TOML based
// on extension) and verifies that an entry named serverName matches expect.
func AssertMCPEntry(t *testing.T, env *testEnv, agent string, scope Scope, serverName string, expect MCPExpect) {
	t.Helper()
	cfgPath, ok := agentMCPConfigPath(env, agent, scope)
	if !ok {
		t.Fatalf("agent %q has no MCP config file at scope=%s", agent, scope)
	}
	if !env.c.FileExists(t, cfgPath) {
		t.Fatalf("expected MCP config %s to exist after sync", cfgPath)
	}
	body := env.c.ReadFile(t, cfgPath)

	servers := decodeServers(t, cfgPath, body)
	got, present := servers[serverName]
	if !present {
		names := make([]string, 0, len(servers))
		for n := range servers {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Fatalf("MCP server %q not found in %s\n  configured: %v\n  body: %s",
			serverName, cfgPath, names, body)
	}

	if expect.Command != "" && got.Command != expect.Command {
		t.Fatalf("MCP %q: command mismatch\n  want: %s\n  got:  %s",
			serverName, expect.Command, got.Command)
	}
	if len(expect.Args) > 0 {
		if !slicesEqual(got.Args, expect.Args) {
			t.Fatalf("MCP %q: args mismatch\n  want: %v\n  got:  %v",
				serverName, expect.Args, got.Args)
		}
	}
	for k, v := range expect.Env {
		if got.Env[k] != v {
			t.Fatalf("MCP %q: env[%s] mismatch\n  want: %s\n  got:  %s",
				serverName, k, v, got.Env[k])
		}
	}
}

// AssertMCPAbsent verifies that no entry named serverName appears in the
// agent's MCP config file. A missing config file counts as absent.
func AssertMCPAbsent(t *testing.T, env *testEnv, agent string, scope Scope, serverName string) {
	t.Helper()
	cfgPath, ok := agentMCPConfigPath(env, agent, scope)
	if !ok {
		t.Fatalf("agent %q has no MCP config file at scope=%s", agent, scope)
	}
	if !env.c.FileExists(t, cfgPath) {
		return
	}
	body := env.c.ReadFile(t, cfgPath)
	servers := decodeServers(t, cfgPath, body)
	if _, present := servers[serverName]; present {
		t.Fatalf("MCP server %q unexpectedly present in %s\n  body: %s",
			serverName, cfgPath, body)
	}
}

// AssertValidJSON parses p as JSON and fails the test on any decode error.
// Useful as a smoke check after mutation tests so a sync-then-prune round
// does not leave the agent config in a corrupt state.
func AssertValidJSON(t *testing.T, env *testEnv, p string) {
	t.Helper()
	body := env.c.ReadFile(t, p)
	var any any
	if err := json.Unmarshal([]byte(body), &any); err != nil {
		t.Fatalf("expected %s to be valid JSON: %v\n  body: %s", p, err, body)
	}
}

// AssertValidTOML parses p as TOML and fails the test on any decode error.
func AssertValidTOML(t *testing.T, env *testEnv, p string) {
	t.Helper()
	body := env.c.ReadFile(t, p)
	var any any
	if err := toml.Unmarshal([]byte(body), &any); err != nil {
		t.Fatalf("expected %s to be valid TOML: %v\n  body: %s", p, err, body)
	}
}

// AssertNoStaleSymlinks walks dir and fails the test on any dangling
// symlink. Used by prune scenarios to make sure the install copies are
// not symlinks pointing into the old skill cache. Uses the POSIX-portable
// `-type l ! -exec test -e` chain so the assertion works against busybox
// find (the Alpine base shell), which does not implement GNU find's
// -xtype extension.
func AssertNoStaleSymlinks(t *testing.T, env *testEnv, dir string) {
	t.Helper()
	if !env.c.FileExists(t, dir) {
		return
	}
	res := env.c.MustExec(t, nil, "",
		"sh", "-c",
		fmt.Sprintf(`find %s -type l '!' -exec test -e {} ';' -print 2>/dev/null`, shellQuote(dir)),
	)
	if strings.TrimSpace(res.Stdout) != "" {
		t.Fatalf("stale symlinks under %s:\n%s", dir, res.Stdout)
	}
}

// mcpServer mirrors the on-disk shape stored by gaal in both JSON and
// TOML config files. Kept structurally identical to internal/mcp.serverEntry.
type mcpServer struct {
	Command string            `json:"command,omitempty" toml:"command,omitempty"`
	Args    []string          `json:"args,omitempty"    toml:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"     toml:"env,omitempty"`
}

func decodeServers(t *testing.T, cfgPath, body string) map[string]mcpServer {
	t.Helper()
	if strings.HasSuffix(cfgPath, ".toml") {
		return decodeTOMLServers(t, cfgPath, body)
	}
	return decodeJSONServers(t, cfgPath, body)
}

// decodeJSONServers mirrors internal/mcp/codec.go: the document is a
// JSON object with a top-level "mcpServers" map.
func decodeJSONServers(t *testing.T, cfgPath, body string) map[string]mcpServer {
	t.Helper()
	if strings.TrimSpace(body) == "" {
		return map[string]mcpServer{}
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("parsing %s as JSON: %v\n  body: %s", cfgPath, err, body)
	}
	srvRaw, ok := raw["mcpServers"]
	if !ok {
		return map[string]mcpServer{}
	}
	out := map[string]mcpServer{}
	if err := json.Unmarshal(srvRaw, &out); err != nil {
		t.Fatalf("parsing mcpServers in %s: %v", cfgPath, err)
	}
	return out
}

// decodeTOMLServers mirrors internal/mcp/codec.go: the document carries a
// top-level [mcp_servers] table.
func decodeTOMLServers(t *testing.T, cfgPath, body string) map[string]mcpServer {
	t.Helper()
	if strings.TrimSpace(body) == "" {
		return map[string]mcpServer{}
	}
	doc := map[string]any{}
	if err := toml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("parsing %s as TOML: %v\n  body: %s", cfgPath, err, body)
	}
	tab, ok := doc["mcp_servers"].(map[string]any)
	if !ok {
		return map[string]mcpServer{}
	}
	out := make(map[string]mcpServer, len(tab))
	for name, v := range tab {
		entry, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("mcp_servers.%s in %s is not a table", name, cfgPath)
		}
		var s mcpServer
		if cmd, ok := entry["command"].(string); ok {
			s.Command = cmd
		}
		if args, ok := entry["args"].([]any); ok {
			for _, a := range args {
				if str, ok := a.(string); ok {
					s.Args = append(s.Args, str)
				}
			}
		}
		if env, ok := entry["env"].(map[string]any); ok {
			s.Env = map[string]string{}
			for k, v := range env {
				if str, ok := v.(string); ok {
					s.Env[k] = str
				}
			}
		}
		out[name] = s
	}
	return out
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
