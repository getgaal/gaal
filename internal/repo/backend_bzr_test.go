package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// VcsBazaar - IsCloned (no bzr binary needed)
// ---------------------------------------------------------------------------

func TestVcsBazaar_IsCloned_False(t *testing.T) {
	b := &VcsBazaar{}
	if b.IsCloned(t.TempDir()) {
		t.Error("expected IsCloned=false for dir without .bzr")
	}
}

func TestVcsBazaar_IsCloned_True(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bzr"), 0o755)
	b := &VcsBazaar{}
	if !b.IsCloned(dir) {
		t.Error("expected IsCloned=true for dir with .bzr")
	}
}

// ---------------------------------------------------------------------------
// VcsBazaar - error when bzr binary missing
// ---------------------------------------------------------------------------

func TestVcsBazaar_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	b := &VcsBazaar{}
	err := b.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when bzr binary missing")
	}
}

func TestVcsBazaar_Update_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	b := &VcsBazaar{}
	err := b.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when bzr binary missing")
	}
}

func TestVcsBazaar_CurrentVersion_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	b := &VcsBazaar{}
	_, err := b.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when bzr binary missing")
	}
}

// ---------------------------------------------------------------------------
// VcsBazaar - full code-path with fake bzr binary
// ---------------------------------------------------------------------------

func TestVcsBazaar_Clone_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "exit 0")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := b.Clone(context.Background(), "http://fake/repo", dest, ""); err != nil {
		t.Fatalf("Clone with fake bzr: %v", err)
	}
}

func TestVcsBazaar_Clone_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "exit 0")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := b.Clone(context.Background(), "http://fake/repo", dest, "1"); err != nil {
		t.Fatalf("Clone with fake bzr + version: %v", err)
	}
}

func TestVcsBazaar_Update_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "exit 0")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	if err := b.Update(context.Background(), t.TempDir(), ""); err != nil {
		t.Fatalf("Update with fake bzr: %v", err)
	}
}

func TestVcsBazaar_Update_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "exit 0")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	if err := b.Update(context.Background(), t.TempDir(), "2"); err != nil {
		t.Fatalf("Update with fake bzr + version: %v", err)
	}
}

func TestVcsBazaar_Update_PullFails(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "exit 1")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	err := b.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when bzr pull fails")
	}
}

func TestVcsBazaar_CurrentVersion_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "bzr", "echo 42")
	t.Setenv("PATH", binDir)
	b := &VcsBazaar{}
	ver, err := b.CurrentVersion(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CurrentVersion with fake bzr: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty version")
	}
}
