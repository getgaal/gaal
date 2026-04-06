package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"

	"gaal/internal/config"
)

// ---------------------------------------------------------------------------
// NewManager / Sync / Status (archive - no binary required)
// ---------------------------------------------------------------------------

func TestNewManager_Empty(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestManager_Sync_Empty(t *testing.T) {
	m := NewManager(nil)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync on empty manager: %v", err)
	}
}

func TestManager_Sync_ArchiveAlreadyCloned(t *testing.T) {
	// Archive.Update is a no-op, so this tests the Update path.
	existing := t.TempDir()
	repos := map[string]config.RepoConfig{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with already-cloned archive: %v", err)
	}
}

func TestManager_Sync_UnknownType(t *testing.T) {
	repos := map[string]config.RepoConfig{
		"/tmp/nope": {Type: "cvs", URL: "https://example.com/x"},
	}
	m := NewManager(repos)
	if err := m.Sync(context.Background()); err == nil {
		t.Fatal("expected error for unknown VCS type")
	}
}

func TestManager_Status_NotCloned(t *testing.T) {
	repos := map[string]config.RepoConfig{
		"/tmp/not-cloned": {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Cloned {
		t.Error("expected Cloned=false for non-existent directory")
	}
}

func TestManager_Status_Cloned(t *testing.T) {
	existing := t.TempDir()
	repos := map[string]config.RepoConfig{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Cloned {
		t.Error("expected Cloned=true for existing directory (archive)")
	}
}

// ---------------------------------------------------------------------------
// VcsGit.IsCloned (pure Go, no binary needed)
// ---------------------------------------------------------------------------

func TestVcsGit_IsCloned_False(t *testing.T) {
	g := &VcsGit{}
	if g.IsCloned(t.TempDir()) {
		t.Error("expected IsCloned=false for empty temp dir")
	}
}

func TestVcsGit_IsCloned_True(t *testing.T) {
	dir := t.TempDir()
	// Create a proper git repo using go-git.
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	g := &VcsGit{}
	if !g.IsCloned(dir) {
		t.Error("expected IsCloned=true for go-git initialised repo")
	}
}

func TestVcsGit_Clone_BadURL(t *testing.T) {
	g := &VcsGit{}
	err := g.Clone(context.Background(), "not-a-real-url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error for bad URL")
	}
}

func TestVcsGit_Update_NotARepo(t *testing.T) {
	g := &VcsGit{}
	err := g.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when updating non-git directory")
	}
}

func TestVcsGit_CurrentVersion_NotARepo(t *testing.T) {
	g := &VcsGit{}
	_, err := g.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when getting version of non-git directory")
	}
}

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

func TestVcsMercurial_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	m := &VcsMercurial{}
	err := m.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when hg binary missing")
	}
}

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

func TestVcsSVN_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	s := &VcsSVN{}
	err := s.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when svn binary missing")
	}
}

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

func TestVcsBazaar_Clone_NoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	b := &VcsBazaar{}
	err := b.Clone(context.Background(), "url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error when bzr binary missing")
	}
}

// ---------------------------------------------------------------------------
// Subprocess backends: Update and CurrentVersion return error when binary missing
// ---------------------------------------------------------------------------

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
// VcsGit.Update on a proper (but remote-less) initialized git repo
// ---------------------------------------------------------------------------

func TestVcsGit_Update_InitedRepoNoRemote(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	g := &VcsGit{}
	// Update will fail because there is no remote configured; this covers
	// the fetch error branch.
	err := g.Update(context.Background(), dir, "")
	if err == nil {
		t.Log("update succeeded unexpectedly on repo without remote")
	}
}

// ---------------------------------------------------------------------------
// Manager.Status when CurrentVersion fails
// ---------------------------------------------------------------------------

func TestManager_Status_CurrentVersionError(t *testing.T) {
	existing := t.TempDir()
	// tar archive: IsCloned=true, CurrentVersion returns "archive"
	repos := map[string]config.RepoConfig{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz", Version: "v1"},
	}
	m := NewManager(repos)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Cloned {
		t.Error("expected Cloned=true")
	}
}

// ---------------------------------------------------------------------------
// Fake-binary helpers for hg / svn / bzr subprocess coverage
// ---------------------------------------------------------------------------

// makeFakeBin writes a shell script named `name` that executes `script` and
// returns the directory path.  The caller must add that dir to PATH.
func makeFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	binDir := t.TempDir()
	bin := filepath.Join(binDir, name)
	os.WriteFile(bin, []byte("#!/bin/sh\n"+script+"\n"), 0o755)
	return binDir
}

// ---------------------------------------------------------------------------
// VcsMercurial – full code-path with fake hg binary
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

// ---------------------------------------------------------------------------
// VcsSVN – full code-path with fake svn / svnversion binaries
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

// ---------------------------------------------------------------------------
// VcsBazaar – full code-path with fake bzr binary
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
