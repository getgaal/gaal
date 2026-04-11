package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ── trunc ─────────────────────────────────────────────────────────────────────

func TestTrunc(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"", 5, ""},
		{"a", 1, "a"},   // fits exactly → no truncation
		{"ab", 2, "ab"}, // fits exactly → no truncation
		{"ab", 1, "…"},  // 2 > 1 → truncate
		{"abc", 3, "abc"},
		{"abcd", 3, "ab…"},
	}
	for _, tt := range tests {
		got := trunc(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

// ── varColWidth ───────────────────────────────────────────────────────────────

func TestVarColWidth_Basic(t *testing.T) {
	// termW=120, 4 cols → overhead=13, avail=120-13-22=85, varWidth=85/2=42
	got := varColWidth(120, 4, 2, 22)
	if got < 20 {
		t.Errorf("varColWidth(120,4,2,22) = %d, want >= 20", got)
	}
}

func TestVarColWidth_MinEnforced(t *testing.T) {
	// Very narrow terminal: min 12 per variable column should be enforced.
	got := varColWidth(20, 4, 2, 22)
	if got < 12 {
		t.Errorf("varColWidth(20,4,2,22) = %d, want >= 12", got)
	}
}

// ── statusCell ────────────────────────────────────────────────────────────────

func TestStatusCell(t *testing.T) {
	tests := []struct {
		code    StatusCode
		errMsg  string
		contain string
	}{
		{StatusOK, "", "synced"},
		{StatusPresent, "", "present"},
		{StatusNotCloned, "", "not cloned"},
		{StatusAbsent, "", "absent"},
		{StatusPartial, "", "partial"},
		{StatusError, "disk full", "disk full"},
		{StatusError, "", "error"},
	}
	for _, tt := range tests {
		cell := statusCell(tt.code, tt.errMsg)
		if !strings.Contains(cell, tt.contain) {
			t.Errorf("statusCell(%q, %q) = %q, want to contain %q", tt.code, tt.errMsg, cell, tt.contain)
		}
	}
}

// ── tableRenderer ─────────────────────────────────────────────────────────────

func TestTableRenderer_Empty(t *testing.T) {
	var buf bytes.Buffer
	r := &tableRenderer{}
	report := &StatusReport{
		Repositories: []RepoEntry{},
		Skills:       []SkillEntry{},
		MCPs:         []MCPEntry{},
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("tableRenderer.Render empty: %v", err)
	}
	out := buf.String()
	for _, section := range []string{"Repositories", "Skills", "MCP Configs"} {
		if !strings.Contains(out, section) {
			t.Errorf("table output missing section %q", section)
		}
	}
}

func TestTableRenderer_WithData(t *testing.T) {
	var buf bytes.Buffer
	r := &tableRenderer{}
	report := &StatusReport{
		Repositories: []RepoEntry{
			{Path: "/src/myrepo", Type: "git", Status: StatusOK, Current: "abc1234", Want: "main"},
			{Path: "/src/other", Type: "hg", Status: StatusNotCloned, URL: "https://example.com/x"},
		},
		Skills: []SkillEntry{
			{Source: "vercel-labs/skills", Agent: "claude-code", Status: StatusOK,
				Installed: []string{"tool-use"}, Missing: []string{}},
		},
		MCPs: []MCPEntry{
			{Name: "filesystem", Status: StatusPresent, Target: "/home/user/.config/claude.json"},
		},
		Agents: []AgentEntry{
			{Name: "cline", ProjectSkillsDir: ".agents/skills", GlobalSkillsDir: "/home/user/.agents/skills", ProjectSkillsViaGeneric: true, GlobalSkillsViaGeneric: true},
		},
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("tableRenderer.Render with data: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"myrepo", "git", "other", "filesystem", "cline", ".agents/skills"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q", want)
		}
	}
}

// ── aggregateSkillsByName ─────────────────────────────────────────────────────

func TestAggregateSkillsByName_InstalledEverywhereYieldsStar(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"code-reviewer"}},
		{Source: "owner/repo", Agent: "codex", Status: StatusOK, Installed: []string{"code-reviewer"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d: %+v", len(got), got)
	}
	s := got[0]
	if s.Name != "code-reviewer" {
		t.Errorf("name: got %q, want %q", s.Name, "code-reviewer")
	}
	if !s.AllAgents {
		t.Errorf("AllAgents: got false, want true (installed in every targeted agent)")
	}
	if s.Status != StatusOK {
		t.Errorf("status: got %q, want %q", s.Status, StatusOK)
	}
	if len(s.Sources) != 1 || s.Sources[0] != "owner/repo" {
		t.Errorf("sources: got %v", s.Sources)
	}
}

func TestAggregateSkillsByName_PartialWhenSomeAgentsMissing(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"find-skills"}},
		{Source: "owner/repo", Agent: "codex", Status: StatusPartial, Missing: []string{"find-skills"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d", len(got))
	}
	s := got[0]
	if s.AllAgents {
		t.Error("AllAgents: got true, want false")
	}
	if s.Status != StatusPartial {
		t.Errorf("status: got %q, want %q", s.Status, StatusPartial)
	}
	if len(s.Agents) != 1 || s.Agents[0] != "cursor" {
		t.Errorf("agents: got %v, want [cursor]", s.Agents)
	}
}

func TestAggregateSkillsByName_DirtyWhenAnyModified(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"add-to-inbox"}},
		{Source: "owner/repo", Agent: "codex", Status: StatusDirty, Installed: []string{"add-to-inbox"}, Modified: []string{"add-to-inbox"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d", len(got))
	}
	s := got[0]
	if s.Status != StatusDirty {
		t.Errorf("status: got %q, want %q", s.Status, StatusDirty)
	}
	// Modified agents are still "installed" — show * when present everywhere.
	if !s.AllAgents {
		t.Errorf("AllAgents: dirty-but-installed-everywhere should show *")
	}
}

func TestAggregateSkillsByName_MissingEverywhere(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusPartial, Missing: []string{"react-doctor"}},
		{Source: "owner/repo", Agent: "codex", Status: StatusPartial, Missing: []string{"react-doctor"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d", len(got))
	}
	s := got[0]
	if s.Status != StatusPartial {
		t.Errorf("status: got %q, want %q", s.Status, StatusPartial)
	}
	if len(s.Agents) != 0 {
		t.Errorf("agents: got %v, want empty (installed nowhere)", s.Agents)
	}
	if s.AllAgents {
		t.Error("AllAgents should be false when installed nowhere")
	}
}

func TestAggregateSkillsByName_MultipleSourcesMerged(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo-a", Agent: "cursor", Status: StatusOK, Installed: []string{"shared-skill"}},
		{Source: "owner/repo-b", Agent: "codex", Status: StatusOK, Installed: []string{"shared-skill"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d", len(got))
	}
	s := got[0]
	if len(s.Sources) != 2 {
		t.Errorf("sources: got %v, want 2 entries", s.Sources)
	}
	// Sources should be sorted for determinism.
	if s.Sources[0] != "owner/repo-a" || s.Sources[1] != "owner/repo-b" {
		t.Errorf("sources not sorted: got %v", s.Sources)
	}
	if !s.AllAgents {
		t.Error("AllAgents: skill is installed in every agent that targets it")
	}
}

func TestAggregateSkillsByName_ErrorPropagates(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"broken"}},
		{Source: "owner/repo", Agent: "codex", Status: StatusError, Error: "permission denied", Missing: []string{"broken"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 aggregated skill, got %d", len(got))
	}
	s := got[0]
	if s.Status != StatusError {
		t.Errorf("status: got %q, want %q", s.Status, StatusError)
	}
	if !strings.Contains(s.Error, "permission denied") {
		t.Errorf("error: got %q, want to contain 'permission denied'", s.Error)
	}
}

func TestAggregateSkillsByName_SortedByName(t *testing.T) {
	in := []SkillEntry{
		{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"zebra", "apple", "mango"}},
	}
	got := aggregateSkillsByName(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 aggregated skills, got %d", len(got))
	}
	want := []string{"apple", "mango", "zebra"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestTableRenderer_SkillsByName_RendersStar(t *testing.T) {
	var buf bytes.Buffer
	r := &tableRenderer{}
	report := &StatusReport{
		Skills: []SkillEntry{
			{Source: "owner/repo", Agent: "cursor", Status: StatusOK, Installed: []string{"code-reviewer"}},
			{Source: "owner/repo", Agent: "codex", Status: StatusOK, Installed: []string{"code-reviewer"}},
		},
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "code-reviewer") {
		t.Error("expected skill name in output")
	}
	if !strings.Contains(out, "SKILL") || !strings.Contains(out, "AGENTS") {
		t.Errorf("expected new column headers SKILL and AGENTS, got:\n%s", out)
	}
	// Exactly one data row (not one per agent).
	if strings.Count(out, "code-reviewer") != 1 {
		t.Errorf("expected code-reviewer to appear exactly once, got %d occurrences", strings.Count(out, "code-reviewer"))
	}
}

// ── jsonRenderer ─────────────────────────────────────────────────────────────

func TestJSONRenderer_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	r := &jsonRenderer{}
	report := &StatusReport{
		Repositories: []RepoEntry{
			{Path: "/src/repo", Type: "git", Status: StatusOK, Current: "abc", Want: "main"},
		},
		Skills: []SkillEntry{},
		MCPs:   []MCPEntry{},
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("jsonRenderer.Render: %v", err)
	}

	var decoded StatusReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(decoded.Repositories) != 1 || decoded.Repositories[0].Path != "/src/repo" {
		t.Errorf("unexpected decoded repos: %+v", decoded.Repositories)
	}
}

func TestJSONRenderer_EmptySlicesNotNull(t *testing.T) {
	var buf bytes.Buffer
	r := &jsonRenderer{}
	report := &StatusReport{
		Repositories: []RepoEntry{},
		Skills:       []SkillEntry{},
		MCPs:         []MCPEntry{},
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("jsonRenderer.Render empty: %v", err)
	}
	// Arrays must be [] not null in JSON output.
	out := buf.String()
	for _, want := range []string{`"repositories": []`, `"skills": []`, `"mcps": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("json output: expected %q\ngot:\n%s", want, out)
		}
	}
}

// ── NewRenderer ───────────────────────────────────────────────────────────────

func TestNewRenderer_Table(t *testing.T) {
	r, err := NewRenderer(FormatTable)
	if err != nil || r == nil {
		t.Fatalf("NewRenderer(table): err=%v, r=%v", err, r)
	}
}

func TestNewRenderer_JSON(t *testing.T) {
	r, err := NewRenderer(FormatJSON)
	if err != nil || r == nil {
		t.Fatalf("NewRenderer(json): err=%v, r=%v", err, r)
	}
}

func TestNewRenderer_Unknown(t *testing.T) {
	_, err := NewRenderer("csv")
	if err == nil {
		t.Fatal("NewRenderer(csv): expected error, got nil")
	}
}
