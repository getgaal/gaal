package discover

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// FileRecord stores per-file metadata for the fast-path drift check.
// It mirrors Git's index format: size and mtime provide an O(1) "definitely
// unchanged" decision; the SHA-256 hash is only computed when stat data differ.
type FileRecord struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
	Hash    [32]byte  `json:"hash"`
}

// Snapshot maps root-relative file paths to their last-recorded FileRecord.
// It is persisted as JSON in the gaal state directory after every successful sync.
type Snapshot map[string]FileRecord

// Change is a single file-level drift event returned by DiffPath.
type Change struct {
	Path  string
	Drift DriftState
}

// SnapshotPath returns the conventional path for a snapshot file identified by
// key under stateDir (e.g. stateDir/skill-abc123.json).
func SnapshotPath(stateDir, key string) string {
	return filepath.Join(stateDir, key+".json")
}

// WorkdirKey returns a short, stable, collision-resistant identifier derived
// from an absolute path, suitable for use as part of a snapshot filename.
func WorkdirKey(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h[:4])
}

// Load reads a Snapshot from path. Returns an empty (non-nil) Snapshot when
// the file does not exist — the normal first-run state.
func Load(path string) (Snapshot, error) {
	slog.Debug("loading snapshot", "path", path)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(Snapshot), nil
	}
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// Save writes s to path atomically via a temporary file and os.Rename.
// Parent directories are created if they do not exist.
func Save(path string, s Snapshot) error {
	slog.Debug("saving snapshot", "path", path, "entries", len(s))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Record builds a FileRecord for the file at path by reading its stat and
// computing its SHA-256 hash. Call this during sync to capture the
// post-install state of each managed file.
func Record(path string) (FileRecord, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileRecord{}, err
	}
	h, err := hashFile(path)
	if err != nil {
		return FileRecord{}, err
	}
	return FileRecord{Size: fi.Size(), ModTime: fi.ModTime(), Hash: h}, nil
}

// SnapshotDir walks root and builds a fresh Snapshot from every file it finds.
// This is the canonical way to snapshot a directory after a successful sync.
func SnapshotDir(root string) (Snapshot, error) {
	slog.Debug("snapshotting directory", "root", root)
	s := make(Snapshot)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rec, err := Record(path)
		if err != nil {
			slog.Debug("snapshot record error", "path", path, "err", err)
			return nil
		}
		s[rel] = rec
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// DiffPath compares every file under root against snapshot s and returns the
// list of files that have drifted.
//
// Algorithm (Git index-inspired):
//  1. stat(file): if size and mtime match the FileRecord → DriftOK (no read).
//  2. If stat differs → compute SHA-256: if hash matches, update mtime
//     in-place (racy-git repair) and return DriftOK.
//  3. If hash differs → DriftModified.
//  4. Files in snapshot but absent on disk → DriftMissing.
//
// Files found on disk but absent from the snapshot are silently skipped;
// the caller classifies them (e.g. DriftUnmanaged) using higher-level logic.
func DiffPath(root string, s Snapshot) ([]Change, error) {
	slog.Debug("diffing path against snapshot", "root", root, "entries", len(s))
	seen := make(map[string]struct{}, len(s))
	var changes []Change

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Debug("walk error", "path", path, "err", err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		seen[rel] = struct{}{}

		rec, known := s[rel]
		if !known {
			return nil // not tracked by this snapshot
		}

		fi, err := d.Info()
		if err != nil {
			changes = append(changes, Change{Path: rel, Drift: DriftUnknown})
			return nil
		}

		// Fast path: stat matches → assume unchanged.
		if fi.Size() == rec.Size && fi.ModTime().Equal(rec.ModTime) {
			return nil
		}

		// Stat differs → verify via hash.
		h, err := hashFile(path)
		if err != nil {
			changes = append(changes, Change{Path: rel, Drift: DriftUnknown})
			return nil
		}
		if h == rec.Hash {
			// Racy-git scenario: content unchanged but mtime updated. Repair.
			s[rel] = FileRecord{Size: fi.Size(), ModTime: fi.ModTime(), Hash: h}
			return nil
		}

		changes = append(changes, Change{Path: rel, Drift: DriftModified})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Files expected by snapshot but missing from disk.
	for rel := range s {
		if _, ok := seen[rel]; !ok {
			changes = append(changes, Change{Path: rel, Drift: DriftMissing})
		}
	}

	return changes, nil
}

// hashFile computes the SHA-256 checksum of the file at path.
func hashFile(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}
