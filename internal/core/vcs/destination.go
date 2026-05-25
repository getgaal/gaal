package vcs

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NonEmptyDestinationError is returned by CheckEmptyDestination when a clone
// would write into a directory that already contains content. gaal refuses to
// proceed in that case because go-git's PlainClone will silently overwrite
// tracked files and leave untracked siblings exposed to checkout reset (see
// issue #217: live runtime state under ~/.claude/ was being wiped on sync).
type NonEmptyDestinationError struct {
	Path    string   // destination path
	Entries []string // names of entries found in the destination (may be truncated)
}

// maxListedEntries caps how many entry names the error message embeds, so a
// destination with hundreds of files does not produce an unreadable blob.
const maxListedEntries = 8

func (e *NonEmptyDestinationError) Error() string {
	names := e.Entries
	suffix := ""
	if len(names) > maxListedEntries {
		extra := len(names) - maxListedEntries
		names = names[:maxListedEntries]
		suffix = fmt.Sprintf(" (and %d more)", extra)
	}
	return fmt.Sprintf(
		"destination %s is not empty (contains: %s%s); "+
			"gaal refuses to clone into a directory with existing content to avoid "+
			"overwriting untracked files — move or delete the existing content, or "+
			"point `repositories:` at a different path",
		e.Path,
		strings.Join(names, ", "),
		suffix,
	)
}

// CheckEmptyDestination returns a *NonEmptyDestinationError when path exists
// and contains any entries (hidden files included). Returns nil when path
// does not exist or exists and is empty. Returns a plain error when path
// exists but is not a directory.
//
// Used by the repository Manager before delegating to a backend's Clone, so
// that all VCS types share a single refusal policy.
func CheckEmptyDestination(path string) error {
	slog.Debug("checking destination is empty", "path", shortPath(path))

	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspecting destination %s: %w", path, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("destination %s exists and is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("reading destination %s: %w", path, err)
	}
	if len(entries) == 0 {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return &NonEmptyDestinationError{Path: path, Entries: names}
}
