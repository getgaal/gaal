package repo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gaal/internal/config"
)

// ---------------------------------------------------------------------------
// NewManager / Sync / Status
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
// Fake-binary helper for subprocess backends (hg / svn / bzr)
// ---------------------------------------------------------------------------

// makeFakeBin writes a minimal executable named `name` that executes `script`
// and returns the directory path. The caller must add that dir to PATH.
// On Windows a .bat wrapper is used because shell scripts are not executable.
func makeFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		bin := filepath.Join(binDir, name+".bat")
		os.WriteFile(bin, []byte("@echo off\n"+script+"\n"), 0o755)
	} else {
		bin := filepath.Join(binDir, name)
		os.WriteFile(bin, []byte("#!/bin/sh\n"+script+"\n"), 0o755)
	}
	return binDir
}
