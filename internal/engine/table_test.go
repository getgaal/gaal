package engine

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
	}
	if err := r.Render(&buf, report); err != nil {
		t.Fatalf("tableRenderer.Render with data: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"myrepo", "git", "other", "filesystem"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q", want)
		}
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
