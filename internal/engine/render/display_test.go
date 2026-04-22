package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisplaySkillName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "—"},
		{"anthropics/skills", "anthropics/skills"},
		{"owner/repo", "owner/repo"},
		{"https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"ssh://git@host/path/to/repo", "ssh://git@host/path/to/repo"},
		{"skills", "skills"},
		{"/Users/nls/.claude/plugins/cache/claude-plugins-official/claude-md-management/1.0.0/skills/claude-md-improver", "claude-md-improver"},
		{"/abs/path/to/skill", "skill"},
		{"./local/path/skill", "skill"},
		{"../other/skill-dir", "skill-dir"},
		{"~/skills/my-skill", "my-skill"},
		{"a/b/c", "c"},
	}
	for _, tc := range cases {
		if got := displaySkillName(tc.in); got != tc.want {
			t.Errorf("displaySkillName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDisplayTarget_HomeAbbreviation(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}
	in := filepath.Join(home, ".config", "claude", "claude_desktop_config.json")
	got := displayTarget(in, 0)
	if !strings.HasPrefix(got, "~") {
		t.Errorf("expected ~-abbreviated path, got %q", got)
	}
	if got == in {
		t.Errorf("expected substitution, got unchanged %q", got)
	}
}

func TestDisplayTarget_NoTruncationUnderLimit(t *testing.T) {
	got := displayTarget("/short/path.json", 60)
	if got != "/short/path.json" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestDisplayTarget_TruncatesLongPath(t *testing.T) {
	long := "/opt/very/deeply/nested/directory/tree/with/many/segments/and/more/still/config.json"
	got := displayTarget(long, 30)
	if !strings.HasPrefix(got, "…") {
		t.Errorf("expected leading ellipsis, got %q", got)
	}
	if !strings.HasSuffix(got, "config.json") {
		t.Errorf("expected trailing filename, got %q", got)
	}
	// Must be shorter than the original.
	if len(got) >= len(long) {
		t.Errorf("truncation did not shorten path: %q (len %d) vs original len %d", got, len(got), len(long))
	}
}

func TestDisplayTarget_SoftLimitZeroDisablesTruncation(t *testing.T) {
	long := "/opt/very/deeply/nested/directory/tree/that/should/not/be/truncated.json"
	got := displayTarget(long, 0)
	if got != long {
		t.Errorf("got %q, want unchanged (softLimit=0 should disable truncation)", got)
	}
}

func TestDisplayTarget_EmptyStringUnchanged(t *testing.T) {
	if got := displayTarget("", 60); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
