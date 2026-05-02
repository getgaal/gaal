//go:build e2e

package e2e

import (
	"path"
	"strings"
	"testing"
)

// TestMutate_AddSkill_PropagatesOnResync covers the most common edit:
// add a new skill entry, re-sync (without --prune), the new skill appears.
func TestMutate_AddSkill_PropagatesOnResync(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
	AssertSkillAbsent(t, env, "claude-code", ScopeProject, "second-skill")

	cfg2 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		AddSkill(localSkillsRoot+"/second-skill", []string{"claude-code"}, false).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "second-skill")
}

// TestMutate_AddMCP_PropagatesOnResync verifies a new MCP entry is upserted
// without disturbing existing entries.
func TestMutate_AddMCP_PropagatesOnResync(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")

	cfg2 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		AddMCP("git", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-git"}, nil).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync")

	AssertMCPEntry(t, env, "claude-code", ScopeGlobal, "filesystem", MCPExpect{Command: "uvx"})
	AssertMCPEntry(t, env, "claude-code", ScopeGlobal, "git", MCPExpect{Command: "uvx"})
	AssertValidJSON(t, env, path.Join(env.home, ".claude.json"))
}

// TestMutate_AddCodexAgent verifies that adding a second agent to an
// existing skill entry causes the skill to land in the new agent's dir
// without disturbing the first.
func TestMutate_AddCodexAgent(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
	AssertSkillAbsent(t, env, "codex", ScopeGlobal, "stub-skill")

	cfg2 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code", "codex"}, true).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync")

	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
	AssertSkillInstalled(t, env, "codex", ScopeGlobal, "stub-skill")
}

// TestMutate_SwitchScope_PopulatesNewLocation flips global: false → true
// and re-syncs. The new global location must receive the skill copy.
//
// Note: prune currently only walks the skill directories the *current*
// config references (see internal/skill/manager.go Prune). A scope flip
// makes the previous project-scope dir no longer present in the
// `expected` map, so the project-scope copy is left in place. The test
// only asserts the new location is populated; the legacy copy is a
// known limitation tracked separately.
func TestMutate_SwitchScope_PopulatesNewLocation(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertSkillInstalled(t, env, "claude-code", ScopeProject, "stub-skill")
	AssertSkillAbsent(t, env, "claude-code", ScopeGlobal, "stub-skill")

	cfg2 := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
}

// TestMutate_SkillContent_ReSyncUpdates ensures the installed copy
// reflects updated source content after a re-sync. The fixture is
// read-only, so the test stages its own scratch source under $HOME with
// the layout gaal expects (a parent directory whose subdirectories each
// contain a SKILL.md). The marker file lives inside the skill's own
// directory, so it travels with the skill on install.
func TestMutate_SkillContent_ReSyncUpdates(t *testing.T) {
	env := newTestEnv(t)

	scratchRoot := path.Join(env.home, "scratch-skills")
	scratchSkill := path.Join(scratchRoot, "stub-skill")
	env.c.MustExec(t, nil, "", "mkdir", "-p", scratchRoot)
	env.c.MustExec(t, nil, "", "cp", "-r", localSkillsRoot+"/stub-skill", scratchSkill)
	env.c.WriteFile(t, path.Join(scratchSkill, "marker.txt"), "v1")

	cfg := newConfig().
		AddSkill(scratchRoot, []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dir, _ := agentSkillDir(env, "claude-code", ScopeProject)
	installedMarker := path.Join(dir, "stub-skill", "marker.txt")
	if got := strings.TrimSpace(env.c.ReadFile(t, installedMarker)); got != "v1" {
		t.Fatalf("expected marker v1 after first sync, got %q", got)
	}

	env.c.WriteFile(t, path.Join(scratchSkill, "marker.txt"), "v2")
	env.mustGaal(t, cfgPath, "sync")

	if got := strings.TrimSpace(env.c.ReadFile(t, installedMarker)); got != "v2" {
		t.Fatalf("expected marker v2 after re-sync, got %q", got)
	}
}

// TestMutate_MCPEnv_ReSyncUpdates verifies that changing inline env vars
// in an MCP entry causes the on-disk config to update on the next sync.
func TestMutate_MCPEnv_ReSyncUpdates(t *testing.T) {
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddMCP("git", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-git"},
			map[string]string{"GIT_AUTHOR_NAME": "ci"},
		).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")
	AssertMCPEntry(t, env, "claude-code", ScopeProject, "git", MCPExpect{
		Env: map[string]string{"GIT_AUTHOR_NAME": "ci"},
	})

	cfg2 := newConfig().
		AddMCP("git", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-git"},
			map[string]string{"GIT_AUTHOR_NAME": "release"},
		).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync")

	AssertMCPEntry(t, env, "claude-code", ScopeProject, "git", MCPExpect{
		Env: map[string]string{"GIT_AUTHOR_NAME": "release"},
	})
}

// TestMutate_StatusReportsCounts smoke-tests the JSON status output so the
// summary-first PR (#107) is provably consumable from end-to-end test code.
// Using -o json keeps the assertion stable across summary-format tweaks.
func TestMutate_StatusReportsCounts(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		AddMCP("filesystem", []string{"claude-code"}, false,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "-o", "json", "status")
	if !strings.Contains(res.Stdout, `"skills"`) || !strings.Contains(res.Stdout, "stub-skill") {
		t.Fatalf("expected status JSON to include skills + stub-skill; got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, `"mcps"`) || !strings.Contains(res.Stdout, "filesystem") {
		t.Fatalf("expected status JSON to include mcps + filesystem; got:\n%s", res.Stdout)
	}
}
