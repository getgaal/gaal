package config

import (
	"fmt"
	"log/slog"
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

// validateRepositoryContainment rejects repository keys that escape the
// workspace root via an absolute path, "..", or "~/" prefix. This prevents a
// shared / public gaal.yaml from clobbering arbitrary user-writable paths
// (e.g. ~/.ssh) on first sync. Only applied to project-scope configs
// (LoadStrict); global/user configs legitimately use absolute paths.
//
// Set GAAL_ALLOW_ABSOLUTE_PATHS=1 to bypass when needed.
func (c *Config) validateRepositoryContainment() error {
	if os.Getenv("GAAL_ALLOW_ABSOLUTE_PATHS") == "1" {
		return nil
	}
	for key := range c.Repositories {
		if err := checkRepoPathContained(key); err != nil {
			return err
		}
	}
	return nil
}

// checkRepoPathContained rejects keys that look like an attempt to escape the
// workspace dir. Returns nil on safe keys.
func checkRepoPathContained(key string) error {
	if filepath.IsAbs(key) || strings.HasPrefix(key, "/") {
		return fmt.Errorf("repository path %q is absolute; refusing to clone outside the workspace (set GAAL_ALLOW_ABSOLUTE_PATHS=1 to bypass)", key)
	}
	if strings.HasPrefix(key, "~/") || strings.HasPrefix(key, `~\`) {
		return fmt.Errorf("repository path %q starts with ~ (home directory); refusing to clone outside the workspace (set GAAL_ALLOW_ABSOLUTE_PATHS=1 to bypass)", key)
	}
	cleaned := filepath.ToSlash(filepath.Clean(key))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("repository path %q escapes the workspace via '..'; refusing (set GAAL_ALLOW_ABSOLUTE_PATHS=1 to bypass)", key)
	}
	return nil
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
	// Track which raw key produced each expanded path so a collision
	// surfaces a useful warning instead of silently overwriting one of
	// the entries (#136).
	rawKeyFor := make(map[string]string, len(c.Repositories))
	for path, repo := range c.Repositories {
		exp := expandPath(path)
		if prev, dup := rawKeyFor[exp]; dup {
			slog.Warn("repository entries collide after path expansion; one will be lost",
				"expanded", exp, "first_key", prev, "duplicate_key", path)
		}
		expanded[exp] = repo
		rawKeyFor[exp] = path
	}
	c.Repositories = expanded

	for i := range c.Skills {
		src := c.Skills[i].Source
		if !isRemoteURL(src) && !isGitHubShorthand(src) {
			c.Skills[i].Source = expandPath(src)
		}
	}

	for i := range c.MCPs {
		if c.MCPs[i].Target == "" {
			continue
		}
		c.MCPs[i].Target = expandPath(c.MCPs[i].Target)
	}
}
