package skill

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestAgentNames_NonEmpty(t *testing.T) {
	names := AgentNames()
	if len(names) == 0 {
		t.Fatal("expected at least one agent name")
	}
}

func TestAgentNames_ContainsKnownAgents(t *testing.T) {
	names := AgentNames()
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	for _, expected := range []string{"github-copilot", "claude-code", "cursor", "goose"} {
		if _, ok := set[expected]; !ok {
			t.Errorf("expected agent %q in AgentNames()", expected)
		}
	}
}

func TestAgentNames_NoDuplicates(t *testing.T) {
	names := AgentNames()
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			t.Errorf("duplicate agent name: %q", names[i])
		}
	}
}

func TestSkillDir_Known_Global(t *testing.T) {
	home := "/home/testuser"
	dir, ok := SkillDir("claude-code", true, home)
	if !ok {
		t.Fatal("expected ok=true for known agent 'claude-code'")
	}
	if dir == "" {
		t.Fatal("expected non-empty dir")
	}
	if dir[0] == '~' {
		t.Errorf("expected ~ to be expanded, got %q", dir)
	}
}

func TestSkillDir_Known_Project(t *testing.T) {
	dir, ok := SkillDir("claude-code", false, "/home/testuser")
	if !ok {
		t.Fatal("expected ok=true for known agent")
	}
	if dir == "" {
		t.Fatal("expected non-empty dir")
	}
}

func TestSkillDir_Unknown(t *testing.T) {
	_, ok := SkillDir("no-such-agent-xyz", false, "/home/testuser")
	if ok {
		t.Fatal("expected ok=false for unknown agent")
	}
}

func TestExpandHome_POSIX(t *testing.T) {
	home := "/home/alice"
	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~nontilde/path", "~nontilde/path"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := expandHome(tc.input, home)
			if got != tc.want {
				t.Errorf("expandHome(%q, %q) = %q, want %q", tc.input, home, got, tc.want)
			}
		})
	}
}

func TestExpandHome_Windows(t *testing.T) {
	home := `C:\Users\alice`
	got := expandHome(`~\foo\bar`, home)
	want := filepath.Join(home, `foo\bar`)
	if got != want {
		t.Errorf("expandHome Windows: got %q, want %q", got, want)
	}
}
