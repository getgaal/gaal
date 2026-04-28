package discover

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile creates path with content, returning its os.FileInfo.
func writeFile(t *testing.T, path, content string) os.FileInfo {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

// TestSnapshotPath verifies the path construction helper.
func TestSnapshotPath(t *testing.T) {
	got := SnapshotPath("/state", "skill-abc")
	// Use filepath.Join so the expected value matches the OS separator on Windows.
	want := filepath.Join("/state", "skill-abc.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWorkdirKey verifies stability and uniqueness.
func TestWorkdirKey(t *testing.T) {
	k1 := WorkdirKey("/home/user/project")
	k2 := WorkdirKey("/home/user/project")
	k3 := WorkdirKey("/home/user/other")
	if k1 != k2 {
		t.Errorf("same input produced different keys: %q vs %q", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("different inputs produced same key: %q", k1)
	}
	if len(k1) != 8 {
		t.Errorf("expected 8-char hex key, got %q (len %d)", k1, len(k1))
	}
}

// TestLoad_notExist returns an empty snapshot for a missing file.
func TestLoad_notExist(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("expected empty snapshot, got %d entries", len(s))
	}
}

// TestLoad_valid round-trips a snapshot through Save then Load.
func TestLoad_valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	orig := Snapshot{
		"a/b.txt": {Size: 42, ModTime: time.Unix(1000, 0).UTC()},
	}
	if err := Save(path, orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
	rec, ok := loaded["a/b.txt"]
	if !ok {
		t.Fatal("expected key 'a/b.txt'")
	}
	if rec.Size != 42 {
		t.Errorf("size: got %d, want 42", rec.Size)
	}
}

// TestLoad_invalid returns an error for corrupted JSON.
func TestLoad_invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestSave_atomic verifies no leftover temp file after a successful Save.
func TestSave_atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	if err := Save(path, Snapshot{}); err != nil {
		t.Fatal(err)
	}
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file left behind after Save")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("snapshot file not created: %v", err)
	}
}

// TestRecord builds a correct FileRecord for a known file.
func TestRecord(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	writeFile(t, p, "hello")

	rec, err := Record(p)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Size != 5 {
		t.Errorf("size: got %d, want 5", rec.Size)
	}
	if rec.Hash == ([32]byte{}) {
		t.Error("hash should not be zero")
	}
	// Re-record the same file: must be identical.
	rec2, _ := Record(p)
	if rec.Hash != rec2.Hash {
		t.Error("same file produced different hashes")
	}
}

// TestRecord_missing returns an error for a non-existent file.
func TestRecord_missing(t *testing.T) {
	_, err := Record(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestHashFile is deterministic and differs for different content.
func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.txt")
	p2 := filepath.Join(dir, "b.txt")
	writeFile(t, p1, "hello")
	writeFile(t, p2, "world")

	h1, err := hashFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hashFile(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("different contents should produce different hashes")
	}
	h1b, _ := hashFile(p1)
	if h1 != h1b {
		t.Error("same file should produce stable hash")
	}
}

// TestSnapshotDir builds a snapshot from a directory tree.
func TestSnapshotDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "aaa")
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "bbb")

	s, err := SnapshotDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s))
	}
	if _, ok := s["a.txt"]; !ok {
		t.Error("missing a.txt")
	}
	if _, ok := s[filepath.Join("sub", "b.txt")]; !ok {
		t.Error("missing sub/b.txt")
	}
}

// TestDiffPath_ok: snapshot matches disk → no changes.
func TestDiffPath_ok(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f.txt"), "content")

	s, _ := SnapshotDir(dir)
	changes, err := DiffPath(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}

// TestDiffPath_modified: file content changed after snapshot.
func TestDiffPath_modified(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "original")

	s, _ := SnapshotDir(dir)

	if err := os.WriteFile(p, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := DiffPath(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Drift != DriftModified {
		t.Errorf("expected DriftModified, got %v", changes)
	}
}

// TestDiffPath_missing: file in snapshot but deleted from disk.
func TestDiffPath_missing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "hello")

	s, _ := SnapshotDir(dir)

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	changes, err := DiffPath(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Drift != DriftMissing {
		t.Errorf("expected DriftMissing, got %v", changes)
	}
}

// TestDiffPath_racyGit: same content but mtime differs — snapshot is repaired.
func TestDiffPath_racyGit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "stable")

	s, _ := SnapshotDir(dir)

	// Alter the recorded mtime to simulate a racy situation.
	old := s["f.txt"]
	old.ModTime = old.ModTime.Add(-5 * time.Second)
	s["f.txt"] = old

	changes, err := DiffPath(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes (racy-git fix), got %v", changes)
	}
	// Snapshot must have been repaired.
	fi, _ := os.Stat(p)
	if !s["f.txt"].ModTime.Equal(fi.ModTime()) {
		t.Error("snapshot mtime was not repaired after racy-git detection")
	}
}

// TestDiffPath_emptyRoot: non-existent root → no error, no changes.
func TestDiffPath_emptyRoot(t *testing.T) {
	changes, err := DiffPath(filepath.Join(t.TempDir(), "nope"), make(Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes for empty root, got %v", changes)
	}
}
