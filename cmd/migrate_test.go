package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/engine"
)

func resetMigrateFlags(t *testing.T) {
	t.Helper()
	origCfg := cfgFile
	origTarget := migrateTarget
	origDryRun := migrateDryRun
	origYes := migrateYes
	origOpts := engineOpts
	origResolved := resolvedCfg
	t.Cleanup(func() {
		cfgFile = origCfg
		migrateTarget = origTarget
		migrateDryRun = origDryRun
		migrateYes = origYes
		engineOpts = origOpts
		resolvedCfg = origResolved
	})
	migrateTarget = ""
	migrateDryRun = false
	migrateYes = false
}

func writeMinimalConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "gaal.yaml")
	data := []byte(`repositories:
  src/app:
    type: git
    url: https://github.com/example/app.git
skills:
  - source: example/skills
    agents: ["*"]
mcps:
  - name: filesystem
    target: ~/.config/claude/config.json
    inline:
      command: uvx
      args: [mcp-server-filesystem]
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrate_ValidConfig(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	setConfig(t, writeMinimalConfig(t, workDir))
	migrateTarget = "community"
	engineOpts = engine.Options{WorkDir: workDir}

	out := captureStdout(t, func() {
		if err := runMigrate(migrateCmd, []string{"https://community.example.com"}); err != nil {
			t.Fatalf("runMigrate: %v", err)
		}
	})

	if !strings.Contains(out, "1 repositories") {
		t.Errorf("expected repo count in output:\n%s", out)
	}
	if !strings.Contains(out, "1 skills") {
		t.Errorf("expected skill count in output:\n%s", out)
	}
	if !strings.Contains(out, "1 MCP servers") {
		t.Errorf("expected MCP count in output:\n%s", out)
	}
	if !strings.Contains(out, "community.example.com") {
		t.Errorf("expected URL in output:\n%s", out)
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected waiting message in output:\n%s", out)
	}
}

func TestMigrate_InvalidConfig(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	// Write an invalid config (missing required fields).
	bad := filepath.Join(workDir, "gaal.yaml")
	if err := os.WriteFile(bad, []byte("skills:\n  - agents: [\"*\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile = bad
	resolvedCfg = nil // invalid config — simulate load failure
	migrateTarget = "community"
	engineOpts = engine.Options{WorkDir: workDir}

	var gotErr error
	out := captureStdout(t, func() {
		gotErr = runMigrate(migrateCmd, []string{"https://community.example.com"})
	})
	if gotErr == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected disclaimer even on config error:\n%s", out)
	}
}

func TestMigrate_BadURL(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	setConfig(t, writeMinimalConfig(t, workDir))
	migrateTarget = "community"
	engineOpts = engine.Options{WorkDir: workDir}

	var gotErr error
	out := captureStdout(t, func() {
		gotErr = runMigrate(migrateCmd, []string{"not-a-url"})
	})
	if gotErr == nil {
		t.Fatal("expected error for bad URL")
	}
	if !strings.Contains(gotErr.Error(), "invalid URL") {
		t.Errorf("expected 'invalid URL' in error, got: %v", gotErr)
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected disclaimer even on bad URL:\n%s", out)
	}
}

func TestMigrate_UnknownTarget(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	setConfig(t, writeMinimalConfig(t, workDir))
	migrateTarget = "saas"
	engineOpts = engine.Options{WorkDir: workDir}

	var gotErr error
	out := captureStdout(t, func() {
		gotErr = runMigrate(migrateCmd, []string{"https://example.com"})
	})
	if gotErr == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(gotErr.Error(), "unknown migration target") {
		t.Errorf("expected 'unknown migration target' in error, got: %v", gotErr)
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected disclaimer even on unknown target:\n%s", out)
	}
}

func TestMigrate_NoArgs(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	setConfig(t, writeMinimalConfig(t, workDir))
	engineOpts = engine.Options{WorkDir: workDir}

	var gotErr error
	out := captureStdout(t, func() {
		gotErr = runMigrate(migrateCmd, nil)
	})
	if gotErr == nil {
		t.Fatal("expected error when no URL or --to provided")
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected disclaimer even with missing params:\n%s", out)
	}
}

func TestMigrate_DryRun(t *testing.T) {
	resetMigrateFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	setConfig(t, writeMinimalConfig(t, workDir))
	migrateTarget = "community"
	migrateDryRun = true
	engineOpts = engine.Options{WorkDir: workDir}

	out := captureStdout(t, func() {
		if err := runMigrate(migrateCmd, []string{"https://community.example.com"}); err != nil {
			t.Fatalf("runMigrate: %v", err)
		}
	})

	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected dry-run marker in output:\n%s", out)
	}
	if !strings.Contains(out, "not yet available") {
		t.Errorf("expected waiting message in output:\n%s", out)
	}
}
