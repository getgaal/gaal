package config

import (
	"os"
	"path/filepath"
	"strings"
)

// indexOf returns the index of the first element in items for which match
// returns true, or -1 if none is found.
func indexOf[T any](items []T, match func(T) bool) int {
	for i, item := range items {
		if match(item) {
			return i
		}
	}
	return -1
}

// deduplicate returns a copy of items with duplicate entries removed, keeping
// the first occurrence. key extracts the deduplication key from each element.
func deduplicate[T any](items []T, key func(T) string) []T {
	seen := make(map[string]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, item := range items {
		k := key(item)
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

// ── Cross-platform path expansion ────────────────────────────────────────────

// isRemoteURL reports whether s is a remote URL (http, https, git@, ssh).
func isRemoteURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "ssh://")
}

// isGitHubShorthand reports whether s is a GitHub owner/repo shorthand
// (exactly one forward-slash, no scheme, not a local path).
func isGitHubShorthand(s string) bool {
	if isRemoteURL(s) ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, `.\`) ||
		strings.HasPrefix(s, "../") || strings.HasPrefix(s, `..\`) ||
		strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) ||
		strings.HasPrefix(s, "/") || filepath.IsAbs(s) {
		return false
	}
	return len(strings.Split(s, "/")) == 2
}

// expandPaths expands ~ and relative paths in c, while leaving remote URLs and
// GitHub shorthands (owner/repo) untouched.
func (c *Config) expandPaths(baseDir string) {
	home, _ := os.UserHomeDir()

	expandPath := func(p string) string {
		// Accept both ~/ (POSIX) and ~\ (Windows) as home-relative prefixes.
		if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			return filepath.Join(home, p[2:])
		}
		// filepath.IsAbs("/posix/path") returns false on Windows;
		// handle POSIX-style absolute paths explicitly so cross-platform
		// config files (e.g. written on Linux, used on Windows) are preserved.
		if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
			return p
		}
		return filepath.Join(baseDir, p)
	}

	expanded := make(map[string]ConfigRepo, len(c.Repositories))
	for path, repo := range c.Repositories {
		expanded[expandPath(path)] = repo
	}
	c.Repositories = expanded

	for i := range c.Skills {
		src := c.Skills[i].Source
		if !isRemoteURL(src) && !isGitHubShorthand(src) {
			c.Skills[i].Source = expandPath(src)
		}
	}

	for i := range c.MCPs {
		c.MCPs[i].Target = expandPath(c.MCPs[i].Target)
	}
}
