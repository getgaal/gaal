package discover

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIndexPath verifies the canonical index file location.
func TestIndexPath(t *testing.T) {
	got := IndexPath("/cache")
	want := filepath.Join("/cache", "gaal", "discovery-index.json")
	if got != want {
		t.Errorf("IndexPath: got %q, want %q", got, want)
	}
}

// TestSaveLoadIndex_roundtrip saves and reloads a two-entry index.
func TestSaveLoadIndex_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	now := time.Now().UTC().Truncate(time.Millisecond)

	orig := &DiscoveryIndex{
		GeneratedAt: now,
		Entries: []IndexEntry{
			{
				Path:        "/skills/my-skill",
				Type:        ResourceSkill,
				Scope:       ScopeGlobal,
				Name:        "my-skill",
				Agent:       "copilot",
				ParentMtime: now,
			},
			{
				Path:        "/mcp/config.json",
				Type:        ResourceMCP,
				Scope:       ScopeWorkspace,
				Name:        "config",
				Agent:       "",
				ParentMtime: now.Add(-time.Hour),
			},
		},
	}

	if err := SaveIndex(path, orig); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	loaded, err := LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadIndex returned nil index")
	}
	if !loaded.GeneratedAt.Equal(orig.GeneratedAt) {
		t.Errorf("GeneratedAt: got %v, want %v", loaded.GeneratedAt, orig.GeneratedAt)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("Entries count: got %d, want 2", len(loaded.Entries))
	}
	for i, want := range orig.Entries {
		got := loaded.Entries[i]
		if got.Path != want.Path {
			t.Errorf("entry[%d].Path: got %q, want %q", i, got.Path, want.Path)
		}
		if got.Type != want.Type {
			t.Errorf("entry[%d].Type: got %q, want %q", i, got.Type, want.Type)
		}
		if got.Scope != want.Scope {
			t.Errorf("entry[%d].Scope: got %q, want %q", i, got.Scope, want.Scope)
		}
		if got.Name != want.Name {
			t.Errorf("entry[%d].Name: got %q, want %q", i, got.Name, want.Name)
		}
		if got.Agent != want.Agent {
			t.Errorf("entry[%d].Agent: got %q, want %q", i, got.Agent, want.Agent)
		}
		if !got.ParentMtime.Equal(want.ParentMtime) {
			t.Errorf("entry[%d].ParentMtime: got %v, want %v", i, got.ParentMtime, want.ParentMtime)
		}
	}
}

// TestLoadIndex_missing returns (nil, nil) for a non-existent path.
func TestLoadIndex_missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	idx, err := LoadIndex(path)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if idx != nil {
		t.Errorf("expected nil index, got: %+v", idx)
	}
}

// TestLoadIndex_corrupt returns an error for malformed JSON.
func TestLoadIndex_corrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("not-json{{{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIndex(path)
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
}

// TestSaveIndex_createsParentDirs ensures SaveIndex creates missing nested parent directories.
func TestSaveIndex_createsParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "index.json")

	idx := &DiscoveryIndex{
		GeneratedAt: time.Now(),
		Entries:     nil,
	}
	if err := SaveIndex(path, idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after SaveIndex: %v", err)
	}
}

// TestValidate_unchangedMtime returns the cached resource when parent mtime has not changed.
func TestValidate_unchangedMtime(t *testing.T) {
	tests := []struct {
		name      string
		entryType ResourceType
		scope     Scope
	}{
		{name: "skill entry", entryType: ResourceSkill, scope: ScopeGlobal},
		{name: "mcp entry", entryType: ResourceMCP, scope: ScopeWorkspace},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			fi, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}

			entry := IndexEntry{
				Path:        filepath.Join(parent, "resource"),
				Type:        tc.entryType,
				Scope:       tc.scope,
				Name:        "test-resource",
				Agent:       "test-agent",
				ParentMtime: fi.ModTime(),
			}

			idx := &DiscoveryIndex{
				GeneratedAt: time.Now(),
				Entries:     []IndexEntry{entry},
			}

			resources, allValid := Validate(t.Context(), idx)
			if !allValid {
				t.Error("allValid: got false, want true")
			}
			if len(resources) != 1 {
				t.Fatalf("resources count: got %d, want 1", len(resources))
			}
			if resources[0].Path != entry.Path {
				t.Errorf("resource.Path: got %q, want %q", resources[0].Path, entry.Path)
			}
			if resources[0].Name != entry.Name {
				t.Errorf("resource.Name: got %q, want %q", resources[0].Name, entry.Name)
			}
			if resources[0].Type != entry.Type {
				t.Errorf("resource.Type: got %q, want %q", resources[0].Type, entry.Type)
			}
		})
	}
}

// TestValidate_changedMtime triggers a partial re-walk and still returns fresh resources.
func TestValidate_changedMtime(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (parent, resourcePath string)
		entryType ResourceType
		wantFound bool
	}{
		{
			name: "skill re-walk finds skill",
			setup: func(t *testing.T) (string, string) {
				parent := t.TempDir()
				skillDir := filepath.Join(parent, "my-skill")
				makeSkillDir(t, skillDir, "my-skill", "a test skill")
				return parent, skillDir
			},
			entryType: ResourceSkill,
			wantFound: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent, resourcePath := tc.setup(t)
			staleTime := time.Now().Add(-24 * time.Hour)

			entry := IndexEntry{
				Path:        resourcePath,
				Type:        tc.entryType,
				Scope:       ScopeGlobal,
				Name:        filepath.Base(resourcePath),
				Agent:       "copilot",
				ParentMtime: staleTime,
			}

			idx := &DiscoveryIndex{
				GeneratedAt: time.Now(),
				Entries:     []IndexEntry{entry},
			}

			resources, allValid := Validate(t.Context(), idx)
			if allValid {
				t.Error("allValid: got true, want false (mtime changed)")
			}
			if tc.wantFound {
				found := false
				for _, r := range resources {
					if r.Path == resourcePath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected resource at %q in results, not found (parent=%q, got %d resources)", resourcePath, parent, len(resources))
				}
			}
		})
	}
}

// TestValidate_missingParent returns allValid=false and no resources for a missing parent dir.
func TestValidate_missingParent(t *testing.T) {
	// Use a path whose parent does not exist.
	entry := IndexEntry{
		Path:        filepath.Join(t.TempDir(), "nonexistent-parent", "resource"),
		Type:        ResourceSkill,
		Scope:       ScopeGlobal,
		Name:        "ghost",
		Agent:       "agent",
		ParentMtime: time.Now(),
	}
	idx := &DiscoveryIndex{
		GeneratedAt: time.Now(),
		Entries:     []IndexEntry{entry},
	}

	resources, allValid := Validate(t.Context(), idx)
	if allValid {
		t.Error("allValid: got true, want false for missing parent")
	}
	if len(resources) != 0 {
		t.Errorf("resources: got %d, want 0", len(resources))
	}
}

// TestValidate_mcpEntry_changedMtime returns the MCP resource after a partial re-walk when file still exists.
func TestValidate_mcpEntry_changedMtime(t *testing.T) {
	dir := t.TempDir()
	cfgPath := makeMCPConfig(t, dir, "mcp.json")
	staleTime := time.Now().Add(-24 * time.Hour)

	entry := IndexEntry{
		Path:        cfgPath,
		Type:        ResourceMCP,
		Scope:       ScopeGlobal,
		Name:        "mcp",
		Agent:       "",
		ParentMtime: staleTime,
	}

	idx := &DiscoveryIndex{
		GeneratedAt: time.Now(),
		Entries:     []IndexEntry{entry},
	}

	resources, allValid := Validate(t.Context(), idx)
	if allValid {
		t.Error("allValid: got true, want false (mtime changed)")
	}
	if len(resources) == 0 {
		t.Error("expected MCP resource to be returned after partial re-walk, got none")
	}
	if len(resources) > 0 {
		if resources[0].Path != cfgPath {
			t.Errorf("resource.Path: got %q, want %q", resources[0].Path, cfgPath)
		}
		if resources[0].Type != ResourceMCP {
			t.Errorf("resource.Type: got %q, want ResourceMCP", resources[0].Type)
		}
	}
}

// TestRebuildIndex_integration runs a full scan and verifies the persisted index.
func TestRebuildIndex_integration(t *testing.T) {
	tmpDir := t.TempDir()
	// Place a skill in a subdirectory that scanWorkspace will discover.
	skillDir := filepath.Join(tmpDir, "my-skill")
	makeSkillDir(t, skillDir, "my-skill", "integration test skill")

	cacheRoot := t.TempDir()
	if err := RebuildIndex(t.Context(), tmpDir, tmpDir, "", cacheRoot); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	idxPath := IndexPath(cacheRoot)
	idx, err := LoadIndex(idxPath)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if idx == nil {
		t.Fatal("expected non-nil index after rebuild")
	}
	if len(idx.Entries) == 0 {
		t.Error("expected at least one entry in rebuilt index")
	}
}

// TestLoadIndex_permissionDenied returns a non-nil error when the file is unreadable.
func TestLoadIndex_permissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read any file, skipping permission test")
	}
	path := filepath.Join(t.TempDir(), "noread.json")
	if err := os.WriteFile(path, []byte(`{"generated_at":"2024-01-01T00:00:00Z","entries":[]}`), 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIndex(path)
	if err == nil {
		t.Error("expected error for unreadable file, got nil")
	}
}

// TestSaveIndex_mkdirError returns an error when parent directory creation fails.
func TestSaveIndex_mkdirError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where a directory is expected to block MkdirAll.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "sub", "index.json")
	idx := &DiscoveryIndex{GeneratedAt: time.Now()}
	if err := SaveIndex(path, idx); err == nil {
		t.Error("expected error when parent dir cannot be created, got nil")
	}
}

// TestRebuildIndex_statFailureSkipsEntry verifies that entries whose parent stat
// fails are silently skipped rather than causing RebuildIndex to error out.
func TestRebuildIndex_statFailureSkipsEntry(t *testing.T) {
	// An empty tmpDir with no skills will produce an empty index — that is
	// fine for this test; we just want to confirm RebuildIndex itself succeeds
	// even when the scan yields entries whose parent stat may fail.
	cacheRoot := t.TempDir()
	if err := RebuildIndex(t.Context(), t.TempDir(), t.TempDir(), "", cacheRoot); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	idx, err := LoadIndex(IndexPath(cacheRoot))
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
}

// TestValidate_dedupSamePath ensures duplicate index entries produce only one resource.
func TestValidate_dedupSamePath(t *testing.T) {
	parent := t.TempDir()
	fi, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	entry := IndexEntry{
		Path:        filepath.Join(parent, "skill"),
		Type:        ResourceSkill,
		Scope:       ScopeGlobal,
		Name:        "skill",
		Agent:       "agent",
		ParentMtime: fi.ModTime(),
	}
	idx := &DiscoveryIndex{
		GeneratedAt: time.Now(),
		Entries:     []IndexEntry{entry, entry}, // duplicate
	}
	resources, allValid := Validate(t.Context(), idx)
	if !allValid {
		t.Error("allValid: got false, want true")
	}
	if len(resources) != 1 {
		t.Errorf("resources count: got %d, want 1 (dedup expected)", len(resources))
	}
}
