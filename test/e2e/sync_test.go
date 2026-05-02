//go:build e2e

package e2e

import (
	"path"
	"strings"
	"testing"
)

// localSkillsRoot is the read-only fixtures path inside the container —
// every test that uses a local skill source points its config here.
const localSkillsRoot = fixturesDir + "/skills"

// preparePresentAgents creates the directories that mark each named agent
// as "installed" in the test HOME, so wildcard expansion (agents: ["*"])
// resolves to the expected set without --force. The directory list mirrors
// the registry in internal/core/agent/agents.yaml — adding a new agent
// here means adding the dir its detection check looks for.
func preparePresentAgents(t *testing.T, env *testEnv, scope Scope, names ...string) {
	t.Helper()
	for _, n := range names {
		dir, ok := agentSkillDir(env, n, scope)
		if !ok {
			t.Fatalf("no skill dir mapping for %q", n)
		}
		env.c.MustExec(t, nil, "", "mkdir", "-p", dir)
		// codex MCP target lives under ~/.codex; pre-create so MCP sync
		// can write its TOML.
		if n == "codex" && scope == ScopeGlobal {
			env.c.MustExec(t, nil, "", "mkdir", "-p", path.Join(env.home, ".codex"))
		}
	}
}

// TestSync_SkillToClaudeCode_Project verifies a project-scope skill sync
// to claude-code lands SKILL.md inside .claude/skills/<skill>/ under the
// project working directory.
func TestSync_SkillToClaudeCode_Project(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
}

// TestSync_SkillToClaudeCode_Global mirrors the project-scope test for
// home-relative installs (~/.claude/skills/).
func TestSync_SkillToClaudeCode_Global(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
}

// TestSync_SkillToCodex_Global verifies the codex global-skill path,
// which writes to ~/.codex/skills/. codex has no project-scope skills
// dir per the registry — its supports_generic_project flag delegates
// project-scope writes to .agents/skills (the generic convention) — so
// the project counterpart is covered by TestSync_SkillToGeneric_Project.
func TestSync_SkillToCodex_Global(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"codex"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "codex", ScopeGlobal, "stub-skill")
}

// TestSync_MCPToClaudeCode_Global verifies that an inline MCP entry lands
// in ~/.claude.json under the mcpServers key with command/args preserved.
func TestSync_MCPToClaudeCode_Global(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddMCP("filesystem",
			[]string{"claude-code"}, true,
			"uvx",
			[]string{"mcp-server-filesystem", "/data"},
			nil,
		).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertMCPEntry(t, env, "claude-code", ScopeGlobal, "filesystem", MCPExpect{
		Command: "uvx",
		Args:    []string{"mcp-server-filesystem", "/data"},
	})
	AssertValidJSON(t, env, path.Join(env.home, ".claude.json"))
}

// TestSync_MCPToClaudeCode_Project asserts the workspace-scoped path:
// the entry must land in ./.mcp.json, not ~/.claude.json.
func TestSync_MCPToClaudeCode_Project(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddMCP("filesystem",
			[]string{"claude-code"}, false,
			"uvx",
			[]string{"mcp-server-filesystem", "/data"},
			nil,
		).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertMCPEntry(t, env, "claude-code", ScopeProject, "filesystem", MCPExpect{
		Command: "uvx",
		Args:    []string{"mcp-server-filesystem", "/data"},
	})
	AssertFileAbsent(t, env, path.Join(env.home, ".claude.json"))
	AssertValidJSON(t, env, path.Join(env.workdir, ".mcp.json"))
}

// TestSync_MCPToCodex_Global verifies the TOML codec path: codex stores
// MCP entries in ~/.codex/config.toml under the [mcp_servers.<name>] table.
// This is the regression target for #91.
func TestSync_MCPToCodex_Global(t *testing.T) {
	env := newTestEnv(t)

	// codex's config dir must exist for mergeIntoTarget to write into it.
	env.c.MustExec(t, nil, "", "mkdir", "-p", path.Join(env.home, ".codex"))

	cfg := newConfig().
		AddMCP("git",
			[]string{"codex"}, true,
			"uvx",
			[]string{"mcp-server-git", "--repository", "/data/repo"},
			map[string]string{"GIT_AUTHOR_NAME": "ci"},
		).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertMCPEntry(t, env, "codex", ScopeGlobal, "git", MCPExpect{
		Command: "uvx",
		Args:    []string{"mcp-server-git", "--repository", "/data/repo"},
		Env:     map[string]string{"GIT_AUTHOR_NAME": "ci"},
	})
	tomlPath := path.Join(env.home, ".codex", "config.toml")
	AssertValidTOML(t, env, tomlPath)

	// The TOML body must use the table form gaal writes — sanity-check the
	// surface syntax so a regression to inline tables is caught here.
	body := env.c.ReadFile(t, tomlPath)
	if !strings.Contains(body, "[mcp_servers.git]") {
		t.Fatalf("expected [mcp_servers.git] table in codex config.toml, got:\n%s", body)
	}
}

// TestSync_WildcardAgents_Skills covers the agents: ["*"] expansion: every
// installed agent must receive the skill, but agents whose dir is missing
// must NOT be created as a side effect (the safe wildcard contract).
func TestSync_WildcardAgents_Skills(t *testing.T) {
	env := newTestEnv(t)

	// Mark claude-code AND codex as installed at global scope.
	preparePresentAgents(t, env, ScopeGlobal, "claude-code", "codex")

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"*"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
	AssertSkillInstalled(t, env, "codex", ScopeGlobal, "stub-skill")
	// "generic" was NOT pre-installed → must not pick up the skill.
	AssertSkillAbsent(t, env, "generic", ScopeGlobal, "stub-skill")
}

// TestSync_RealisticSkill_PreservesTree asserts that gaal copies the
// entire skill directory, not just SKILL.md. This is the regression
// target for any change that walks files individually instead of using
// a recursive copy.
func TestSync_RealisticSkill_PreservesTree(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/realistic-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	dir, _ := agentSkillDir(env, "claude-code", ScopeProject)
	skillRoot := path.Join(dir, "realistic-skill")

	for _, rel := range []string{
		"SKILL.md",
		"prompts/main.md",
		"templates/component.tsx",
	} {
		AssertFileExists(t, env, path.Join(skillRoot, rel))
	}
}

// TestSync_SelectFiltersSkills verifies the select: list narrows the
// install set: a single named skill drops in even though the source
// directory only carries that one (so the test doubles as a smoke check
// that select with a single match works at all).
func TestSync_SelectFiltersSkills(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/realistic-skill",
			[]string{"claude-code"}, false, "realistic-skill").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "realistic-skill")
}
