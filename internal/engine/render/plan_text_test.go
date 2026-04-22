package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlanText_EmptyPlan(t *testing.T) {
	var buf bytes.Buffer
	pr := &planTextRenderer{}
	if err := pr.Render(&buf, &PlanReport{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "nothing to do") {
		t.Errorf("expected 'nothing to do', got:\n%s", out)
	}
}

func TestPlanText_NestedStructure(t *testing.T) {
	var buf bytes.Buffer
	pr := &planTextRenderer{}
	report := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "src/example", Type: "git", Action: PlanClone, Want: "main"},
			{Path: "src/dataset", Type: "git", Action: PlanNoOp},
		},
		Skills: []PlanSkillEntry{
			{Source: "owner/code-review", Agent: "claude-code", Action: PlanCreate, Install: []string{"code-review"}},
			{Source: "owner/code-review", Agent: "cursor", Action: PlanCreate, Install: []string{"code-review"}},
		},
		MCPs: []PlanMCPEntry{
			{Name: "filesystem", Target: "/Users/test/.config/claude/claude_desktop_config.json", Action: PlanCreate},
		},
		HasChanges: true,
	}
	if err := pr.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"plan:",
		"  repositories",
		"  skills",
		"  mcps",
		"+ clone",
		"= unchanged",
		"src/example",
		"src/dataset",
		"claude_desktop_config.json",
		"changes pending",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q, full output:\n%s", want, out)
		}
	}
}

func TestPlanText_VerbAndNameAlignmentPerSection(t *testing.T) {
	var buf bytes.Buffer
	pr := &planTextRenderer{}
	report := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "short", Type: "git", Action: PlanClone, Want: "main"},
			{Path: "a-much-longer-path", Type: "git", Action: PlanUpdate, Current: "v1", Want: "v2"},
			{Path: "unchanged-path", Type: "git", Action: PlanNoOp},
		},
		HasChanges: true,
	}
	if err := pr.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// All three repo lines should line up the name column at the same visual
	// offset. Extract the three data rows under "  repositories".
	lines := strings.Split(buf.String(), "\n")
	var repoLines []string
	inRepo := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "repositories" {
			inRepo = true
			continue
		}
		if inRepo {
			if strings.HasPrefix(strings.TrimSpace(l), "+") ||
				strings.HasPrefix(strings.TrimSpace(l), "~") ||
				strings.HasPrefix(strings.TrimSpace(l), "=") {
				repoLines = append(repoLines, l)
			} else if !strings.HasPrefix(l, "    ") {
				break
			}
		}
	}
	if len(repoLines) != 3 {
		t.Fatalf("expected 3 repo lines, got %d:\n%s", len(repoLines), strings.Join(repoLines, "\n"))
	}
	// The name "short" must appear at the same column as "a-much-longer-path"
	// and "unchanged-path". Locate each.
	find := func(line, name string) int { return strings.Index(line, name) }
	p0 := find(repoLines[0], "short")
	p1 := find(repoLines[1], "a-much-longer-path")
	p2 := find(repoLines[2], "unchanged-path")
	if p0 == -1 || p1 == -1 || p2 == -1 {
		t.Fatalf("missing name in one of: %q / %q / %q", repoLines[0], repoLines[1], repoLines[2])
	}
	if !(p0 == p1 && p1 == p2) {
		t.Errorf("name column not aligned: %d / %d / %d\n%s", p0, p1, p2, strings.Join(repoLines, "\n"))
	}
}

func TestPlanText_SkillNameUsesBasenameForLocalSource(t *testing.T) {
	var buf bytes.Buffer
	pr := &planTextRenderer{}
	report := &PlanReport{
		Skills: []PlanSkillEntry{
			{
				Source:  "/deeply/nested/plugins/cache/my-skill",
				Agent:   "claude-code",
				Action:  PlanCreate,
				Install: []string{"my-skill"},
			},
		},
		HasChanges: true,
	}
	if err := pr.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	// The long source path should not appear; the basename should.
	if strings.Contains(out, "/deeply/nested/plugins/cache/my-skill") {
		t.Errorf("output leaks full source path:\n%s", out)
	}
	if !strings.Contains(out, "my-skill") {
		t.Errorf("output missing basename 'my-skill':\n%s", out)
	}
}
