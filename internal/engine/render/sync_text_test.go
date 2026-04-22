package render

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderSyncSummary_DocsFormat(t *testing.T) {
	// Matches the canonical docs example:
	//   ✓ src/example          cloned
	//   ✓ code-review          installed in claude-code, cursor
	//   ✓ filesystem           upserted in claude_desktop_config.json
	//   sync complete in 1.2s
	plan := &PlanReport{
		Repositories: []PlanRepoEntry{
			{Path: "src/example", Action: PlanClone},
		},
		Skills: []PlanSkillEntry{
			{Source: "owner/code-review", Agent: "claude-code", Action: PlanCreate, Install: []string{"code-review"}},
			{Source: "owner/code-review", Agent: "cursor", Action: PlanCreate, Install: []string{"code-review"}},
		},
		MCPs: []PlanMCPEntry{
			{Name: "filesystem", Target: "/home/user/.config/claude/claude_desktop_config.json", Action: PlanCreate},
		},
	}
	status := &StatusReport{
		Repositories: []RepoEntry{
			{Path: "src/example", Status: StatusOK},
		},
		Skills: []SkillEntry{
			{Source: "owner/code-review", Agent: "claude-code", Status: StatusOK, Installed: []string{"code-review"}},
			{Source: "owner/code-review", Agent: "cursor", Status: StatusOK, Installed: []string{"code-review"}},
		},
		MCPs: []MCPEntry{
			{Name: "filesystem", Status: StatusPresent, Target: "/home/user/.config/claude/claude_desktop_config.json"},
		},
	}
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, plan, status, 1200*time.Millisecond); err != nil {
		t.Fatalf("RenderSyncSummary: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"✓ src/example",
		"cloned",
		"✓ code-review",
		"installed in claude-code, cursor",
		"✓ filesystem",
		"upserted in claude_desktop_config.json",
		"sync complete in 1.2s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRenderSyncSummary_NoOpShowsUpToDate(t *testing.T) {
	plan := &PlanReport{
		Repositories: []PlanRepoEntry{{Path: "src/example", Action: PlanNoOp}},
		Skills: []PlanSkillEntry{
			{Source: "owner/repo", Agent: "claude-code", Action: PlanNoOp},
		},
		MCPs: []PlanMCPEntry{{Name: "filesystem", Target: "/x/claude_desktop_config.json", Action: PlanNoOp}},
	}
	status := &StatusReport{
		Repositories: []RepoEntry{{Path: "src/example", Status: StatusOK}},
		Skills: []SkillEntry{
			{Source: "owner/repo", Agent: "claude-code", Status: StatusOK, Installed: []string{"already-there"}},
		},
		MCPs: []MCPEntry{{Name: "filesystem", Target: "/x/claude_desktop_config.json", Status: StatusPresent}},
	}
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, plan, status, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "up to date") {
		t.Errorf("expected 'up to date' for no-op, got:\n%s", out)
	}
}

func TestRenderSyncSummary_MCPTargetShowsBasename(t *testing.T) {
	plan := &PlanReport{
		MCPs: []PlanMCPEntry{{Name: "filesystem", Target: "/deeply/nested/path/claude_desktop_config.json", Action: PlanCreate}},
	}
	status := &StatusReport{
		MCPs: []MCPEntry{{Name: "filesystem", Target: "/deeply/nested/path/claude_desktop_config.json", Status: StatusPresent}},
	}
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, plan, status, 0); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "/deeply/nested/path") {
		t.Errorf("output leaks full MCP target path:\n%s", out)
	}
	if !strings.Contains(out, "claude_desktop_config.json") {
		t.Errorf("basename missing from output:\n%s", out)
	}
}

func TestRenderSyncSummary_ErrorMarker(t *testing.T) {
	plan := &PlanReport{
		Repositories: []PlanRepoEntry{{Path: "src/broken", Action: PlanError, Error: "fetch failed"}},
	}
	status := &StatusReport{
		Repositories: []RepoEntry{{Path: "src/broken", Status: StatusError, Error: "fetch failed"}},
	}
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, plan, status, 0); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ marker for error, got:\n%s", out)
	}
	if !strings.Contains(out, "fetch failed") {
		t.Errorf("expected error message in output, got:\n%s", out)
	}
}

func TestRenderSyncSummary_EmptySummaryPrintsCompleteLine(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, &PlanReport{}, &StatusReport{}, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "nothing to sync") {
		t.Errorf("expected 'nothing to sync', got:\n%s", out)
	}
	if !strings.Contains(out, "sync complete in") {
		t.Errorf("expected trailing 'sync complete in', got:\n%s", out)
	}
}

func TestRenderSyncSummary_NameColumnAlignment(t *testing.T) {
	plan := &PlanReport{
		Repositories: []PlanRepoEntry{{Path: "a", Action: PlanClone}},
		MCPs:         []PlanMCPEntry{{Name: "much-longer-name", Target: "/x/config.json", Action: PlanCreate}},
	}
	status := &StatusReport{
		Repositories: []RepoEntry{{Path: "a", Status: StatusOK}},
		MCPs:         []MCPEntry{{Name: "much-longer-name", Target: "/x/config.json", Status: StatusPresent}},
	}
	var buf bytes.Buffer
	if err := RenderSyncSummary(&buf, plan, status, 0); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Find the "a" line and the "much-longer-name" line. Their detail ("cloned"
	// and "upserted in …") should start at the same column because the
	// renderer pads names to the widest name in the summary.
	var aLine, mLine string
	for _, l := range lines {
		if strings.Contains(l, " a ") || strings.HasSuffix(l, " a  cloned") {
			aLine = l
		}
		if strings.Contains(l, "much-longer-name") {
			mLine = l
		}
	}
	if aLine == "" || mLine == "" {
		t.Fatalf("could not find both rows in:\n%s", buf.String())
	}
	aDetail := strings.Index(aLine, "cloned")
	mDetail := strings.Index(mLine, "upserted")
	if aDetail != mDetail {
		t.Errorf("detail column not aligned: a=%d, m=%d\n%s", aDetail, mDetail, buf.String())
	}
}
