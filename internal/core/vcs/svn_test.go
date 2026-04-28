package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// VcsSVN - IsCloned (no svn binary needed)
// ---------------------------------------------------------------------------

func TestVcsSVN_IsCloned_False(t *testing.T) {
	s := &VcsSVN{}
	if s.IsCloned(t.TempDir()) {
		t.Error("expected IsCloned=false for dir without .svn")
	}
}

func TestVcsSVN_IsCloned_True(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".svn"), 0o755)
	s := &VcsSVN{}
	if !s.IsCloned(dir) {
		t.Error("expected IsCloned=true for dir with .svn")
	}
}

// ---------------------------------------------------------------------------
// VcsSVN - error when svn binary missing
// ---------------------------------------------------------------------------

func TestVcsSVN_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	s := &VcsSVN{}
	err := s.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when svn binary missing")
	}
}

func TestVcsSVN_Update_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	s := &VcsSVN{}
	err := s.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when svn binary missing")
	}
}

func TestVcsSVN_CurrentVersion_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	s := &VcsSVN{}
	_, err := s.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when svnversion binary missing")
	}
}

// ---------------------------------------------------------------------------
// VcsSVN - full code-path with fake svn / svnversion binaries
// ---------------------------------------------------------------------------

func TestVcsSVN_Clone_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 0")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := s.Clone(context.Background(), "http://fake/repo", dest, ""); err != nil {
		t.Fatalf("Clone with fake svn: %v", err)
	}
}

func TestVcsSVN_Clone_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 0")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	dest := filepath.Join(t.TempDir(), "repo")
	if err := s.Clone(context.Background(), "http://fake/repo", dest, "HEAD"); err != nil {
		t.Fatalf("Clone with fake svn + version: %v", err)
	}
}

func TestVcsSVN_Update_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 0")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	if err := s.Update(context.Background(), t.TempDir(), ""); err != nil {
		t.Fatalf("Update with fake svn: %v", err)
	}
}

func TestVcsSVN_Update_FakeBin_WithVersion(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 0")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	if err := s.Update(context.Background(), t.TempDir(), "42"); err != nil {
		t.Fatalf("Update with fake svn + version: %v", err)
	}
}

func TestVcsSVN_CurrentVersion_FakeBin(t *testing.T) {
	binDir := makeFakeBin(t, "svnversion", "echo 1234")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	ver, err := s.CurrentVersion(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CurrentVersion with fake svnversion: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty version")
	}
}

func TestVcsSVN_CurrentVersion_ExecError(t *testing.T) {
	binDir := makeFakeBin(t, "svnversion", "exit 1")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	_, err := s.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when svnversion exits with non-zero status")
	}
}

// ---------------------------------------------------------------------------
// VcsSVN - HasChanges
// ---------------------------------------------------------------------------

func TestVcsSVN_HasChanges_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	s := &VcsSVN{}
	_, err := s.HasChanges(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when svn binary missing")
	}
}

func TestVcsSVN_HasChanges_FakeBin_Clean(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 0")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	dirty, err := s.HasChanges(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HasChanges clean: %v", err)
	}
	if dirty {
		t.Error("expected HasChanges=false for empty svn status output")
	}
}

func TestVcsSVN_HasChanges_FakeBin_Dirty(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "echo M")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	dirty, err := s.HasChanges(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("HasChanges dirty: %v", err)
	}
	if !dirty {
		t.Error("expected HasChanges=true for non-empty svn status output")
	}
}

func TestVcsSVN_HasChanges_ExecError(t *testing.T) {
	binDir := makeFakeBin(t, "svn", "exit 1")
	t.Setenv("PATH", binDir)
	s := &VcsSVN{}
	_, err := s.HasChanges(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when svn status exits with non-zero status")
	}
}
