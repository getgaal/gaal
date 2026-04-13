package discover

import (
	"path/filepath"
	"testing"
	"time"
)

// TestApplyDefaults fills zero values correctly.
func TestApplyDefaults(t *testing.T) {
	o := applyDefaults(ScanOptions{})
	if o.MaxDepth != defaultMaxDepth {
		t.Errorf("MaxDepth: got %d, want %d", o.MaxDepth, defaultMaxDepth)
	}
	if o.Timeout != defaultTimeout {
		t.Errorf("Timeout: got %v, want %v", o.Timeout, defaultTimeout)
	}
}

// TestApplyDefaults_preservesExplicit does not overwrite explicit values.
func TestApplyDefaults_preservesExplicit(t *testing.T) {
	o := applyDefaults(ScanOptions{MaxDepth: 2, Timeout: 5 * time.Second})
	if o.MaxDepth != 2 {
		t.Errorf("MaxDepth should be 2, got %d", o.MaxDepth)
	}
	if o.Timeout != 5*time.Second {
		t.Errorf("Timeout should be 5s, got %v", o.Timeout)
	}
}

// TestScan_dedup does not return the same path twice even if multiple
// scanners would find it (global + workspace).
func TestScan_dedup(t *testing.T) {
	root := t.TempDir()
	// Place a skill that would be discoverable from the workspace walk.
	skillDir := root
	makeSkillDir(t, skillDir, "root-skill", "")

	results, err := Scan(t.Context(), t.TempDir(), root, ScanOptions{
		IncludeWorkspace: true,
		MaxDepth:         1,
		Timeout:          defaultTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, r := range results {
		seen[r.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("path %q appears %d times, expected 1", path, count)
		}
	}
}

// TestScan_noWorkspace skips workspace scan when IncludeWorkspace is false.
func TestScan_noWorkspace(t *testing.T) {
	root := t.TempDir()
	// Create a skill that would only be found by workspace walk.
	skillDir := filepath.Join(root, "hidden")
	makeSkillDir(t, skillDir, "hidden-skill", "")

	results, err := Scan(t.Context(), t.TempDir(), root, ScanOptions{
		IncludeWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == skillDir {
			t.Errorf("workspace skill should not appear when IncludeWorkspace=false")
		}
	}
}
