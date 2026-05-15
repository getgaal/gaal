package agent_test

import (
	"testing"

	"gaal/internal/core/agent"
)

// ── Validate matrix ─────────────────────────────────────────────────────────

func TestBehaviorValidate_Matrix(t *testing.T) {
	full := agent.Behavior{
		Name:               "full",
		SupportsSkills:     true,
		SupportsMCPGlobal:  true,
		SupportsMCPProject: true,
		SupportedPlatforms: nil,
	}
	noSkills := agent.Behavior{
		Name:               "no-skills",
		SupportsSkills:     false,
		SupportsMCPGlobal:  true,
		SupportsMCPProject: true,
	}
	noMCPProject := agent.Behavior{
		Name:               "no-mcp-project",
		SupportsSkills:     true,
		SupportsMCPGlobal:  true,
		SupportsMCPProject: false,
	}
	noMCPGlobal := agent.Behavior{
		Name:               "no-mcp-global",
		SupportsSkills:     true,
		SupportsMCPGlobal:  false,
		SupportsMCPProject: true,
	}
	desktopOnly := agent.Behavior{
		Name:               "desktop-only",
		SupportsSkills:     true,
		SupportsMCPGlobal:  true,
		SupportsMCPProject: true,
		SupportedPlatforms: []string{"darwin", "windows"},
	}

	type want struct {
		codes []agent.WarningCode
	}
	cases := []struct {
		name string
		b    agent.Behavior
		s    agent.Scope
		goos string
		want want
	}{
		// Happy paths — no warnings.
		{"full-skill-project-linux", full, agent.ScopeSkillProject, "linux", want{}},
		{"full-skill-global-linux", full, agent.ScopeSkillGlobal, "linux", want{}},
		{"full-mcp-project-linux", full, agent.ScopeMCPProject, "linux", want{}},
		{"full-mcp-global-linux", full, agent.ScopeMCPGlobal, "linux", want{}},

		// Skills opt-out fires for both skill scopes only.
		{"no-skills-skill-project", noSkills, agent.ScopeSkillProject, "linux",
			want{[]agent.WarningCode{agent.WarnSkillsUnsupported}}},
		{"no-skills-skill-global", noSkills, agent.ScopeSkillGlobal, "linux",
			want{[]agent.WarningCode{agent.WarnSkillsUnsupported}}},
		{"no-skills-mcp-scope-quiet", noSkills, agent.ScopeMCPGlobal, "linux", want{}},

		// MCP-project opt-out is project-only.
		{"no-mcp-project-project", noMCPProject, agent.ScopeMCPProject, "linux",
			want{[]agent.WarningCode{agent.WarnMCPProjectUnsupported}}},
		{"no-mcp-project-global-quiet", noMCPProject, agent.ScopeMCPGlobal, "linux", want{}},

		// MCP-global opt-out is global-only.
		{"no-mcp-global-global", noMCPGlobal, agent.ScopeMCPGlobal, "linux",
			want{[]agent.WarningCode{agent.WarnMCPGlobalUnsupported}}},
		{"no-mcp-global-project-quiet", noMCPGlobal, agent.ScopeMCPProject, "linux", want{}},

		// Platform restriction fires regardless of scope.
		{"desktop-only-on-linux-skill", desktopOnly, agent.ScopeSkillProject, "linux",
			want{[]agent.WarningCode{agent.WarnUnsupportedPlatform}}},
		{"desktop-only-on-linux-mcp", desktopOnly, agent.ScopeMCPGlobal, "linux",
			want{[]agent.WarningCode{agent.WarnUnsupportedPlatform}}},
		{"desktop-only-on-darwin-quiet", desktopOnly, agent.ScopeSkillProject, "darwin", want{}},
		{"desktop-only-on-windows-quiet", desktopOnly, agent.ScopeMCPGlobal, "windows", want{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.b.Validate(tc.s, tc.goos)
			if len(got) != len(tc.want.codes) {
				t.Fatalf("got %d warnings, want %d; warnings=%+v", len(got), len(tc.want.codes), got)
			}
			for i, w := range got {
				if w.Code != tc.want.codes[i] {
					t.Errorf("warning %d: code=%q, want %q", i, w.Code, tc.want.codes[i])
				}
				if w.Agent != tc.b.Name {
					t.Errorf("warning %d: agent=%q, want %q", i, w.Agent, tc.b.Name)
				}
				if w.Scope != tc.s {
					t.Errorf("warning %d: scope=%q, want %q", i, w.Scope, tc.s)
				}
				if w.Msg == "" {
					t.Errorf("warning %d: empty Msg", i)
				}
				if w.Hint == "" {
					t.Errorf("warning %d: empty Hint", i)
				}
			}
		})
	}
}

func TestBehaviorValidate_CombinesPlatformAndScopeWarnings(t *testing.T) {
	b := agent.Behavior{
		Name:               "claude-desktop-like",
		SupportsSkills:     false,
		SupportsMCPGlobal:  true,
		SupportedPlatforms: []string{"darwin", "windows"},
	}
	got := b.Validate(agent.ScopeSkillGlobal, "linux")
	if len(got) != 2 {
		t.Fatalf("got %d warnings, want 2: %+v", len(got), got)
	}
	codes := map[agent.WarningCode]bool{}
	for _, w := range got {
		codes[w.Code] = true
	}
	if !codes[agent.WarnUnsupportedPlatform] {
		t.Error("expected WarnUnsupportedPlatform")
	}
	if !codes[agent.WarnSkillsUnsupported] {
		t.Error("expected WarnSkillsUnsupported")
	}
}

// ── BehaviorFor on the embedded registry ────────────────────────────────────

func TestBehaviorFor_Unknown(t *testing.T) {
	if _, ok := agent.BehaviorFor("no-such-agent-xyz"); ok {
		t.Error("BehaviorFor returned ok=true for unknown agent")
	}
}

func TestBehaviorFor_ClaudeDesktop_HasYAMLOverrides(t *testing.T) {
	b, ok := agent.BehaviorFor("claude-desktop")
	if !ok {
		t.Fatal("BehaviorFor(claude-desktop) returned false")
	}
	if b.SupportsSkills {
		t.Error("claude-desktop should have SupportsSkills=false (set in agents.yaml)")
	}
	if len(b.SupportedPlatforms) == 0 {
		t.Fatal("claude-desktop should have a SupportedPlatforms restriction")
	}
	gotDarwin, gotWindows, gotLinux := false, false, false
	for _, p := range b.SupportedPlatforms {
		switch p {
		case "darwin":
			gotDarwin = true
		case "windows":
			gotWindows = true
		case "linux":
			gotLinux = true
		}
	}
	if !gotDarwin || !gotWindows {
		t.Errorf("expected darwin+windows in SupportedPlatforms, got %v", b.SupportedPlatforms)
	}
	if gotLinux {
		t.Errorf("did not expect linux in SupportedPlatforms, got %v", b.SupportedPlatforms)
	}
}

func TestBehaviorFor_ClaudeCode_HasDefaults(t *testing.T) {
	b, ok := agent.BehaviorFor("claude-code")
	if !ok {
		t.Fatal("BehaviorFor(claude-code) returned false")
	}
	if !b.SupportsSkills {
		t.Error("claude-code should default to SupportsSkills=true")
	}
	if len(b.SupportedPlatforms) != 0 {
		t.Errorf("claude-code should have no platform restriction, got %v", b.SupportedPlatforms)
	}
	if !b.SupportsMCPGlobal {
		t.Error("claude-code has ~/.claude.json — SupportsMCPGlobal should be true")
	}
	if !b.SupportsMCPProject {
		t.Error("claude-code has .mcp.json — SupportsMCPProject should be true")
	}
}

func TestBehaviorFor_StructuralMCPDerivation(t *testing.T) {
	// generic carries no MCP config at all — both flags must be false.
	b, ok := agent.BehaviorFor("generic")
	if !ok {
		t.Fatal("BehaviorFor(generic) returned false")
	}
	if b.SupportsMCPGlobal {
		t.Error("generic has no global_mcp_config_file — SupportsMCPGlobal should be false")
	}
	if b.SupportsMCPProject {
		t.Error("generic has no project_mcp_config_file — SupportsMCPProject should be false")
	}
}

// ── Integration with the live YAML registry ─────────────────────────────────

func TestBehaviorFor_AllRegisteredAgentsAreResolvable(t *testing.T) {
	for _, name := range agent.Names() {
		if _, ok := agent.BehaviorFor(name); !ok {
			t.Errorf("BehaviorFor(%q) returned ok=false for a registered agent", name)
		}
	}
}

func TestBehaviorFor_ClaudeDesktopOnLinux_EmitsBothWarnings(t *testing.T) {
	// Live-registry smoke test: the agents.yaml overrides are picked up
	// end-to-end, so a sync targeting claude-desktop on linux yields
	// both the platform and the skills warnings.
	b, ok := agent.BehaviorFor("claude-desktop")
	if !ok {
		t.Fatal("BehaviorFor(claude-desktop) returned false")
	}
	warnings := b.Validate(agent.ScopeSkillGlobal, "linux")
	codes := map[agent.WarningCode]bool{}
	for _, w := range warnings {
		codes[w.Code] = true
	}
	if !codes[agent.WarnUnsupportedPlatform] {
		t.Error("expected WarnUnsupportedPlatform for claude-desktop on linux")
	}
	if !codes[agent.WarnSkillsUnsupported] {
		t.Error("expected WarnSkillsUnsupported for claude-desktop")
	}
}

// ── CollectWarnings ─────────────────────────────────────────────────────────

func TestCollectWarnings_Empty(t *testing.T) {
	if got := agent.CollectWarnings("linux"); len(got) != 0 {
		t.Errorf("no groups should yield no warnings, got %d", len(got))
	}
}

func TestCollectWarnings_DedupesAcrossScopes(t *testing.T) {
	// claude-desktop in both skill-global and skill-project on linux
	// must emit each fact exactly once (WarnUnsupportedPlatform +
	// WarnSkillsUnsupported), not duplicated per scope.
	got := agent.CollectWarnings("linux",
		agent.Group{Scope: agent.ScopeSkillGlobal, Agents: []string{"claude-desktop"}},
		agent.Group{Scope: agent.ScopeSkillProject, Agents: []string{"claude-desktop"}},
	)
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped warnings, got %d: %+v", len(got), got)
	}
	codes := map[agent.WarningCode]int{}
	for _, w := range got {
		codes[w.Code]++
	}
	if codes[agent.WarnUnsupportedPlatform] != 1 {
		t.Errorf("WarnUnsupportedPlatform count = %d, want 1", codes[agent.WarnUnsupportedPlatform])
	}
	if codes[agent.WarnSkillsUnsupported] != 1 {
		t.Errorf("WarnSkillsUnsupported count = %d, want 1", codes[agent.WarnSkillsUnsupported])
	}
}

func TestCollectWarnings_ExpandsWildcard(t *testing.T) {
	// "*" must surface claude-desktop's behaviour warnings on linux.
	got := agent.CollectWarnings("linux",
		agent.Group{Scope: agent.ScopeSkillGlobal, Agents: []string{"*"}},
	)
	foundSkills := false
	foundPlatform := false
	for _, w := range got {
		if w.Agent == "claude-desktop" {
			switch w.Code {
			case agent.WarnSkillsUnsupported:
				foundSkills = true
			case agent.WarnUnsupportedPlatform:
				foundPlatform = true
			}
		}
	}
	if !foundSkills {
		t.Error("wildcard expansion should surface WarnSkillsUnsupported for claude-desktop")
	}
	if !foundPlatform {
		t.Error("wildcard expansion should surface WarnUnsupportedPlatform for claude-desktop on linux")
	}
}

func TestCollectWarnings_SilentForHappyAgents(t *testing.T) {
	// claude-code on linux carries no behaviour warnings.
	got := agent.CollectWarnings("linux",
		agent.Group{Scope: agent.ScopeSkillProject, Agents: []string{"claude-code", "codex"}},
		agent.Group{Scope: agent.ScopeMCPGlobal, Agents: []string{"claude-code"}},
	)
	if len(got) != 0 {
		t.Errorf("expected no warnings, got %+v", got)
	}
}

func TestCollectWarnings_UnknownAgentIgnored(t *testing.T) {
	got := agent.CollectWarnings("linux",
		agent.Group{Scope: agent.ScopeSkillProject, Agents: []string{"no-such-agent-xyz"}},
	)
	if len(got) != 0 {
		t.Errorf("unknown agent must contribute no warnings, got %+v", got)
	}
}

func TestCollectWarnings_StableOrder(t *testing.T) {
	// Two runs of the same input must produce identical output. With
	// wildcard expansion this is the riskiest sort path.
	first := agent.CollectWarnings("linux",
		agent.Group{Scope: agent.ScopeSkillGlobal, Agents: []string{"*"}},
	)
	for i := 0; i < 3; i++ {
		next := agent.CollectWarnings("linux",
			agent.Group{Scope: agent.ScopeSkillGlobal, Agents: []string{"*"}},
		)
		if len(next) != len(first) {
			t.Fatalf("run %d: length drift %d != %d", i, len(next), len(first))
		}
		for j := range first {
			if first[j].Code != next[j].Code || first[j].Agent != next[j].Agent {
				t.Errorf("run %d slot %d: order drift first=(%s,%s) next=(%s,%s)",
					i, j, first[j].Code, first[j].Agent, next[j].Code, next[j].Agent)
			}
		}
	}
}
