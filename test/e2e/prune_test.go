//go:build e2e

package e2e

import (
	"path"
	"testing"
)

// TestPrune_RemovesOrphanSkill exercises the "drop a skill from config,
// re-sync with --prune" workflow. The skill must be gone from the agent
// directory after the second sync.
func TestPrune_RemovesOrphanSkill(t *testing.T) {
	env := newTestEnv(t)

	// 1. Sync with two skills present.
	cfg1 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		AddSkill(localSkillsRoot+"/second-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "second-skill")

	// 2. Drop second-skill from the config and sync --prune.
	cfg2 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
	AssertSkillAbsent(t, env, "claude-code", ScopeProject, "second-skill")

	// 3. The remaining install must not contain dangling symlinks pointing
	//    into the removed skill's cache copy.
	dir, _ := agentSkillDir(env, "claude-code", ScopeProject)
	AssertNoStaleSymlinks(t, env, dir)
}

// TestPrune_RemovesOrphanMCPEntry covers the JSON codec prune path:
// removing an MCP entry from the config + sync --prune drops the
// matching mcpServers key from .mcp.json while preserving siblings.
func TestPrune_RemovesOrphanMCPEntry(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		AddMCP("git", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-git"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertMCPEntry(t, env, "claude-code", ScopeProject, "filesystem", MCPExpect{Command: "uvx"})
	AssertMCPEntry(t, env, "claude-code", ScopeProject, "git", MCPExpect{Command: "uvx"})

	cfg2 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	AssertMCPEntry(t, env, "claude-code", ScopeProject, "filesystem", MCPExpect{Command: "uvx"})
	AssertMCPAbsent(t, env, "claude-code", ScopeProject, "git")
	AssertValidJSON(t, env, path.Join(env.workdir, ".mcp.json"))
}

// TestPrune_RemovesOrphanMCPEntry_TOML is the codex/TOML twin of the JSON
// test above. The codec branch is selected purely by file extension so it
// matters that both paths get a regression test.
func TestPrune_RemovesOrphanMCPEntry_TOML(t *testing.T) {
	env := newTestEnv(t)
	env.c.MustExec(t, nil, "", "mkdir", "-p", path.Join(env.home, ".codex"))

	cfg1 := newConfig().
		AddMCP("git", []string{"codex"}, true,
			"uvx", []string{"mcp-server-git"}, nil).
		AddMCP("filesystem", []string{"codex"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertMCPEntry(t, env, "codex", ScopeGlobal, "git", MCPExpect{Command: "uvx"})
	AssertMCPEntry(t, env, "codex", ScopeGlobal, "filesystem", MCPExpect{Command: "uvx"})

	cfg2 := newConfig().
		AddMCP("git", []string{"codex"}, true,
			"uvx", []string{"mcp-server-git"}, nil).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	AssertMCPEntry(t, env, "codex", ScopeGlobal, "git", MCPExpect{Command: "uvx"})
	AssertMCPAbsent(t, env, "codex", ScopeGlobal, "filesystem")
	AssertValidTOML(t, env, path.Join(env.home, ".codex", "config.toml"))
}

// TestPrune_AcrossMultipleAgents removes a skill that was synced to two
// agents and verifies it disappears from both.
func TestPrune_AcrossMultipleAgents(t *testing.T) {
	env := newTestEnv(t)
	preparePresentAgents(t, env, ScopeGlobal, "claude-code", "codex")

	cfg1 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code", "codex"}, true).
		AddSkill(localSkillsRoot+"/second-skill", []string{"claude-code", "codex"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "second-skill")
	AssertSkillInstalled(t, env, "codex", ScopeGlobal, "second-skill")

	cfg2 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code", "codex"}, true).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	AssertSkillAbsent(t, env, "claude-code", ScopeGlobal, "second-skill")
	AssertSkillAbsent(t, env, "codex", ScopeGlobal, "second-skill")
}
