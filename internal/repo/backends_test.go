package repo

import (
	"context"
	"testing"

	"gaal/internal/config"
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
