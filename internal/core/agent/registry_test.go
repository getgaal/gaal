package agent_test

import (
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
	if info.MCPConfigFile == "" {
		t.Error("expected non-empty MCPConfigFile for claude-code")
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
	if !strings.HasPrefix(dir, home) {
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
	path, ok := agent.MCPConfigPath("claude-code", home)
	if !ok {
		t.Fatal("expected MCPConfigPath to return ok=true for claude-code")
	}
	if strings.HasPrefix(path, "~") {
		t.Errorf("expected ~ to be expanded, got %q", path)
	}
	if !strings.HasPrefix(path, home) {
		t.Errorf("expected path to start with home %q, got %q", home, path)
	}
}

func TestMCPConfigPath_Unknown(t *testing.T) {
	_, ok := agent.MCPConfigPath("no-such-agent", "/home/user")
	if ok {
		t.Error("expected ok=false for unknown agent")
	}
}

func TestMCPConfigPath_EmptyWhenNotSet(t *testing.T) {
	// antigravity has an empty MCPConfigFile.
	_, ok := agent.MCPConfigPath("antigravity", "/home/user")
	if ok {
		t.Error("expected ok=false for agent with empty MCPConfigFile")
	}
}

func TestAllAgents_HaveNonEmptySkillDirs(t *testing.T) {
	home := "/home/testuser"
	for _, name := range agent.Names() {
		info, _ := agent.Lookup(name)
		if info.ProjectSkillsDir == "" {
			t.Errorf("agent %q: empty ProjectSkillsDir", name)
		}
		if info.GlobalSkillsDir == "" {
			t.Errorf("agent %q: empty GlobalSkillsDir", name)
		}
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
		{"~/foo/bar", home + "/foo/bar"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}
	for _, tc := range cases {
		got := agent.ExpandHome(tc.input, home)
		if got != tc.want {
			t.Errorf("ExpandHome(%q, %q) = %q, want %q", tc.input, home, got, tc.want)
		}
	}
}
