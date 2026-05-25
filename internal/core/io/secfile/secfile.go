// Package secfile writes files with mode 0o600 and tightens existing files
// that were created with looser permissions. Use this for any file that may
// hold secrets — MCP env tokens, telemetry state, log records, gaal config.
package secfile

import (
	"log/slog"
	"os"
)

// Mode is the permission bits applied to all secret files.
const Mode os.FileMode = 0o600

// Write writes data to path with mode 0o600. If path already exists with
// looser permissions, it is tightened via Chmod after the write — os.WriteFile
// only honours the mode argument on file creation, so an existing 0o644 file
// would otherwise stay world-readable.
func Write(path string, data []byte) error {
	if err := os.WriteFile(path, data, Mode); err != nil {
		return err
	}
	if err := os.Chmod(path, Mode); err != nil {
		slog.Warn("secfile: chmod failed", "path", path, "err", err)
	}
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
