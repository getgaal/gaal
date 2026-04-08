package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gaal/internal/config"
)

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
	// Status writes to stdout — capture via os.Pipe so the test is side-effect free.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	statusErr := e.Status(context.Background(), FormatTable)

	w.Close()
	os.Stdout = origStdout
	r.Close()

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

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	statusErr := e.Status(context.Background(), FormatTable)
	w.Close()
	os.Stdout = origStdout
	r.Close()

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

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	e.Status(context.Background(), FormatTable)
	w.Close()
	os.Stdout = origStdout
	r.Close()
}

func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Errorf("orDefault empty: got %q, want fallback", got)
	}
	if got := orDefault("value", "fallback"); got != "value" {
		t.Errorf("orDefault non-empty: got %q, want value", got)
	}
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
	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w
	statusErr := e.Status(context.Background(), FormatTable)
	w.Close()
	os.Stdout = origStdout
	r.Close()
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
	r, w, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = w
	statusErr := e.Status(context.Background(), FormatTable)
	w.Close()
	os.Stdout = origStdout
	r.Close()
	if statusErr != nil {
		t.Fatalf("Status should not error even with skill error entries: %v", statusErr)
	}
}
