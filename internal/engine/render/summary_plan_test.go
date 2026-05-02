package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestSummaryPlan_NothingToDo(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	if err := r.Render(&buf, &PlanReport{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.TrimSpace(out) != "nothing to do" {
		t.Errorf("expected 'nothing to do', got: %q", out)
	}
}

func TestSummaryPlan_AllUnchanged(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	report := &PlanReport{
		Skills: []PlanSkillEntry{
			{Source: "owner/code-review", Agent: "claude-code", Action: PlanNoOp, NoOp: []string{"code-review", "refactor"}},
		},
		MCPs: []PlanMCPEntry{
			{Name: "filesystem", Target: "/some/path", Action: PlanNoOp},
		},
		HasChanges: false,
		HasErrors:  false,
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	// Must have Plan: header
	if !strings.Contains(out, "Plan:") {
		t.Errorf("missing 'Plan:' header, got:\n%s", out)
	}
	// Must have unchanged summary line
	if !strings.Contains(out, "unchanged") {
		t.Errorf("missing 'unchanged' summary, got:\n%s", out)
	}
	// Footer must be "Everything up to date."
	if !strings.Contains(out, "Everything up to date.") {
		t.Errorf("missing 'Everything up to date.' footer, got:\n%s", out)
	}
	// Must NOT have "Run gaal sync" footer
	if strings.Contains(out, "Run gaal sync") {
		t.Errorf("should not have 'Run gaal sync' footer for all-unchanged, got:\n%s", out)
	}
}

func TestSummaryPlan_WithChanges(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	report := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "src/example", Type: "git", Action: PlanClone, Want: "main"},
		},
		Skills: []PlanSkillEntry{
			{Source: "owner/code-review", Agent: "claude-code", Action: PlanCreate, Install: []string{"code-review", "refactor"}},
			{Source: "owner/memory", Agent: "claude-code", Action: PlanNoOp, NoOp: []string{"memory"}},
			{Source: "owner/search", Agent: "claude-code", Action: PlanNoOp, NoOp: []string{"search", "browser", "fetch", "perplexity", "brave", "ddg", "exa", "tavily", "serp", "google"}},
		},
		MCPs: []PlanMCPEntry{
			{Name: "memory-mcp", Target: "/some/path", Action: PlanUpdate},
			{Name: "filesystem", Target: "/some/other/path", Action: PlanNoOp},
		},
		HasChanges: true,
		HasErrors:  false,
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	// Must have Plan: header
	if !strings.Contains(out, "Plan:") {
		t.Errorf("missing 'Plan:' header, got:\n%s", out)
	}
	// Must have clone row for repo
	if !strings.Contains(out, "clone") || !strings.Contains(out, "src/example") {
		t.Errorf("missing clone repo row, got:\n%s", out)
	}
	// Must have install row for skills
	if !strings.Contains(out, "install") {
		t.Errorf("missing install skills row, got:\n%s", out)
	}
	// Must show skill names
	if !strings.Contains(out, "code-review") {
		t.Errorf("missing skill name 'code-review', got:\n%s", out)
	}
	// Must have update row for MCP
	if !strings.Contains(out, "update") || !strings.Contains(out, "memory-mcp") {
		t.Errorf("missing update MCP row, got:\n%s", out)
	}
	// Must have unchanged summary line (collapsing no-ops)
	if !strings.Contains(out, "unchanged") {
		t.Errorf("missing unchanged summary, got:\n%s", out)
	}
	// Footer must be "Run gaal sync to apply."
	if !strings.Contains(out, "Run gaal sync to apply.") {
		t.Errorf("missing 'Run gaal sync to apply.' footer, got:\n%s", out)
	}
	// The unchanged line should use = marker
	lines := strings.Split(out, "\n")
	var unchangedLine string
	for _, l := range lines {
		if strings.Contains(l, "unchanged") {
			unchangedLine = l
			break
		}
	}
	if unchangedLine == "" {
		t.Fatalf("no unchanged line found in:\n%s", out)
	}
	if !strings.Contains(unchangedLine, "=") {
		t.Errorf("unchanged line missing '=' marker: %q", unchangedLine)
	}
}

func TestSummaryPlan_WithErrors(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	report := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "src/broken", Action: PlanError, Error: "network timeout"},
		},
		HasChanges: false,
		HasErrors:  true,
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	// Footer must be "Plan completed with errors."
	if !strings.Contains(out, "Plan completed with errors.") {
		t.Errorf("missing 'Plan completed with errors.' footer, got:\n%s", out)
	}
}

func TestSummaryPlan_VerbAlignment(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	report := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "src/example", Type: "git", Action: PlanClone},
		},
		MCPs: []PlanMCPEntry{
			{Name: "memory-mcp", Target: "/some/path", Action: PlanUpdate},
		},
		HasChanges: true,
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")
	var actionLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "~") || strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "!") {
			actionLines = append(actionLines, l)
		}
	}
	if len(actionLines) < 2 {
		t.Fatalf("expected at least 2 action lines, got %d:\n%s", len(actionLines), out)
	}
	// Each action line should have the verb at a consistent offset (after marker + space)
	// Find where the verb starts — it should be at the same column for all rows
	verbPositions := make([]int, len(actionLines))
	for i, l := range actionLines {
		// marker is at some indent; verb comes after "  X " (2 spaces, marker, space)
		// Find the first non-space char after the marker
		idx := strings.IndexAny(l, "+=~!")
		if idx < 0 {
			t.Fatalf("no marker found in line: %q", l)
		}
		// verb starts right after marker + space
		verbStart := idx + 2
		if verbStart >= len(l) {
			t.Fatalf("line too short after marker: %q", l)
		}
		verbPositions[i] = verbStart
	}
	for i := 1; i < len(verbPositions); i++ {
		if verbPositions[i] != verbPositions[0] {
			t.Errorf("verb positions differ: line 0 at %d, line %d at %d\nlines:\n%s",
				verbPositions[0], i, verbPositions[i], strings.Join(actionLines, "\n"))
		}
	}
}

func TestSummaryPlan_PluraliseHelper(t *testing.T) {
	cases := []struct {
		n    int
		one  string
		many string
		want string
	}{
		{1, "skill", "skills", "1 skill"},
		{2, "skill", "skills", "2 skills"},
		{0, "skill", "skills", "0 skills"},
	}
	for _, tc := range cases {
		got := pluralise(tc.n, tc.one, tc.many)
		if got != tc.want {
			t.Errorf("pluralise(%d, %q, %q) = %q, want %q", tc.n, tc.one, tc.many, got, tc.want)
		}
	}
}

func TestSummaryPlan_SkillCountInParens(t *testing.T) {
	var buf bytes.Buffer
	r := &summaryPlanRenderer{}
	report := &PlanReport{
		Skills: []PlanSkillEntry{
			{Source: "owner/pkg", Agent: "claude-code", Action: PlanCreate, Install: []string{"code-review", "refactor"}},
		},
		HasChanges: true,
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	// When multiple skills in one install row, should show count in parens
	if !strings.Contains(out, "(2 skills)") {
		t.Errorf("expected '(2 skills)' count in install row, got:\n%s", out)
	}
}
