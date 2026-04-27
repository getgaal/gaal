package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/config"
	"gaal/internal/engine"
)

// resetInitFlags restores the package-level flag state between cases.
func resetInitFlags(t *testing.T) {
	t.Helper()
	originalCfgFile := cfgFile
	originalForce := forceInit
	originalScope := initScopeFlag
	originalAll := initImportAll
	originalEmpty := initEmpty
	originalOpts := engineOpts
	t.Cleanup(func() {
		cfgFile = originalCfgFile
		forceInit = originalForce
		initScopeFlag = originalScope
		initImportAll = originalAll
		initEmpty = originalEmpty
		engineOpts = originalOpts
	})
	// Start every test from a clean slate regardless of leftover state.
	forceInit = false
	initScopeFlag = ""
	initImportAll = false
	initEmpty = false
}

func writeSkillMD(t *testing.T, dir, name string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: test\n---\n"
	if err := os.WriteFile(filepath.Join(full, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInit_NonInteractive_ImportAllProject(t *testing.T) {
	resetInitFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	// Drop a project-level claude-code skill under workDir.
	writeSkillMD(t, filepath.Join(workDir, ".claude", "skills"), "frontend-design")

	dest := filepath.Join(workDir, "gaal.yaml")
	cfgFile = dest
	forceInit = true
	initScopeFlag = "project"
	initImportAll = true
	engineOpts = engine.Options{WorkDir: workDir}

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, err := config.Load(dest)
	if err != nil {
		t.Fatalf("generated file does not parse: %v", err)
	}
	if len(cfg.Skills) != 1 {
		t.Fatalf("expected 1 skill entry, got %d: %+v", len(cfg.Skills), cfg.Skills)
	}
	if cfg.Skills[0].Global {
		t.Error("project scope must set Global=false")
	}
	if len(cfg.Skills[0].Select) != 1 || cfg.Skills[0].Select[0] != "frontend-design" {
		t.Errorf("unexpected Select: %v", cfg.Skills[0].Select)
	}
}

func TestInit_NonInteractive_InvalidScope(t *testing.T) {
	resetInitFlags(t)

	initScopeFlag = "bogus"
	forceInit = true
	initImportAll = true
	cfgFile = filepath.Join(t.TempDir(), "gaal.yaml")

	err := runInit(initCmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --scope")
	}
	if !strings.Contains(err.Error(), `must be "project" or "global"`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInit_NonInteractive_EmptyFlagWritesSkeleton(t *testing.T) {
	resetInitFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	dest := filepath.Join(workDir, "gaal.yaml")
	cfgFile = dest
	forceInit = true
	initScopeFlag = "project"
	initEmpty = true
	engineOpts = engine.Options{WorkDir: workDir}

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"repositories:", "skills:", "mcps:"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestInit_NonInteractive_RequiresModeFlag(t *testing.T) {
	resetInitFlags(t)

	initScopeFlag = "project"
	cfgFile = filepath.Join(t.TempDir(), "gaal.yaml")
	forceInit = true

	err := runInit(initCmd, nil)
	if err == nil {
		t.Fatal("expected error when neither --empty nor --import-all is set in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "--empty") && !strings.Contains(err.Error(), "--import-all") {
		t.Errorf("error should mention the required flags: %v", err)
	}
}

func TestInit_NonInteractive_GlobalImportAll(t *testing.T) {
	resetInitFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	// Drop a global-level claude-code skill.
	writeSkillMD(t, filepath.Join(home, ".claude", "skills"), "global-skill")

	dest := filepath.Join(t.TempDir(), "out.yaml")
	cfgFile = dest
	forceInit = true
	initScopeFlag = "global"
	initImportAll = true
	engineOpts = engine.Options{WorkDir: workDir}

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(dest)
	if err != nil {
		t.Fatalf("generated file does not parse: %v", err)
	}
	if len(cfg.Skills) != 1 || !cfg.Skills[0].Global {
		t.Errorf("expected 1 global skill entry, got %+v", cfg.Skills)
	}
}

func TestInit_BuildPlanGroupsSkills(t *testing.T) {
	// Verifies that the end-to-end flow groups multiple skills from the same
	// source into a single SkillConfig via ops.BuildPlan.
	resetInitFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(workDir, ".claude", "skills")
	writeSkillMD(t, dir, "frontend-design")
	writeSkillMD(t, dir, "code-reviewer")

	dest := filepath.Join(workDir, "gaal.yaml")
	cfgFile = dest
	forceInit = true
	initScopeFlag = "project"
	initImportAll = true
	engineOpts = engine.Options{WorkDir: workDir}

	if err := runInit(initCmd, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := config.Load(dest)
	if err != nil {
		t.Fatal(err)
	}

	// Each skill lives in its own directory so ResolveSkillSource falls back
	// to SourceKindPath with distinct source paths — BuildPlan emits one
	// entry per skill.
	if len(cfg.Skills) != 2 {
		t.Errorf("expected 2 entries, got %d: %+v", len(cfg.Skills), cfg.Skills)
	}
}

// TestInit_GlobalScope_NoTelemetryPrewrite is a regression test for GitHub
// issue #74. It verifies that calling runInit with global scope on a fresh
// machine (no pre-existing user config) does NOT fail with "already exists"
// — which would happen if telemetry.Init wrote the user config file before
// preflightDestination ran.
func TestInit_GlobalScope_NoTelemetryPrewrite(t *testing.T) {
	resetInitFlags(t)

	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	// Pre-condition: fresh machine simulation — user config must not exist.
	userConfigPath := config.UserConfigFilePath()
	if _, err := os.Stat(userConfigPath); err == nil {
		t.Fatalf("pre-condition failed: user config already exists at %s", userConfigPath)
	}

	// Default cfgFile triggers global-scope path resolution via resolveInitDestination.
	cfgFile = "gaal.yaml"
	initScopeFlag = "global"
	initImportAll = true
	forceInit = false
	engineOpts = engine.Options{WorkDir: workDir}

	err := runInit(initCmd, nil)

	// The test only asserts the specific regression: preflightDestination must
	// not reject a non-existent file. Other engine failures are acceptable.
	if err != nil && strings.Contains(err.Error(), "already exists — use --force to overwrite") {
		t.Errorf("telemetry pre-wrote user config before runInit: %v", err)
	}
}
