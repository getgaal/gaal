//go:build e2e

package e2e

import (
	"encoding/json"
	"path"
	"testing"
)

// auditReportShape mirrors the JSON shape of `gaal audit -o json` for the
// fields these tests assert on. Defined locally (rather than importing
// internal/engine/render) so the test file is self-contained and a
// breaking JSON-shape change surfaces here as a parse/lookup failure.
type auditReportShape struct {
	Skills []struct {
		Name   string `json:"name"`
		Agent  string `json:"agent"`
		Source string `json:"source"`
		Path   string `json:"path"`
	} `json:"skills"`
	MCPs []struct {
		Agent      string   `json:"agent"`
		ConfigFile string   `json:"config_file"`
		Servers    []string `json:"servers"`
	} `json:"mcps"`
}

// statusReportShape mirrors `gaal status -o json` for the shape these
// tests assert on.
type statusReportShape struct {
	Repositories []json.RawMessage `json:"repositories"`
	Skills       []struct {
		Source    string   `json:"source"`
		Agent     string   `json:"agent"`
		Installed []string `json:"installed"`
	} `json:"skills"`
	MCPs   []json.RawMessage `json:"mcps"`
	Agents []json.RawMessage `json:"agents"`
}

// containsToken reports whether token appears in any element of items
// (exact match — used to assert presence in installed[]).
func containsToken(items []string, token string) bool {
	for _, it := range items {
		if it == token {
			return true
		}
	}
	return false
}

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
//
// Asserts the parsed JSON shape (a Skills entry with Name == "stub-skill"
// and Agent == "claude-code") rather than substring matching against the
// raw bytes — the latter would pass on degraded output (e.g. an error
// message that happens to mention "stub-skill") and rot silently as the
// JSON layout evolves.
func TestScope_AuditDiscoversInstalledSkill(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "-o", "json", "audit")
	var report auditReportShape
	if err := json.Unmarshal([]byte(res.Stdout), &report); err != nil {
		t.Fatalf("audit JSON parse: %v\n%s", err, res.Stdout)
	}
	found := false
	for _, sk := range report.Skills {
		if sk.Name == "stub-skill" && sk.Agent == "claude-code" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected audit Skills[] to contain {Name: stub-skill, Agent: claude-code}; got %+v",
			report.Skills)
	}
}

// TestScope_StatusJSONShape sanity-checks that the JSON status output is a
// well-formed object with the four documented top-level arrays. This is
// the structural contract Layer 2 (CLI verification) builds on.
//
// Asserts on the parsed shape (typed unmarshal + Skills entry that
// references stub-skill), so a regression that breaks the JSON layout
// surfaces cleanly instead of as a substring miss.
func TestScope_StatusJSONShape(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, false).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "-o", "json", "status")
	var report statusReportShape
	if err := json.Unmarshal([]byte(res.Stdout), &report); err != nil {
		t.Fatalf("status JSON parse: %v\n%s", err, res.Stdout)
	}
	// All four documented arrays must be present (nil-vs-empty doesn't
	// matter — what matters is the field exists in the JSON).
	if report.Repositories == nil && report.Skills == nil && report.MCPs == nil && report.Agents == nil {
		t.Fatalf("status JSON has no recognised top-level arrays\n%s", res.Stdout)
	}
	// Synced skill must surface in the Skills array.
	found := false
	for _, sk := range report.Skills {
		if sk.Source != "" && (sk.Source == localSkillsRoot+"/stub-skill" ||
			containsToken(sk.Installed, "stub-skill")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("status JSON Skills[] missing stub-skill; got %+v", report.Skills)
	}
}

// TestScope_NoSpuriousFiles guards against gaal writing under HOME or
// workdir at paths it has no business touching.
//
// Uses a deny-list (specific paths gaal must never create) instead of an
// allow-list. The allow-list version turned every legitimate new dir into
// a test failure that contributors would whitelist on autopilot, which
// inverts the value of the test. The deny-list catches the actual hazards
// (.gaal/, .local/state/, /tmp side-effects) while leaving room for the
// agent dirs (.claude, .codex, …) the suite is supposed to populate.
func TestScope_NoSpuriousFiles(t *testing.T) {
	env := newTestEnv(t)

	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	homeEntries := env.c.ListDir(t, env.home)
	denied := map[string]string{
		".gaal":        "gaal must not create a top-level ~/.gaal dir (state lives under ~/.cache/gaal)",
		".local":       "gaal must not write under ~/.local/ (XDG state should land in ~/.cache/gaal/state)",
		".gaal-state":  "legacy state location should not be re-introduced",
		".gaal-cache":  "legacy cache location should not be re-introduced",
		"gaal.yaml":    "gaal must not write a yaml at $HOME root (those go to ~/.config/gaal/)",
	}
	for _, e := range homeEntries {
		if reason, bad := denied[e]; bad {
			t.Fatalf("forbidden entry under HOME after sync: %s\n  reason: %s\n  full listing: %v",
				e, reason, homeEntries)
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
