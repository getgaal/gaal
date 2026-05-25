package repo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"gaal/internal/config"
	"gaal/internal/core/vcs"
)

// ---------------------------------------------------------------------------
// NewManager / Sync / Status
// ---------------------------------------------------------------------------

func TestNewManager_Empty(t *testing.T) {
	m := NewManager(nil, "")
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestManager_Sync_Empty(t *testing.T) {
	m := NewManager(nil, "")
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync on empty manager: %v", err)
	}
}

func TestManager_Sync_ArchiveAlreadyCloned(t *testing.T) {
	// Archive.Update is a no-op, so this tests the Update path.
	existing := t.TempDir()
	repos := map[string]config.ConfigRepo{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos, "")
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with already-cloned archive: %v", err)
	}
}

func TestManager_Sync_UnknownType(t *testing.T) {
	repos := map[string]config.ConfigRepo{
		"/tmp/nope": {Type: "cvs", URL: "https://example.com/x"},
	}
	m := NewManager(repos, "")
	if err := m.Sync(context.Background()); err == nil {
		t.Fatal("expected error for unknown VCS type")
	}
}

func TestManager_Status_NotCloned(t *testing.T) {
	repos := map[string]config.ConfigRepo{
		"/tmp/not-cloned": {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos, "")
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
	repos := map[string]config.ConfigRepo{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos, "")
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Cloned {
		t.Error("expected Cloned=true for existing directory (archive)")
	}
}

func TestManager_Status_CurrentVersionError(t *testing.T) {
	existing := t.TempDir()
	// tar archive: IsCloned=true, CurrentVersion returns "archive"
	repos := map[string]config.ConfigRepo{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz", Version: "v1"},
	}
	m := NewManager(repos, "")
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Cloned {
		t.Error("expected Cloned=true")
	}
}

// ---------------------------------------------------------------------------
// Manager.Status - Dirty propagation
// ---------------------------------------------------------------------------

func TestManager_Status_DirtyFalse_Archive(t *testing.T) {
	existing := t.TempDir()
	repos := map[string]config.ConfigRepo{
		existing: {Type: "tar", URL: "https://example.com/x.tar.gz"},
	}
	m := NewManager(repos, "")
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Dirty {
		t.Error("expected Dirty=false for archive backend")
	}
}

// ---------------------------------------------------------------------------
// Manager.Sync - URL mismatch wraps *vcs.RemoteURLMismatchError (#220)
// ---------------------------------------------------------------------------

func TestManager_Sync_URLMismatchReturnsStructuredError(t *testing.T) {
	srcDir := t.TempDir()
	r, err := gogit.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := w.Add("f"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := w.Commit("c", &gogit.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "wc")
	gitBackend, err := vcs.New("git")
	if err != nil {
		t.Fatalf("vcs.New: %v", err)
	}
	if err := gitBackend.Clone(context.Background(), srcDir, destDir, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Rewrite origin so it disagrees with what we'll declare in gaal.yaml.
	rr, err := gogit.PlainOpen(destDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	cfg, err := rr.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	cfg.Remotes["origin"] = &gogitconfig.RemoteConfig{Name: "origin", URLs: []string{"git@gitlab.example.com:owner/repo.git"}}
	if err := rr.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	repos := map[string]config.ConfigRepo{
		destDir: {Type: "git", URL: "https://github.com/owner/repo.git"},
	}
	m := NewManager(repos, "")
	err = m.Sync(context.Background())
	if err == nil {
		t.Fatal("expected Sync to return an error")
	}
	var mm *vcs.RemoteURLMismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("expected wrapped *vcs.RemoteURLMismatchError, got %T: %v", err, err)
	}
	if mm.Path != destDir {
		t.Errorf("Path = %q, want %q", mm.Path, destDir)
	}
}
