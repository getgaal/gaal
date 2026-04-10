package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"gaal/internal/config"
)

// captureStdout redirects os.Stdout to an os.Pipe for the duration of fn,
// then restores it and returns everything that was written.
// The pipe is drained concurrently to prevent blocking when output is large.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf.ReadFrom(r) //nolint:errcheck
	}()

	fn()
	w.Close()
	os.Stdout = orig
	<-done
	r.Close()
	return buf.String()
}

// ── Info (engine method) ──────────────────────────────────────────────────────

func TestInfo_UnknownPackage(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		err := e.Info(context.Background(), "unknown", "", FormatTable)
		if err == nil {
			t.Error("expected error for unknown package type")
		}
	})
	_ = out // output may be partial; we just care about the error
}

func TestInfo_UnknownPackage_Error(t *testing.T) {
	e := New(&config.Config{})
	err := e.Info(context.Background(), "foo", "", FormatTable)
	if err == nil || !strings.Contains(err.Error(), "unknown package type") {
		t.Errorf("expected 'unknown package type' error, got: %v", err)
	}
}

func TestInfo_JSON_Agent(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "", FormatJSON); err != nil {
			t.Errorf("Info agent json: %v", err)
		}
	})
	var result struct {
		Agents []AgentEntry `json:"agents"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, out)
	}
	if len(result.Agents) == 0 {
		t.Error("expected at least one agent in JSON output")
	}
}

func TestInfo_JSON_Agent_Filter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "cursor", FormatJSON); err != nil {
			t.Errorf("Info agent json filter: %v", err)
		}
	})
	var result struct {
		Agents []AgentEntry `json:"agents"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, a := range result.Agents {
		if !strings.Contains(a.Name, "cursor") {
			t.Errorf("expected only cursor agents, got %q", a.Name)
		}
	}
}

func TestInfo_JSON_Repo_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "repo", "", FormatJSON); err != nil {
			t.Errorf("Info repo json: %v", err)
		}
	})
	var result struct {
		Repositories []RepoEntry `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
}

func TestInfo_JSON_UnknownPackage(t *testing.T) {
	e := New(&config.Config{})
	err := e.Info(context.Background(), "bad", "", FormatJSON)
	if err == nil || !strings.Contains(err.Error(), "unknown package type") {
		t.Errorf("expected 'unknown package type' error in JSON mode, got: %v", err)
	}
}

func TestInfo_Repo_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "repo", "", FormatTable); err != nil {
			t.Errorf("Info repo empty: %v", err)
		}
	})
	if !strings.Contains(out, "Repositories") {
		t.Errorf("output missing 'Repositories' section, got:\n%s", out)
	}
}

func TestInfo_Skill_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "skill", "", FormatTable); err != nil {
			t.Errorf("Info skill empty: %v", err)
		}
	})
	if !strings.Contains(out, "Skills") {
		t.Errorf("output missing 'Skills' section, got:\n%s", out)
	}
}

func TestInfo_MCP_Empty(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "mcp", "", FormatTable); err != nil {
			t.Errorf("Info mcp empty: %v", err)
		}
	})
	if !strings.Contains(out, "MCP") {
		t.Errorf("output missing 'MCP' section, got:\n%s", out)
	}
}

// ── renderRepoInfo ────────────────────────────────────────────────────────────

func TestRenderRepoInfo_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := renderRepoInfo(&buf, nil, nil, "")
	if err != nil {
		t.Fatalf("renderRepoInfo empty: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Repositories") {
		t.Errorf("expected 'Repositories' header, got:\n%s", out)
	}
	if !strings.Contains(out, "no repositories configured") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestRenderRepoInfo_WithEntry(t *testing.T) {
	cfgRepos := map[string]config.RepoConfig{
		"/workspace/myrepo": {Type: "git", URL: "https://github.com/foo/bar", Version: "main"},
	}
	entries := []RepoEntry{
		{
			Path:    "/workspace/myrepo",
			Type:    "git",
			URL:     "https://github.com/foo/bar",
			Status:  StatusOK,
			Current: "abc1234",
			Want:    "main",
		},
	}
	var buf bytes.Buffer
	if err := renderRepoInfo(&buf, cfgRepos, entries, ""); err != nil {
		t.Fatalf("renderRepoInfo: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"/workspace/myrepo", "git", "https://github.com/foo/bar", "abc1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRepoInfo_DirtyEntry(t *testing.T) {
	cfgRepos := map[string]config.RepoConfig{
		"/ws/repo": {Type: "git", URL: "https://example.com/repo"},
	}
	entries := []RepoEntry{
		{Path: "/ws/repo", Type: "git", Status: StatusDirty, Dirty: true, Current: "HEAD"},
	}
	var buf bytes.Buffer
	if err := renderRepoInfo(&buf, cfgRepos, entries, ""); err != nil {
		t.Fatalf("renderRepoInfo dirty: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "uncommitted") {
		t.Errorf("expected dirty message, got:\n%s", out)
	}
}

func TestRenderRepoInfo_ErrorEntry(t *testing.T) {
	cfgRepos := map[string]config.RepoConfig{
		"/ws/broken": {Type: "git", URL: "https://example.com/broken"},
	}
	entries := []RepoEntry{
		{Path: "/ws/broken", Type: "git", Status: StatusError, Error: "connection refused"},
	}
	var buf bytes.Buffer
	if err := renderRepoInfo(&buf, cfgRepos, entries, ""); err != nil {
		t.Fatalf("renderRepoInfo error entry: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "connection refused") {
		t.Errorf("expected error message, got:\n%s", out)
	}
}

// ── renderSkillInfo ───────────────────────────────────────────────────────────

func TestRenderSkillInfo_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, nil, nil, ""); err != nil {
		t.Fatalf("renderSkillInfo empty: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Skills") {
		t.Errorf("expected 'Skills' header, got:\n%s", out)
	}
	if !strings.Contains(out, "no skills configured") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestRenderSkillInfo_WithEntry(t *testing.T) {
	cfgSkills := []config.SkillConfig{
		{Source: "github.com/foo/skills", Agents: []string{"claude-code"}, Global: false},
	}
	entries := []SkillEntry{
		{
			Source:    "github.com/foo/skills",
			Agent:     "claude-code",
			Status:    StatusOK,
			Installed: []string{"ts-patterns", "react-best"},
		},
	}
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, cfgSkills, entries, ""); err != nil {
		t.Fatalf("renderSkillInfo: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"github.com/foo/skills", "claude-code", "ts-patterns", "react-best"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderSkillInfo_MissingAndModified(t *testing.T) {
	cfgSkills := []config.SkillConfig{
		{Source: "local/skills", Agents: []string{"cursor"}},
	}
	entries := []SkillEntry{
		{
			Source:   "local/skills",
			Agent:    "cursor",
			Status:   StatusPartial,
			Missing:  []string{"missing-skill"},
			Modified: []string{"mod-skill"},
		},
	}
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, cfgSkills, entries, ""); err != nil {
		t.Fatalf("renderSkillInfo partial: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"missing-skill", "mod-skill"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderSkillInfo_GlobalScope(t *testing.T) {
	cfgSkills := []config.SkillConfig{
		{Source: "remote/skills", Global: true},
	}
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, cfgSkills, nil, ""); err != nil {
		t.Fatalf("renderSkillInfo global: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "global") {
		t.Errorf("expected 'global' scope, got:\n%s", out)
	}
}

// ── renderMCPInfo ─────────────────────────────────────────────────────────────

func TestRenderMCPInfo_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, nil, nil, ""); err != nil {
		t.Fatalf("renderMCPInfo empty: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "MCP") {
		t.Errorf("expected 'MCP' header, got:\n%s", out)
	}
	if !strings.Contains(out, "no MCP configs configured") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestRenderMCPInfo_WithInline(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	cfgMCPs := []config.MCPConfig{
		{
			Name:   "my-server",
			Target: target,
			Merge:  true,
			Inline: &config.MCPInlineConfig{
				Command: "node",
				Args:    []string{"server.js", "--port=3000"},
				Env:     map[string]string{"API_KEY": "secret"},
			},
		},
	}
	entries := []MCPEntry{
		{Name: "my-server", Target: target, Status: StatusPresent},
	}
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, cfgMCPs, entries, ""); err != nil {
		t.Fatalf("renderMCPInfo inline: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"my-server", "node", "server.js", "API_KEY"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMCPInfo_DirtyEntry(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	cfgMCPs := []config.MCPConfig{
		{Name: "dirty-server", Target: target},
	}
	entries := []MCPEntry{
		{Name: "dirty-server", Target: target, Status: StatusDirty, Dirty: true},
	}
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, cfgMCPs, entries, ""); err != nil {
		t.Fatalf("renderMCPInfo dirty: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "local changes") {
		t.Errorf("expected dirty message, got:\n%s", out)
	}
}

func TestRenderMCPInfo_WithSource(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	cfgMCPs := []config.MCPConfig{
		{Name: "remote-server", Target: target, Source: "https://example.com/mcp.json"},
	}
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, cfgMCPs, nil, ""); err != nil {
		t.Fatalf("renderMCPInfo source: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "https://example.com/mcp.json") {
		t.Errorf("expected source URL, got:\n%s", out)
	}
}

// ── buildSkillTree ────────────────────────────────────────────────────────────

func TestBuildSkillTree_Empty(t *testing.T) {
	e := SkillEntry{}
	result := buildSkillTree(e)
	if len(result) != 0 {
		t.Errorf("expected empty tree for empty entry, got %d items", len(result))
	}
}

func TestBuildSkillTree_LastItemHasCornerPrefix(t *testing.T) {
	e := SkillEntry{Installed: []string{"a", "b", "c"}}
	result := buildSkillTree(e)
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	// The last line should contain the corner character └.
	last := result[len(result)-1]
	if !strings.Contains(last, "└") {
		t.Errorf("last tree item should have '└' corner, got: %q", last)
	}
	// Other lines should have the branch character ├.
	for _, line := range result[:len(result)-1] {
		if !strings.Contains(line, "├") {
			t.Errorf("non-last item should have '├', got: %q", line)
		}
	}
}

func TestBuildSkillTree_MixedItems(t *testing.T) {
	e := SkillEntry{
		Installed: []string{"ok-skill"},
		Missing:   []string{"gone-skill"},
		Modified:  []string{"changed-skill"},
	}
	result := buildSkillTree(e)
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
}

// ── kvLine ────────────────────────────────────────────────────────────────────

func TestKvLine_ContainsKeyAndValue(t *testing.T) {
	line := kvLine("Type", "git")
	if !strings.Contains(line, "Type") {
		t.Errorf("kvLine missing key: %q", line)
	}
	if !strings.Contains(line, "git") {
		t.Errorf("kvLine missing value: %q", line)
	}
}

// ── visibleLen ────────────────────────────────────────────────────────────────

func TestVisibleLen_PlainString(t *testing.T) {
	if got := visibleLen("hello"); got != 5 {
		t.Errorf("visibleLen plain: got %d, want 5", got)
	}
}

func TestVisibleLen_EmptyString(t *testing.T) {
	if got := visibleLen(""); got != 0 {
		t.Errorf("visibleLen empty: got %d, want 0", got)
	}
}

func TestVisibleLen_ANSIStripped(t *testing.T) {
	// ANSI colour codes must not be counted.
	coloured := pterm.FgGreen.Sprint("hello") // e.g. "\x1b[32mhello\x1b[0m"
	if got := visibleLen(coloured); got != 5 {
		t.Errorf("visibleLen ANSI: got %d, want 5 (string: %q)", got, coloured)
	}
}

// ── padRight ─────────────────────────────────────────────────────────────────

func TestPadRight_ShortString_IsPadded(t *testing.T) {
	got := padRight("hi", 5)
	if visibleLen(got) != 5 {
		t.Errorf("padRight: visible length = %d, want 5 (got %q)", visibleLen(got), got)
	}
	if !strings.HasPrefix(got, "hi") {
		t.Errorf("padRight: original prefix lost (got %q)", got)
	}
}

func TestPadRight_ExactWidth_Unchanged(t *testing.T) {
	got := padRight("hi", 2)
	if got != "hi" {
		t.Errorf("padRight exact: got %q, want %q", got, "hi")
	}
}

func TestPadRight_LongerString_Unchanged(t *testing.T) {
	got := padRight("toolong", 3)
	if got != "toolong" {
		t.Errorf("padRight longer: got %q, want unchanged %q", got, "toolong")
	}
}

func TestPadRight_ANSIString_PaddedCorrectly(t *testing.T) {
	coloured := pterm.FgCyan.Sprint("abc") // visible width = 3
	got := padRight(coloured, 10)
	if visibleLen(got) != 10 {
		t.Errorf("padRight ANSI: visible len = %d, want 10", visibleLen(got))
	}
}

// ── renderAgentInfo ───────────────────────────────────────────────────────────

func TestRenderAgentInfo_Empty(t *testing.T) {
	var buf bytes.Buffer
	// Empty entries with no filter → "no agents registered"
	if err := renderAgentInfo(&buf, []AgentEntry{}, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Supported Agents") {
		t.Error("expected section header 'Supported Agents'")
	}
}

func TestRenderAgentInfo_EmptyWithFilter(t *testing.T) {
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, []AgentEntry{}, "no-match"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no agent matches") {
		t.Error("expected 'no agent matches' message")
	}
}

func TestRenderAgentInfo_ContainsKnownAgents(t *testing.T) {
	// Use a realistic list from agent.List() via collectAgents().
	entries := collectAgents()
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	// Several well-known agents must appear.
	for _, want := range []string{"cursor", "claude-code", "github-copilot"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected agent %q in output", want)
		}
	}
}

func TestRenderAgentInfo_Filter(t *testing.T) {
	entries := collectAgents()
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, "cursor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	if !strings.Contains(out, "cursor") {
		t.Error("expected 'cursor' in filtered output")
	}
	// A completely different agent must NOT appear.
	if strings.Contains(out, "claude-code") {
		t.Error("expected 'claude-code' to be filtered out")
	}
}

func TestRenderAgentInfo_NoMCPShownAsDash(t *testing.T) {
	entries := []AgentEntry{
		{Name: "testagent", ProjectSkillsDir: ".agents/skills", GlobalSkillsDir: "~/.agents/skills", ProjectMCPConfigFile: ""},
	}
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	if !strings.Contains(out, "not supported") {
		t.Error("expected 'not supported' for empty ProjectMCPConfigFile")
	}
}

func TestInfo_Agent_NoFilter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "", FormatTable); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	clean := pterm.RemoveColorFromString(out)
	if !strings.Contains(clean, "Supported Agents") {
		t.Error("expected 'Supported Agents' section header")
	}
}

func TestInfo_Agent_Filter(t *testing.T) {
	e := New(&config.Config{})
	out := captureStdout(t, func() {
		if err := e.Info(context.Background(), "agent", "goose", FormatTable); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	clean := pterm.RemoveColorFromString(out)
	if !strings.Contains(clean, "goose") {
		t.Error("expected 'goose' in filtered agent output")
	}
}

// ── infoBoxes (uniform width) ─────────────────────────────────────────────────

// boxInnerWidth returns the number of characters between the two ASCII `|`
// border characters on a pterm box content row (after stripping ANSI codes).
// Returns -1 if the line is not a content row.
func boxInnerWidth(line string) int {
	clean := pterm.RemoveColorFromString(line)
	runes := []rune(clean)
	if len(runes) < 2 || runes[0] != '|' {
		return -1
	}
	for i := len(runes) - 1; i > 0; i-- {
		if runes[i] == '|' {
			return i - 1 // runes between position 1 (exclusive of leading '|') and i (exclusive)
		}
	}
	return -1
}

func TestInfoBoxes_UniformWidth(t *testing.T) {
	// Card 2's title (" much longer title and content here ") forces the box
	// inner to be titleVisLen+4 = 36+4 = 40; cards 1 and 3 must also reach 40.
	cards := []infoCard{
		{title: " short ", lines: []string{"a"}},
		{title: " much longer title and content here ", lines: []string{"a much longer content line that wins"}},
		{title: " mid ", lines: []string{"medium length line here"}},
	}

	var buf bytes.Buffer
	infoBoxes(&buf, cards)
	out := buf.String()

	// Collect inner widths of all content rows (lines whose clean form starts with '|').
	var widths []int
	for _, line := range strings.Split(out, "\n") {
		w := boxInnerWidth(line)
		if w > 0 {
			widths = append(widths, w)
		}
	}

	if len(widths) == 0 {
		t.Fatal("no content rows found in infoBoxes output")
	}
	// All content rows across all cards must have identical inner widths.
	for i, w := range widths {
		if w != widths[0] {
			t.Errorf("row %d has inner width %d, want %d (all rows must be uniform)\nOutput:\n%s",
				i, w, widths[0], out)
		}
	}
}

func TestInfoBoxes_SingleCard_NoChange(t *testing.T) {
	cards := []infoCard{
		{title: " only ", lines: []string{"line one", "line two"}},
	}
	var buf bytes.Buffer
	infoBoxes(&buf, cards)
	out := buf.String()
	for _, want := range []string{"line one", "line two"} {
		if !strings.Contains(out, want) {
			t.Errorf("single-card output missing %q:\n%s", want, out)
		}
	}
}

func TestInfoBoxes_EmptyCards_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	infoBoxes(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty cards, got %q", buf.String())
	}
}

func TestInfoBoxes_CardWithNoLines_RendersBox(t *testing.T) {
	cards := []infoCard{
		{title: " empty card ", lines: nil},
		{title: " normal ", lines: []string{"some content"}},
	}
	var buf bytes.Buffer
	infoBoxes(&buf, cards)
	out := buf.String()
	if !strings.Contains(out, "some content") {
		t.Errorf("expected 'some content' in output:\n%s", out)
	}
}

// ── filter (matchFilter + render*Info with filter) ────────────────────────────

func TestMatchFilter_EmptyFilter_AlwaysMatches(t *testing.T) {
	if !matchFilter("anything", "") {
		t.Error("empty filter should match everything")
	}
}

func TestMatchFilter_CaseInsensitive(t *testing.T) {
	if !matchFilter("vercel-labs/agent-skills", "Vercel-Labs") {
		t.Error("matchFilter should be case-insensitive")
	}
}

func TestMatchFilter_Substring(t *testing.T) {
	if !matchFilter("vercel-labs/agent-skills", "agent") {
		t.Error("matchFilter should match substrings")
	}
	if matchFilter("vercel-labs/agent-skills", "cursor") {
		t.Error("matchFilter should not match unrelated string")
	}
}

func TestRenderSkillInfo_Filter_Match(t *testing.T) {
	cfgSkills := []config.SkillConfig{
		{Source: "vercel-labs/agent-skills", Agents: []string{"claude-code"}},
		{Source: "other-org/other-skills", Agents: []string{"cursor"}},
	}
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, cfgSkills, nil, "vercel-labs"); err != nil {
		t.Fatalf("renderSkillInfo with filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "vercel-labs/agent-skills") {
		t.Errorf("expected matched skill in output:\n%s", out)
	}
	if strings.Contains(out, "other-org/other-skills") {
		t.Errorf("expected filtered-out skill to be absent:\n%s", out)
	}
}

func TestRenderSkillInfo_Filter_NoMatch(t *testing.T) {
	cfgSkills := []config.SkillConfig{
		{Source: "vercel-labs/agent-skills", Agents: []string{"claude-code"}},
	}
	var buf bytes.Buffer
	if err := renderSkillInfo(&buf, cfgSkills, nil, "nonexistent"); err != nil {
		t.Fatalf("renderSkillInfo no-match filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no skill matches") {
		t.Errorf("expected no-match message, got:\n%s", out)
	}
}

func TestRenderRepoInfo_Filter_Match(t *testing.T) {
	cfgRepos := map[string]config.RepoConfig{
		"/ws/frontend": {Type: "git", URL: "https://example.com/frontend"},
		"/ws/backend":  {Type: "git", URL: "https://example.com/backend"},
	}
	entries := []RepoEntry{
		{Path: "/ws/frontend", Type: "git", Status: StatusOK, URL: "https://example.com/frontend"},
		{Path: "/ws/backend", Type: "git", Status: StatusOK, URL: "https://example.com/backend"},
	}
	var buf bytes.Buffer
	if err := renderRepoInfo(&buf, cfgRepos, entries, "frontend"); err != nil {
		t.Fatalf("renderRepoInfo with filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "frontend") {
		t.Errorf("expected matched repo in output:\n%s", out)
	}
	if strings.Contains(out, "backend") {
		t.Errorf("expected filtered-out repo to be absent:\n%s", out)
	}
}

func TestRenderRepoInfo_Filter_NoMatch(t *testing.T) {
	cfgRepos := map[string]config.RepoConfig{
		"/ws/repo": {Type: "git", URL: "https://example.com/repo"},
	}
	entries := []RepoEntry{
		{Path: "/ws/repo", Type: "git", Status: StatusOK},
	}
	var buf bytes.Buffer
	if err := renderRepoInfo(&buf, cfgRepos, entries, "xyz-no-match"); err != nil {
		t.Fatalf("renderRepoInfo no-match filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no repository matches") {
		t.Errorf("expected no-match message, got:\n%s", out)
	}
}

func TestRenderMCPInfo_Filter_Match(t *testing.T) {
	target := t.TempDir() + "/mcp.json"
	cfgMCPs := []config.MCPConfig{
		{Name: "claude-mcp", Target: target},
		{Name: "cursor-mcp", Target: target},
	}
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, cfgMCPs, nil, "claude"); err != nil {
		t.Fatalf("renderMCPInfo with filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "claude-mcp") {
		t.Errorf("expected matched MCP in output:\n%s", out)
	}
	if strings.Contains(out, "cursor-mcp") {
		t.Errorf("expected filtered-out MCP to be absent:\n%s", out)
	}
}

func TestRenderMCPInfo_Filter_NoMatch(t *testing.T) {
	target := t.TempDir() + "/mcp.json"
	cfgMCPs := []config.MCPConfig{
		{Name: "my-server", Target: target},
	}
	var buf bytes.Buffer
	if err := renderMCPInfo(&buf, cfgMCPs, nil, "nonexistent"); err != nil {
		t.Fatalf("renderMCPInfo no-match filter: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no MCP matches") {
		t.Errorf("expected no-match message, got:\n%s", out)
	}
}
