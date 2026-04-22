package agent_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gaal/internal/core/agent"
)

func TestNames_NonEmpty(t *testing.T) {
	names := agent.Names()
	if len(names) == 0 {
		t.Fatal("expected at least one registered agent")
	}
}

func TestNames_NoDuplicates(t *testing.T) {
	names := agent.Names()
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			t.Errorf("duplicate agent name: %q", names[i])
		}
	}
}

func TestNames_ContainsKnownAgents(t *testing.T) {
	set := make(map[string]struct{})
	for _, n := range agent.Names() {
		set[n] = struct{}{}
	}
	for _, want := range []string{"claude-code", "cursor", "github-copilot", "goose", "windsurf"} {
		if _, ok := set[want]; !ok {
			t.Errorf("expected agent %q to be registered", want)
		}
	}
}

func TestLookup_Known(t *testing.T) {
	info, ok := agent.Lookup("claude-code")
	if !ok {
		t.Fatal("expected Lookup to find claude-code")
	}
	if info.ProjectSkillsDir == "" {
		t.Error("expected non-empty ProjectSkillsDir")
	}
	if info.GlobalSkillsDir == "" {
		t.Error("expected non-empty GlobalSkillsDir")
	}
	if info.GlobalMCPConfigFile == "" {
		t.Error("expected non-empty GlobalMCPConfigFile for claude-code")
	}
}

func TestLookup_Unknown(t *testing.T) {
	_, ok := agent.Lookup("no-such-agent-xyz")
	if ok {
		t.Error("expected Lookup to return ok=false for unknown agent")
	}
}

func TestSkillDir_ProjectScope(t *testing.T) {
	dir, ok := agent.SkillDir("claude-code", false, "/home/user")
	if !ok {
		t.Fatal("expected ok=true for claude-code")
	}
	if dir == "" {
		t.Fatal("expected non-empty project skill dir")
	}
	// Project dir must be relative (not start with /).
	if strings.HasPrefix(dir, "/") {
		t.Errorf("expected relative project dir, got %q", dir)
	}
}

func TestSkillDir_GlobalScope(t *testing.T) {
	home := "/home/testuser"
	dir, ok := agent.SkillDir("claude-code", true, home)
	if !ok {
		t.Fatal("expected ok=true for claude-code")
	}
	if dir == "" {
		t.Fatal("expected non-empty global skill dir")
	}
	// ~ must have been expanded.
	if strings.HasPrefix(dir, "~") {
		t.Errorf("expected ~ to be expanded, got %q", dir)
	}
	if !strings.HasPrefix(filepath.ToSlash(dir), filepath.ToSlash(home)) {
		t.Errorf("expected dir to start with home %q, got %q", home, dir)
	}
}

func TestSkillDir_Unknown(t *testing.T) {
	_, ok := agent.SkillDir("no-such-agent", false, "/home/user")
	if ok {
		t.Error("expected ok=false for unknown agent")
	}
}

func TestMCPConfigPath_Known(t *testing.T) {
	home := "/home/testuser"
	path, ok := agent.GlobalMCPConfigPath("claude-code", home)
	if !ok {
		t.Fatal("expected GlobalMCPConfigPath to return ok=true for claude-code")
	}
	if strings.HasPrefix(path, "~") {
		t.Errorf("expected ~ to be expanded, got %q", path)
	}
	if !strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(home)) {
		t.Errorf("expected path to start with home %q, got %q", home, path)
	}
}

func TestMCPConfigPath_Unknown(t *testing.T) {
	_, ok := agent.ProjectMCPConfigPath("no-such-agent", "/home/user")
	if ok {
		t.Error("expected ok=false for unknown agent")
	}
}

func TestMCPConfigPath_EmptyWhenNotSet(t *testing.T) {
	// antigravity has an empty global_mcp_config_file.
	_, ok := agent.GlobalMCPConfigPath("antigravity", "/home/user")
	if ok {
		t.Error("expected ok=false for agent with empty GlobalMCPConfigFile")
	}
}

func TestAllAgents_HaveNonEmptySkillDirs(t *testing.T) {
	home := "/home/testuser"
	for _, name := range agent.Names() {
		info, _ := agent.Lookup(name)
		if info.ProjectSkillsDir == "" && !info.SupportsGenericProject {
			t.Errorf("agent %q: empty ProjectSkillsDir without supports_generic_project", name)
		}
		if info.GlobalSkillsDir == "" && !info.SupportsGenericGlobal {
			t.Errorf("agent %q: empty GlobalSkillsDir without supports_generic_global", name)
		}
		// SkillDir must always resolve to a non-empty path: flagged
		// agents transparently redirect to generic's canonical paths.
		projDir, ok := agent.SkillDir(name, false, home)
		if !ok || projDir == "" {
			t.Errorf("agent %q: SkillDir(false) returned empty or not-ok", name)
		}
		globalDir, ok := agent.SkillDir(name, true, home)
		if !ok || globalDir == "" {
			t.Errorf("agent %q: SkillDir(true) returned empty or not-ok", name)
		}
	}
}

func TestExpandHome_POSIX(t *testing.T) {
	home := "/home/alice"
	cases := []struct{ input, want string }{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}
	for _, tc := range cases {
		got := agent.ExpandHome(tc.input, home)
		if filepath.ToSlash(got) != filepath.ToSlash(tc.want) {
			t.Errorf("ExpandHome(%q, %q) = %q, want %q", tc.input, home, got, tc.want)
		}
	}
}

func TestList_NonEmpty(t *testing.T) {
	list := agent.List()
	if len(list) == 0 {
		t.Fatal("expected at least one entry from List()")
	}
}

func TestList_LengthMatchesNames(t *testing.T) {
	if got, want := len(agent.List()), len(agent.Names()); got != want {
		t.Errorf("List() length = %d, Names() length = %d", got, want)
	}
}

func TestList_Sorted(t *testing.T) {
	list := agent.List()
	for i := 1; i < len(list); i++ {
		if list[i].Name < list[i-1].Name {
			t.Errorf("List() not sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
}

func TestList_InfoMatchesLookup(t *testing.T) {
	for _, e := range agent.List() {
		info, ok := agent.Lookup(e.Name)
		if !ok {
			t.Errorf("List() entry %q not found by Lookup", e.Name)
			continue
		}
		if e.Info.ProjectSkillsDir != info.ProjectSkillsDir ||
			e.Info.GlobalSkillsDir != info.GlobalSkillsDir ||
			e.Info.GlobalMCPConfigFile != info.GlobalMCPConfigFile {
			t.Errorf("List() entry %q Info mismatch: got %+v, want %+v", e.Name, e.Info, info)
		}
	}
}

// ── Generic convention ──────────────────────────────────────────────────────

func TestGenericAgentRegistered(t *testing.T) {
	info, ok := agent.Lookup("generic")
	if !ok {
		t.Fatal("generic agent not registered")
	}
	if info.ProjectSkillsDir != ".agents/skills" {
		t.Errorf("generic ProjectSkillsDir = %q, want .agents/skills", info.ProjectSkillsDir)
	}
	if info.GlobalSkillsDir != "~/.agents/skills" {
		t.Errorf("generic GlobalSkillsDir = %q, want ~/.agents/skills", info.GlobalSkillsDir)
	}
	if info.SupportsGenericProject || info.SupportsGenericGlobal {
		t.Error("generic itself must not set supports_generic_{project,global}=true")
	}
}

func TestGenericProjectFlaggedAgents(t *testing.T) {
	// Every agent that previously pointed its project_skills_dir at
	// .agents/skills must now delegate to generic.
	want := []string{"amp", "antigravity", "cline", "codex", "cursor", "gemini-cli", "opencode", "warp"}
	for _, name := range want {
		info, ok := agent.Lookup(name)
		if !ok {
			t.Errorf("agent %q not registered", name)
			continue
		}
		if !info.SupportsGenericProject {
			t.Errorf("agent %q: expected SupportsGenericProject=true", name)
		}
		if info.ProjectSkillsDir != "" {
			t.Errorf("agent %q: expected empty ProjectSkillsDir, got %q", name, info.ProjectSkillsDir)
		}
	}
}

func TestGenericGlobalFlaggedAgents(t *testing.T) {
	// Only cline and warp previously pointed their global_skills_dir at
	// ~/.agents/skills.
	want := []string{"cline", "warp"}
	for _, name := range want {
		info, ok := agent.Lookup(name)
		if !ok {
			t.Errorf("agent %q not registered", name)
			continue
		}
		if !info.SupportsGenericGlobal {
			t.Errorf("agent %q: expected SupportsGenericGlobal=true", name)
		}
		if info.GlobalSkillsDir != "" {
			t.Errorf("agent %q: expected empty GlobalSkillsDir, got %q", name, info.GlobalSkillsDir)
		}
	}
}

func TestSkillDir_RedirectsProjectFlaggedToGeneric(t *testing.T) {
	// amp is project-only flagged: project dir must redirect, but
	// global dir must still resolve to amp's own canonical path.
	home := "/tmp/fake-home"

	proj, ok := agent.SkillDir("amp", false, home)
	if !ok {
		t.Fatal("SkillDir(amp, project) returned false")
	}
	if proj != ".agents/skills" {
		t.Errorf("SkillDir(amp, project) = %q, want .agents/skills", proj)
	}

	global, ok := agent.SkillDir("amp", true, home)
	if !ok {
		t.Fatal("SkillDir(amp, global) returned false")
	}
	if global != filepath.Join(home, ".config/agents/skills") {
		t.Errorf("SkillDir(amp, global) = %q, want amp's canonical global dir", global)
	}
}

func TestSkillDir_RedirectsBothScopesForCline(t *testing.T) {
	home := "/tmp/fake-home"

	proj, ok := agent.SkillDir("cline", false, home)
	if !ok {
		t.Fatal("SkillDir(cline, project) returned false")
	}
	if proj != ".agents/skills" {
		t.Errorf("SkillDir(cline, project) = %q, want .agents/skills", proj)
	}

	global, ok := agent.SkillDir("cline", true, home)
	if !ok {
		t.Fatal("SkillDir(cline, global) returned false")
	}
	if global != filepath.Join(home, ".agents/skills") {
		t.Errorf("SkillDir(cline, global) = %q, want %q", global, filepath.Join(home, ".agents/skills"))
	}

	warpGlobal, ok := agent.SkillDir("warp", true, home)
	if !ok {
		t.Fatal("SkillDir(warp, global) returned false")
	}
	if warpGlobal != filepath.Join(home, ".agents/skills") {
		t.Errorf("SkillDir(warp, global) = %q, want %q", warpGlobal, filepath.Join(home, ".agents/skills"))
	}
}
