package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// VcsMercurial - IsCloned (no hg binary needed)
// ---------------------------------------------------------------------------

func TestVcsMercurial_IsCloned_False(t *testing.T) {
	m := &VcsMercurial{}
	if m.IsCloned(t.TempDir()) {
		t.Error("expected IsCloned=false for dir without .hg")
	}
}

func TestVcsMercurial_IsCloned_True(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hg"), 0o755)
	m := &VcsMercurial{}
	if !m.IsCloned(dir) {
		t.Error("expected IsCloned=true for dir with .hg")
	}
}

// ---------------------------------------------------------------------------
// VcsMercurial - error when hg binary missing
// ---------------------------------------------------------------------------

func TestVcsMercurial_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	m := &VcsMercurial{}
	err := m.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when hg binary missing")
	}
}

func TestVcsMercurial_Update_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	m := &VcsMercurial{}
	err := m.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when hg binary missing")
	}
}

func TestVcsMercurial_CurrentVersion_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	m := &VcsMercurial{}
	_, err := m.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when hg binary missing")
	}
}

// ---------------------------------------------------------------------------
// VcsMercurial - full code-path with fake hg binary
// ---------------------------------------------------------------------------

func TestVcsMercurial_Clone_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 0")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := m.Clone(context.Background(), "http://fake/repo", dest, ""); err != nil {
		t.Fatalf("Clone with fake hg: %v", err)
	}
}

func TestVcsMercurial_Clone_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 0")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := m.Clone(context.Background(), "http://fake/repo", dest, "tip"); err != nil {
		t.Fatalf("Clone with fake hg + version: %v", err)
	}
}

func TestVcsMercurial_Update_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 0")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	if err := m.Update(context.Background(), t.TempDir(), ""); err != nil {
		t.Fatalf("Update with fake hg: %v", err)
	}
}

func TestVcsMercurial_Update_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 0")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	if err := m.Update(context.Background(), t.TempDir(), "tip"); err != nil {
		t.Fatalf("Update with fake hg + version: %v", err)
	}
}

func TestVcsMercurial_Update_PullFails(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 1")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	err := m.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when hg pull fails")
	}
}

func TestVcsMercurial_CurrentVersion_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "echo abc123+")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	ver, err := m.CurrentVersion(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CurrentVersion with fake hg: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty version")
	}
}

func TestVcsMercurial_CurrentVersion_ExecError(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 1")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	_, err := m.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when hg exits with non-zero status")
	}
}

// ---------------------------------------------------------------------------
// VcsMercurial - HasChanges
// ---------------------------------------------------------------------------

func TestVcsMercurial_HasChanges_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	m := &VcsMercurial{}
	_, err := m.HasChanges(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when hg binary missing")
	}
}

func TestVcsMercurial_HasChanges_FakeBin_Clean(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 0")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	dirty, err := m.HasChanges(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HasChanges clean: %v", err)
	}
	if dirty {
		t.Error("expected HasChanges=false for empty hg status output")
	}
}

func TestVcsMercurial_HasChanges_FakeBin_Dirty(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "printf 'M file.go'")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	dirty, err := m.HasChanges(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HasChanges dirty: %v", err)
	}
	if !dirty {
		t.Error("expected HasChanges=true for non-empty hg status output")
	}
}

func TestVcsMercurial_HasChanges_ExecError(t *testing.T) {
	binDir := makeFakeBin(t, "hg", "exit 1")
	t.Setenv("PATH", binDir)
	m := &VcsMercurial{}
	_, err := m.HasChanges(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when hg status exits with non-zero status")
	}
}
