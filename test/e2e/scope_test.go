//go:build e2e

package e2e

import (
	"path"
	"strings"
	"testing"
)

// TestScope_Matrix runs the same minimal sync against the (skill, mcp) ×
// (project, global) matrix to confirm the scope flag is plumbed correctly
// for every resource type. Each subtest gets its own HOME, so failures are
// independent and easy to diff.
func TestScope_Matrix(t *testing.T) {
	type expect struct {
		skillScope Scope
		mcpScope   Scope
	}
	cases := []struct {
		name   string
		global bool
		want   expect
	}{
		{"project", false, expect{ScopeProject, ScopeProject}},
		{"global", true, expect{ScopeGlobal, ScopeGlobal}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)

			cfg := newConfig().
				AddSkill(localSkillsRoot+"/stub-skill",
					[]string{"claude-code"}, tc.global).
				AddMCP("filesystem",
					[]string{"claude-code"}, tc.global,
					"uvx",
					[]string{"mcp-server-filesystem", "/data"},
					nil,
				).
				String()
			cfgPath := env.writeProjectConfig(t, cfg)
			env.mustGaal(t, cfgPath, "sync")

			AssertSkillInstalled(t, env, "claude-code", tc.want.skillScope, "stub-skill")
			AssertMCPEntry(t, env, "claude-code", tc.want.mcpScope, "filesystem", MCPExpect{
				Command: "uvx",
			})

			// And the opposite scope must remain empty.
			otherSkillScope := ScopeGlobal
			otherMCPScope := ScopeGlobal
			if tc.global {
				otherSkillScope = ScopeProject
				otherMCPScope = ScopeProject
			}
			AssertSkillAbsent(t, env, "claude-code", otherSkillScope, "stub-skill")
			AssertMCPAbsent(t, env, "claude-code", otherMCPScope, "filesystem")
		})
	}
}

// TestScope_UserConfigLoaded verifies that gaal loads a config from
// $HOME/.config/gaal/config.yaml when no -c is supplied — proof that the
// XDG path resolution (cmd/root.go + internal/config/platform/unix.go)
// works under HOME redirection.
func TestScope_UserConfigLoaded(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	env.writeUserConfig(t, cfg)

	// No -c argument — gaal must walk the user-config search chain and
	// find the file we just wrote.
	res := env.gaal(t, "", "sync")
	if res.ExitCode != 0 {
		t.Fatalf("gaal sync (no -c) failed: exit=%d\n%s", res.ExitCode, res.Combined())
	}
	AssertSkillInstalled(t, env, "claude-code", ScopeGlobal, "stub-skill")
}

// TestScope_DryRunReportsChanges exercises the --dry-run plan path:
// nothing should land on disk, but the command must exit non-zero (1)
// to signal pending changes per cmd/sync.go's contract.
func TestScope_DryRunReportsChanges(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	res := env.gaal(t, cfgPath, "sync", "--dry-run")
	if res.ExitCode != 1 {
		t.Fatalf("expected dry-run exit code 1 (pending changes), got %d\n%s",
			res.ExitCode, res.Combined())
	}
	AssertSkillAbsent(t, env, "claude-code", ScopeProject, "stub-skill")

	// Now actually sync and re-run dry-run — the second run must report
	// no changes and exit 0.
	env.mustGaal(t, cfgPath, "sync")
	res = env.gaal(t, cfgPath, "sync", "--dry-run")
	if res.ExitCode != 0 {
		t.Fatalf("expected post-sync dry-run exit 0 (clean), got %d\n%s",
			res.ExitCode, res.Combined())
	}
}

// TestScope_AuditDiscoversInstalledSkill installs a skill via gaal sync
// then runs `gaal audit -o json` and asserts the discovered tree mentions
// it. This is the regression target for the full sync → audit handoff.
func TestScope_AuditDiscoversInstalledSkill(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "-o", "json", "audit")
	if !strings.Contains(res.Stdout, "stub-skill") {
		t.Fatalf("expected audit JSON to surface stub-skill\n%s", res.Stdout)
	}
}

// TestScope_StatusJSONShape sanity-checks that the JSON status output is a
// well-formed object with the four documented top-level arrays. This is
// the structural contract Layer 2 (CLI verification) builds on.
func TestScope_StatusJSONShape(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "-o", "json", "status")
	for _, key := range []string{`"repositories"`, `"skills"`, `"mcps"`, `"agents"`} {
		if !strings.Contains(res.Stdout, key) {
			t.Fatalf("status JSON missing %s\n%s", key, res.Stdout)
		}
	}

	// And the synced skill must be in the status payload.
	if !strings.Contains(res.Stdout, "stub-skill") {
		t.Fatalf("status JSON missing stub-skill\n%s", res.Stdout)
	}
}

// TestScope_NoSpuriousFiles guards against gaal writing anything under
// HOME or workdir other than the explicitly configured agent paths. A
// regression that, say, accidentally writes under ~/.gaal would show up
// as an unexpected directory entry.
func TestScope_NoSpuriousFiles(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	homeEntries := env.c.ListDir(t, env.home)
	allowed := map[string]struct{}{
		".cache":  {}, // gaal source/state caches
		".claude": {}, // the synced skill's parent
		".config": {}, // potential telemetry/user config
	}
	for _, e := range homeEntries {
		if _, ok := allowed[e]; !ok {
			t.Fatalf("unexpected entry under HOME after sync: %s\n  full listing: %v",
				e, homeEntries)
		}
	}

	// Workdir should only contain the gaal.yaml the test wrote.
	workEntries := env.c.ListDir(t, env.workdir)
	if len(workEntries) != 1 || workEntries[0] != "gaal.yaml" {
		t.Fatalf("unexpected workdir entries after global-scope sync: %v", workEntries)
	}

	// And ~/.claude/skills/stub-skill exists and is a directory.
	dir, _ := agentSkillDir(env, "claude-code", ScopeGlobal)
	if !env.c.IsDir(t, path.Join(dir, "stub-skill")) {
		t.Fatalf("expected synced skill to be a directory at %s", dir)
	}
}
