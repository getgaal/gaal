// Package secfile writes files with mode 0o600 atomically (temp + rename)
// and tightens existing files that were created with looser permissions.
// Use this for any file that may hold secrets — MCP env tokens, telemetry
// state, log records, gaal config.
package secfile

import (
	"log/slog"
	"os"
	"path/filepath"
)

// Mode is the permission bits applied to all secret files.
const Mode os.FileMode = 0o600

// Write writes data to path with mode 0o600 **atomically**: bytes land in
// a sibling temp file, are fsynced, then renamed onto the final path. A
// crash, SIGINT, or full-disk during the write cannot leave a half-
// written file at the target — either the previous content survives or
// the new content is fully present (#120).
//
// On filesystems that don't support a same-dir CreateTemp (rare), Write
// falls back to a direct os.WriteFile + Chmod, with a warn log so the
// degraded mode is visible.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".gaal-tmp-*")
	if err != nil {
		slog.Warn("secfile: temp file creation failed; falling back to direct write",
			"path", path, "err", err)
		if werr := os.WriteFile(path, data, Mode); werr != nil {
			return werr
		}
		if cerr := os.Chmod(path, Mode); cerr != nil {
			slog.Warn("secfile: chmod failed", "path", path, "err", cerr)
		}
		return nil
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(Mode); err != nil {
		slog.Warn("secfile: chmod (temp) failed", "path", tmpPath, "err", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// OpenAppend opens path for append-write with mode 0o600, creating it if
// missing. If the file already exists with looser permissions, it is
// tightened. The caller owns the returned *os.File and must Close it.
func OpenAppend(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, Mode)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, Mode); err != nil {
		slog.Warn("secfile: chmod failed", "path", path, "err", err)
	}
	return f, nil
}
