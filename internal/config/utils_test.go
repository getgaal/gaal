package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// indexOf
// ---------------------------------------------------------------------------

func TestIndexOf_Found(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := indexOf(items, func(s string) bool { return s == "b" })
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestIndexOf_NotFound(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := indexOf(items, func(s string) bool { return s == "z" })
	if got != -1 {
		t.Errorf("got %d, want -1", got)
	}
}

func TestIndexOf_ReturnsFirst(t *testing.T) {
	items := []string{"a", "b", "a"}
	got := indexOf(items, func(s string) bool { return s == "a" })
	if got != 0 {
		t.Errorf("got %d, want 0 (first match)", got)
	}
}

func TestIndexOf_EmptySlice(t *testing.T) {
	got := indexOf([]string{}, func(s string) bool { return s == "a" })
	if got != -1 {
		t.Errorf("got %d, want -1 for empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// deduplicate
// ---------------------------------------------------------------------------

func TestDeduplicate_RemovesDuplicates(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	got := deduplicate(input, func(s string) string { return s })
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDeduplicate_KeepsFirstOccurrence(t *testing.T) {
	type item struct {
		key string
		val int
	}
	input := []item{{"x", 1}, {"y", 2}, {"x", 99}}
	got := deduplicate(input, func(i item) string { return i.key })
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].val != 1 {
		t.Errorf("first occurrence should be kept: got val=%d, want 1", got[0].val)
	}
}

func TestDeduplicate_NoDuplicates(t *testing.T) {
	input := []string{"a", "b", "c"}
	got := deduplicate(input, func(s string) string { return s })
	if len(got) != 3 {
		t.Errorf("got %d items, want 3 (no duplicates)", len(got))
	}
}

func TestDeduplicate_EmptySlice(t *testing.T) {
	got := deduplicate([]string{}, func(s string) string { return s })
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice", got)
	}
}

func TestDeduplicate_AllDuplicates(t *testing.T) {
	input := []string{"a", "a", "a"}
	got := deduplicate(input, func(s string) string { return s })
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("got %v, want [a]", got)
	}
}

// ---------------------------------------------------------------------------
// expandPaths (via Load to exercise the full path)
// ---------------------------------------------------------------------------

func TestExpandPaths_TildeInSkillSource(t *testing.T) {
	home, _ := os.UserHomeDir()

	p := writeYAML(t, `
skills:
  - source: ~/my-skills
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, "my-skills")
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != want {
		t.Errorf("got %v, want %q", cfg.Skills, want)
	}
}

func TestExpandPaths_GitHubShorthandUnchanged(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != "owner/repo" {
		t.Errorf("GitHub shorthand should not be expanded, got %q", cfg.Skills[0].Source)
	}
}

func TestExpandPaths_HTTPSUnchanged(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: https://github.com/owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != "https://github.com/owner/repo" {
		t.Errorf("HTTPS URL should remain unchanged, got %q", cfg.Skills[0].Source)
	}
}

func TestExpandPaths_MCPTargetRelative(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gaal.yaml")
	os.WriteFile(p, []byte(`
mcps:
  - name: myserver
    target: configs/mcp.json
    inline:
      command: npx
`), 0o644)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(dir, "configs", "mcp.json")
	if len(cfg.MCPs) == 0 || cfg.MCPs[0].Target != want {
		t.Errorf("got %q, want %q", cfg.MCPs[0].Target, want)
	}
}

// ---------------------------------------------------------------------------
// isRemoteURL / isGitHubShorthand (unit tests on the helpers directly)
// ---------------------------------------------------------------------------

func TestIsRemoteURL(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://github.com/owner/repo", true},
		{"http://example.com/repo.git", true},
		{"git@github.com:owner/repo.git", true},
		{"ssh://user@host/repo.git", true},
		{"owner/repo", false},
		{"./local", false},
		{"~/local", false},
		{"/abs/path", false},
	}
	for _, tc := range cases {
		if got := isRemoteURL(tc.input); got != tc.want {
			t.Errorf("isRemoteURL(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsGitHubShorthand(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"owner/repo", true},
		{"my-org/my-tool", true},
		{"https://github.com/owner/repo", false},
		{"git@github.com:owner/repo.git", false},
		{"./local/path", false},
		{"../up/path", false},
		{"~/home/path", false},
		{"/abs/path", false},
		{"no-slash", false},
		{"too/many/parts", false},
	}
	for _, tc := range cases {
		if got := isGitHubShorthand(tc.input); got != tc.want {
			t.Errorf("isGitHubShorthand(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
