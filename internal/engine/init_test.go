package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/config"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return NewWithOptions(&config.Config{}, Options{WorkDir: t.TempDir()})
}

func TestInit_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "gaal.yaml")

	if err := newTestEngine(t).Init(dest, false); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	for _, want := range []string{"repositories:", "skills:", "mcps:", "gaal init"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated file missing %q", want)
		}
	}
}

func TestInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "gaal.yaml")
	if err := os.WriteFile(dest, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := newTestEngine(t).Init(dest, false)
	if err == nil {
		t.Fatal("expected error when file exists without force, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error text: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "existing" {
		t.Error("file was overwritten without force")
	}
}

func TestInit_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "gaal.yaml")
	if err := os.WriteFile(dest, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := newTestEngine(t).Init(dest, true); err != nil {
		t.Fatalf("Init force: %v", err)
	}

	data, _ := os.ReadFile(dest)
	if string(data) == "old content" {
		t.Error("force did not overwrite the file")
	}
	if !strings.Contains(string(data), "gaal init") {
		t.Error("overwritten file missing expected marker")
	}
}

func TestInit_StatError(t *testing.T) {
	// Path inside a non-existent parent triggers os.WriteFile error.
	err := newTestEngine(t).Init("/nonexistent-dir/gaal.yaml", false)
	if err == nil {
		t.Fatal("expected an error for unwritable path, got nil")
	}
}
