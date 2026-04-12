package ops

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"gaal/internal/config"
	"gaal/internal/engine/render"
)

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
	entries := []render.RepoEntry{
		{
			Path:    "/workspace/myrepo",
			Type:    "git",
			URL:     "https://github.com/foo/bar",
			Status:  render.StatusOK,
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
	entries := []render.RepoEntry{
		{Path: "/ws/repo", Type: "git", Status: render.StatusDirty, Dirty: true, Current: "HEAD"},
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
	entries := []render.RepoEntry{
		{Path: "/ws/broken", Type: "git", Status: render.StatusError, Error: "connection refused"},
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
	entries := []render.SkillEntry{
		{
			Source:    "github.com/foo/skills",
			Agent:     "claude-code",
			Status:    render.StatusOK,
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
	entries := []render.SkillEntry{
		{
			Source:   "local/skills",
			Agent:    "cursor",
			Status:   render.StatusPartial,
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
	entries := []render.MCPEntry{
		{Name: "my-server", Target: target, Status: render.StatusPresent},
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
	entries := []render.MCPEntry{
		{Name: "dirty-server", Target: target, Status: render.StatusDirty, Dirty: true},
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
	e := render.SkillEntry{}
	result := buildSkillTree(e)
	if len(result) != 0 {
		t.Errorf("expected empty tree for empty entry, got %d items", len(result))
	}
}

func TestBuildSkillTree_LastItemHasCornerPrefix(t *testing.T) {
	e := render.SkillEntry{Installed: []string{"a", "b", "c"}}
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
	e := render.SkillEntry{
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
	if err := renderAgentInfo(&buf, []render.AgentEntry{}, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Supported Agents") {
		t.Error("expected section header 'Supported Agents'")
	}
}

func TestRenderAgentInfo_EmptyWithFilter(t *testing.T) {
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, []render.AgentEntry{}, "no-match"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no agent matches") {
		t.Error("expected 'no agent matches' message")
	}
}

func TestRenderAgentInfo_ContainsKnownAgents(t *testing.T) {
	entries, err := collectAgents()
	if err != nil {
		t.Fatalf("collectAgents: %v", err)
	}
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	for _, want := range []string{"cursor", "claude-code", "github-copilot"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected agent %q in output", want)
		}
	}
}

func TestRenderAgentInfo_Filter(t *testing.T) {
	entries, err := collectAgents()
	if err != nil {
		t.Fatalf("collectAgents: %v", err)
	}
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, "cursor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	if !strings.Contains(out, "cursor") {
		t.Error("expected 'cursor' in filtered output")
	}
	if strings.Contains(out, "claude-code") {
		t.Error("expected 'claude-code' to be filtered out")
	}
}

func TestRenderAgentInfo_NoMCPShownAsDash(t *testing.T) {
	entries := []render.AgentEntry{
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

func TestRenderAgentInfo_InstalledFirst(t *testing.T) {
	entries := []render.AgentEntry{
		{Name: "alpha", Installed: false, Source: "builtin"},
		{Name: "beta", Installed: true, Source: "builtin"},
		{Name: "gamma", Installed: false, Source: "builtin"},
		{Name: "delta", Installed: true, Source: "builtin"},
	}
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	// beta and delta (installed) should appear before alpha and gamma.
	betaIdx := strings.Index(out, "beta")
	deltaIdx := strings.Index(out, "delta")
	alphaIdx := strings.Index(out, "alpha")
	gammaIdx := strings.Index(out, "gamma")
	if betaIdx < 0 || deltaIdx < 0 || alphaIdx < 0 || gammaIdx < 0 {
		t.Fatalf("missing agent names in output:\n%s", out)
	}
	if betaIdx > alphaIdx || deltaIdx > gammaIdx {
		t.Error("expected installed agents to appear before uninstalled agents")
	}
}

func TestRenderAgentInfo_ShowsInstalledStatus(t *testing.T) {
	entries := []render.AgentEntry{
		{Name: "myagent", Installed: true, Source: "builtin", ProjectSkillsDir: ".test/skills", GlobalSkillsDir: "~/.test/skills"},
	}
	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := pterm.RemoveColorFromString(buf.String())
	if !strings.Contains(out, "yes") {
		t.Error("expected 'yes' for installed agent")
	}
}

func TestRenderAgentInfo_GenericConventionShowsResolvedDir(t *testing.T) {
	entries := []render.AgentEntry{
		{
			Name:                    "cline",
			ProjectSkillsDir:        ".agents/skills",
			GlobalSkillsDir:         "/tmp/home/.agents/skills",
			ProjectSkillsViaGeneric: true,
			GlobalSkillsViaGeneric:  true,
		},
	}

	var buf bytes.Buffer
	if err := renderAgentInfo(&buf, entries, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := pterm.RemoveColorFromString(buf.String())
	for _, want := range []string{
		"via generic convention (.agents/skills)",
		"via generic convention (/tmp/home/.agents/skills)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}
