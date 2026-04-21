package tools_test

import (
	"strings"
	"testing"

	"gaal/internal/config"
	"gaal/internal/tools"
)

func TestCollect_EmptyConfig_ReturnsNil(t *testing.T) {
	if got := tools.Collect(&config.Config{}); len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
	if got := tools.Collect(nil); got != nil {
		t.Errorf("want nil for nil config, got %+v", got)
	}
}

func TestCollect_TopLevelOnly(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.ConfigTool{
			{Name: "gh", Hint: "brew install gh"},
			{Name: "fnm"},
		},
	}
	got := tools.Collect(cfg)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Tool.Name != "gh" || got[0].Source != tools.SourceWorkspace {
		t.Errorf("entry[0] = %+v", got[0])
	}
	if got[1].Source != tools.SourceWorkspace {
		t.Errorf("entry[1] source = %q, want workspace", got[1].Source)
	}
}

func TestCollect_PerSkillOnly(t *testing.T) {
	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: "owner/repo", Tools: []config.ConfigTool{{Name: "tree-sitter"}}},
		},
	}
	got := tools.Collect(cfg)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].Tool.Name != "tree-sitter" {
		t.Errorf("entry name = %q", got[0].Tool.Name)
	}
	if got[0].Source != "skill: owner/repo" {
		t.Errorf("entry source = %q", got[0].Source)
	}
}

func TestCollect_TopLevelWinsAttributionOnNameClash(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.ConfigTool{
			{Name: "gh", Hint: "from-workspace"},
		},
		Skills: []config.ConfigSkill{
			{Source: "owner/repo", Tools: []config.ConfigTool{
				{Name: "gh", Hint: "from-skill"}, // should be shadowed
			}},
		},
	}
	got := tools.Collect(cfg)
	if len(got) != 1 {
		t.Fatalf("want 1 entry after dedup, got %d: %+v", len(got), got)
	}
	if got[0].Source != tools.SourceWorkspace {
		t.Errorf("source = %q, want workspace (top-level wins)", got[0].Source)
	}
	if got[0].Tool.Hint != "from-workspace" {
		t.Errorf("hint = %q, want from-workspace", got[0].Tool.Hint)
	}
}

func TestCollect_PreservesOrder_TopLevelThenSkills(t *testing.T) {
	cfg := &config.Config{
		Tools: []config.ConfigTool{{Name: "gh"}, {Name: "fnm"}},
		Skills: []config.ConfigSkill{
			{Source: "a/b", Tools: []config.ConfigTool{{Name: "rtk"}}},
			{Source: "c/d", Tools: []config.ConfigTool{{Name: "tree-sitter"}}},
		},
	}
	got := tools.Collect(cfg)
	wantNames := []string{"gh", "fnm", "rtk", "tree-sitter"}
	if len(got) != len(wantNames) {
		t.Fatalf("want %d entries, got %d", len(wantNames), len(got))
	}
	for i, n := range wantNames {
		if got[i].Tool.Name != n {
			t.Errorf("entry[%d] = %q, want %q", i, got[i].Tool.Name, n)
		}
	}
}

func TestCollect_SkipsEmptyName(t *testing.T) {
	// Validation rejects empty names at parse time, but Collect should still
	// be robust if given one directly.
	cfg := &config.Config{Tools: []config.ConfigTool{{Name: ""}, {Name: "gh"}}}
	got := tools.Collect(cfg)
	if len(got) != 1 || got[0].Tool.Name != "gh" {
		t.Errorf("got %+v, want only gh", got)
	}
}

func TestCheck_PresentBinary(t *testing.T) {
	// `go` is guaranteed to exist because the test suite runs under Go.
	entries := []tools.Entry{{Tool: config.ConfigTool{Name: "go"}, Source: tools.SourceWorkspace}}
	got := tools.Check(entries)
	if len(got) != 1 {
		t.Fatalf("want 1 status, got %d", len(got))
	}
	if !got[0].Found {
		t.Errorf("expected `go` to be found on PATH")
	}
	if got[0].Resolved == "" {
		t.Errorf("expected a non-empty resolved path")
	}
}

func TestCheck_MissingBinary(t *testing.T) {
	entries := []tools.Entry{
		{Tool: config.ConfigTool{Name: "gaal-definitely-not-a-real-tool-xyz"}, Source: tools.SourceWorkspace},
	}
	got := tools.Check(entries)
	if len(got) != 1 {
		t.Fatalf("want 1 status, got %d", len(got))
	}
	if got[0].Found {
		t.Errorf("expected missing binary, got found at %q", got[0].Resolved)
	}
	if got[0].Resolved != "" {
		t.Errorf("expected empty resolved path, got %q", got[0].Resolved)
	}
}

func TestCheck_PreservesOrder(t *testing.T) {
	entries := []tools.Entry{
		{Tool: config.ConfigTool{Name: "go"}},
		{Tool: config.ConfigTool{Name: "gaal-missing-xyz"}},
		{Tool: config.ConfigTool{Name: "go"}},
	}
	got := tools.Check(entries)
	if len(got) != 3 {
		t.Fatalf("want 3 statuses, got %d", len(got))
	}
	if !got[0].Found || got[1].Found || !got[2].Found {
		t.Errorf("found flags = [%v %v %v], want [true false true]", got[0].Found, got[1].Found, got[2].Found)
	}
	// Sanity: names round-trip through the statuses.
	wantNames := []string{"go", "gaal-missing-xyz", "go"}
	for i, n := range wantNames {
		if got[i].Entry.Tool.Name != n {
			t.Errorf("status[%d] name = %q, want %q", i, got[i].Entry.Tool.Name, n)
		}
	}
	// And the resolved path for `go` looks plausible.
	if !strings.HasSuffix(got[0].Resolved, "go") && !strings.HasSuffix(got[0].Resolved, "go.exe") {
		t.Errorf("resolved path %q does not look like a `go` binary", got[0].Resolved)
	}
}
