package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	configDirName  = "gaal"
	configFileName = "config.yaml"

	envXDGConfigHome  = "XDG_CONFIG_HOME"
	envProgramData    = "PROGRAMDATA"
	defaultConfigHome = ".config" // fallback when XDG_CONFIG_HOME is unset

	globalConfigPathUnix = "/etc/gaal/config.yaml" // Shared by Unix and Darwin
)

// ── OS-aware config path resolution ──────────────────────────────────────────

// Configuration file locations by priority (lowest to highest):
//
//  1. Global    — system-wide, set by a package manager
//                   Linux/macOS : /etc/gaal/config.yaml
//                   Windows     : %PROGRAMDATA%\gaal\config.yaml
//  2. User      — per-user customisation
//                   Linux       : $XDG_CONFIG_HOME/gaal/config.yaml  (~/.config/gaal/config.yaml)
//                   macOS       : $XDG_CONFIG_HOME/gaal/config.yaml  (~/.config/gaal/config.yaml)
//                   Windows     : %AppData%\gaal\config.yaml
//  3. Workspace — project-specific, value of the --config flag (default: gaal.yaml in CWD)

// GlobalConfigFilePath and userConfigDir are defined in the OS-specific
// build-constraint files:
//   platform_windows.go  (build: windows)
//   platform_darwin.go   (build: darwin)
//   platform_other.go    (build: !windows && !darwin)

// UserConfigFilePath is the exported accessor for the per-user config path.
// It is used by callers outside this package (e.g. the init wizard and
// telemetry) that need to resolve the per-user config destination before a
// Config is loaded.
func UserConfigFilePath() string {
	return userConfigFilePath()
}

// userConfigFilePath returns the per-user config path for the current OS.
// It respects XDG_CONFIG_HOME on Linux and macOS when set, otherwise ~/.config
// on macOS (see userConfigDir), and %AppData% on Windows.
func userConfigFilePath() string {
	slog.Debug("resolving user config file path")
	dir, err := userConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, defaultConfigHome)
	}
	return filepath.Join(dir, configDirName, configFileName)
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
