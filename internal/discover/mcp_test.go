package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// makeMCPConfig writes a fake MCP JSON config file and returns its path.
func makeMCPConfig(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestComputeMCPDrift_noStateDir returns DriftUnknown with empty stateDir.
func TestComputeMCPDrift_noStateDir(t *testing.T) {
	cfgFile := makeMCPConfig(t, t.TempDir(), "mcp.json")
	got := computeMCPDrift(cfgFile, "")
	if got != DriftUnknown {
		t.Errorf("got %q, want DriftUnknown", got)
	}
}

// TestComputeMCPDrift_noSnapshot returns DriftUnknown when no snapshot exists.
func TestComputeMCPDrift_noSnapshot(t *testing.T) {
	dir := t.TempDir()
	cfgFile := makeMCPConfig(t, dir, "mcp.json")
	got := computeMCPDrift(cfgFile, t.TempDir())
	if got != DriftUnknown {
		t.Errorf("got %q, want DriftUnknown", got)
	}
}

// TestComputeMCPDrift_ok returns DriftOK when file matches snapshot.
func TestComputeMCPDrift_ok(t *testing.T) {
	stateDir := t.TempDir()
	cfgFile := makeMCPConfig(t, t.TempDir(), "mcp.json")

	// Build and save a snapshot for this file.
	rec, err := Record(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{filepath.Base(cfgFile): rec}
	key := "mcp-" + WorkdirKey(cfgFile)
	if err := Save(SnapshotPath(stateDir, key), snap); err != nil {
		t.Fatal(err)
	}

	got := computeMCPDrift(cfgFile, stateDir)
	if got != DriftOK {
		t.Errorf("got %q, want DriftOK", got)
	}
}

// TestComputeMCPDrift_modified returns DriftModified when file content changed.
func TestComputeMCPDrift_modified(t *testing.T) {
	stateDir := t.TempDir()
	cfgFile := makeMCPConfig(t, t.TempDir(), "mcp.json")

	rec, _ := Record(cfgFile)
	snap := Snapshot{filepath.Base(cfgFile): rec}
	key := "mcp-" + WorkdirKey(cfgFile)
	_ = Save(SnapshotPath(stateDir, key), snap)

	// Modify the file.
	if err := os.WriteFile(cfgFile, []byte(`{"mcpServers":{"new":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := computeMCPDrift(cfgFile, stateDir)
	if got != DriftModified {
		t.Errorf("got %q, want DriftModified", got)
	}
}

// TestComputeMCPDrift_missing returns DriftMissing when the file was deleted.
func TestComputeMCPDrift_missing(t *testing.T) {
	stateDir := t.TempDir()
	fileDir := t.TempDir()
	cfgFile := makeMCPConfig(t, fileDir, "mcp.json")

	rec, _ := Record(cfgFile)
	snap := Snapshot{filepath.Base(cfgFile): rec}
	key := "mcp-" + WorkdirKey(cfgFile)
	_ = Save(SnapshotPath(stateDir, key), snap)

	// Delete the file.
	if err := os.Remove(cfgFile); err != nil {
		t.Fatal(err)
	}

	got := computeMCPDrift(cfgFile, stateDir)
	if got != DriftMissing {
		t.Errorf("got %q, want DriftMissing", got)
	}
}
