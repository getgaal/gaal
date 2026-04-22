package render

import (
	"os"
	"path/filepath"
	"strings"
)

// displaySkillName reduces a skill source (typically from ConfigSkill.Source)
// to a short, human-friendly label for the sync summary and plan output.
// Remote URLs and GitHub owner/repo shorthand pass through unchanged; local
// paths collapse to their last segment so deeply-nested plugin-cache
// directories stay readable.
func displaySkillName(source string) string {
	switch {
	case source == "":
		return "—"
	case strings.HasPrefix(source, "http://"),
		strings.HasPrefix(source, "https://"),
		strings.HasPrefix(source, "git@"),
		strings.HasPrefix(source, "ssh://"):
		return source
	case !strings.ContainsAny(source, `/\`):
		return source
	case strings.HasPrefix(source, "./"), strings.HasPrefix(source, `.\`),
		strings.HasPrefix(source, "../"), strings.HasPrefix(source, `..\`),
		strings.HasPrefix(source, "~/"), strings.HasPrefix(source, `~\`),
		strings.HasPrefix(source, "/"), filepath.IsAbs(source):
		return filepath.Base(source)
	}
	// owner/repo shorthand: exactly one slash, no prefixed path marker.
	if strings.Count(source, "/") == 1 {
		return source
	}
	return filepath.Base(source)
}

// displayTarget abbreviates a filesystem path for one-line rendering.
// Home-directory prefixes are replaced with "~"; paths longer than softLimit
// are truncated to "…/<last-three-segments>". softLimit <= 0 disables the
// length check and only applies the ~ substitution.
func displayTarget(path string, softLimit int) string {
	if path == "" {
		return path
	}

	abbreviated := path
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			abbreviated = "~"
		} else if strings.HasPrefix(path, home+string(os.PathSeparator)) {
			abbreviated = "~" + path[len(home):]
		}
	}

	if softLimit <= 0 || len(abbreviated) <= softLimit {
		return abbreviated
	}
	return truncatePath(abbreviated, 3)
}

// truncatePath keeps the last n path segments and prepends "…/" to indicate
// elision. If the path already has fewer than n segments, it is returned
// unchanged.
func truncatePath(p string, n int) string {
	sep := string(os.PathSeparator)
	parts := strings.Split(p, sep)
	// Drop leading empty parts produced by a leading separator.
	for len(parts) > 0 && parts[0] == "" {
		parts = parts[1:]
	}
	if len(parts) <= n {
		return p
	}
	return "…" + sep + strings.Join(parts[len(parts)-n:], sep)
}
