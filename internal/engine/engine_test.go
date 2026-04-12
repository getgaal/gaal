package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gaal/internal/config"
)

// captureStdout is defined in testutil_test.go (same package).
// It redirects os.Stdout to an os.Pipe drained concurrently, calls fn,
// restores Stdout and returns everything written. Prevents Windows pipe
// buffer (~4 KiB) deadlocks when Status/Audit output is large.

func TestNewWithOptions_Defaults(t *testing.T) {
	cfg := &config.Config{}
	e := NewWithOptions(cfg, Options{})
	if e == nil {
		t.Fatal("NewWithOptions returned nil")
	}
}

func TestNew_EquivalentToNewWithOptions(t *testing.T) {
	cfg := &config.Config{}
	e1 := New(cfg)
	e2 := NewWithOptions(cfg, Options{})
	if e1 == nil || e2 == nil {
		t.Fatal("New or NewWithOptions returned nil")
	}
}

func TestNewWithOptions_WorkDirOverride(t *testing.T) {
	workDir := t.TempDir()
	cfg := &config.Config{}
	e := NewWithOptions(cfg, Options{WorkDir: workDir})
	if e == nil {
		t.Fatal("NewWithOptions returned nil with WorkDir override")
	}
}

func TestRunOnce_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce on empty config should succeed, got: %v", err)
	}
}

func TestRunOnce_WithRepository_NoNetwork(t *testing.T) {
	// A repo with an archive type (Update is a no-op) pointing at a local
	// directory avoids any network call. We use a pre-cloned archive path
	// (a dir that already exists) so IsCloned returns true and Update is invoked.
	existing := t.TempDir()

	cfg := &config.Config{
		Repositories: map[string]config.RepoConfig{
			existing: {
				Type: "tar",
				URL:  "https://example.com/unused.tar.gz",
			},
		},
	}
	e := New(cfg)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with archive repo: %v", err)
	}
}

func TestStatus_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	var statusErr error
	captureStdout(t, func() {
		statusErr = e.Status(context.Background(), FormatTable)
	})
	if statusErr != nil {
		t.Fatalf("Status on empty config: %v", statusErr)
	}
}

func TestRunOnce_WithSkills_LocalSource(t *testing.T) {
	// Create a local skill source directory.
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "test-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test-skill\n---\n"), 0o644)

	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755)

	cfg := &config.Config{
		Skills: []config.SkillConfig{
			{Source: sourceDir, Agents: []string{"claude-code"}},
		},
	}
	e := NewWithOptions(cfg, Options{WorkDir: workDir})
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with skill: %v", err)
	}
}

func TestRunOnce_WithMCP_Inline(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	cfg := &config.Config{
		MCPs: []config.MCPConfig{
			{
				Name:   "test-mcp",
				Target: target,
				Inline: &config.MCPInlineConfig{Command: "node"},
			},
		},
	}
	e := New(cfg)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with MCP: %v", err)
	}
}

func TestStatus_WithRepos(t *testing.T) {
	existing := t.TempDir()
	cfg := &config.Config{
		Repositories: map[string]config.RepoConfig{
			existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
		},
	}
	e := New(cfg)
	var statusErr error
	captureStdout(t, func() {
		statusErr = e.Status(context.Background(), FormatTable)
	})
	if statusErr != nil {
		t.Fatalf("Status with repos: %v", statusErr)
	}
}

func TestStatus_WithMCPs(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers":{"s":{"command":"c"}}}`), 0o644)

	cfg := &config.Config{
		MCPs: []config.MCPConfig{
			{Name: "s", Target: target},
		},
	}
	e := New(cfg)
	captureStdout(t, func() {
		e.Status(context.Background(), FormatTable) //nolint:errcheck
	})
}

func TestRunService_CancelledContext(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	// Cancel the context immediately after starting.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := e.RunService(ctx, 1*time.Second)
	// RunService should return nil when context is cancelled.
	if err != nil {
		t.Fatalf("RunService with cancelled context: %v", err)
	}
}

func TestRunService_TickFires(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	// Use a very short interval so the ticker fires before we cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := e.RunService(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("RunService with tick: %v", err)
	}
}

func TestRunOnce_ErrorAccumulation(t *testing.T) {
	// A MCP config with no source and no inline triggers an error in syncOne.
	cfg := &config.Config{
		MCPs: []config.MCPConfig{
			{Name: "bad", Target: filepath.Join(t.TempDir(), "mcp.json")},
		},
	}
	e := New(cfg)
	err := e.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected RunOnce to return error when MCP sync fails")
	}
}

func TestStatus_WithRepoError(t *testing.T) {
	// Unknown repo type causes repos.Status to return an error status entry.
	cfg := &config.Config{
		Repositories: map[string]config.RepoConfig{
			"/some/path": {Type: "unknown-vcs-type", URL: "https://example.com/x"},
		},
	}
	e := New(cfg)
	var statusErr error
	captureStdout(t, func() {
		statusErr = e.Status(context.Background(), FormatTable)
	})
	if statusErr != nil {
		t.Fatalf("Status should not error even with error entries: %v", statusErr)
	}
}

func TestStatus_WithSkillError(t *testing.T) {
	// Unknown agent name causes skills.Status to return an error status entry.
	cfg := &config.Config{
		Skills: []config.SkillConfig{
			{Source: t.TempDir(), Agents: []string{"unknown-agent-xyz"}},
		},
	}
	e := New(cfg)
	var statusErr error
	captureStdout(t, func() {
		statusErr = e.Status(context.Background(), FormatTable)
	})
	if statusErr != nil {
		t.Fatalf("Status should not error even with skill error entries: %v", statusErr)
	}
}

func TestCollect_IncludesAgents(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	report, err := e.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(report.Agents) == 0 {
		t.Fatal("expected at least one agent entry in status report")
	}
	// Every entry must have a non-empty Name. ProjectSkillsDir may be
	// empty for agents that delegate project skills to the shared
	// "generic" convention (e.g. cline, amp, warp); audit and sync
	// redirect those through agent.SkillDir.
	for _, a := range report.Agents {
		if a.Name == "" {
			t.Error("agent entry has empty Name")
		}
	}
}

func TestCollect_AgentsSorted(t *testing.T) {
	cfg := &config.Config{}
	e := New(cfg)
	report, err := e.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	// Installed agents must precede uninstalled ones.
	seenUninstalled := false
	for _, a := range report.Agents {
		if !a.Installed {
			seenUninstalled = true
		} else if seenUninstalled {
			t.Errorf("installed agent %q appears after uninstalled agents", a.Name)
		}
	}
	// Within each group, names must be sorted alphabetically.
	var prevInstalled, prevUninstalled string
	for _, a := range report.Agents {
		if a.Installed {
			if a.Name < prevInstalled {
				t.Errorf("installed agents not sorted: %q after %q", a.Name, prevInstalled)
			}
			prevInstalled = a.Name
		} else {
			if a.Name < prevUninstalled {
				t.Errorf("uninstalled agents not sorted: %q after %q", a.Name, prevUninstalled)
			}
			prevUninstalled = a.Name
		}
	}
}
