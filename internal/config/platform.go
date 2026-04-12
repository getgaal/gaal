package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// GlobalConfigFilePath returns the system-wide read-only config path for the current OS.
func GlobalConfigFilePath() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "gaal", "config.yaml")
	}
	// Linux and macOS both follow the /etc convention for system-wide config.
	return "/etc/gaal/config.yaml"
}

// userConfigDir returns the directory in which gaal stores per-user config.
// On macOS we intentionally diverge from os.UserConfigDir() (which would return
// ~/Library/Application Support) and prefer XDG_CONFIG_HOME when it is set,
// otherwise ~/.config to match the conventions of other CLI tools. Linux and
// Windows fall through to os.UserConfigDir().
func userConfigDir() (string, error) {
	if runtime.GOOS == "darwin" {
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return xdg, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config"), nil
	}
	return os.UserConfigDir()
}

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
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gaal", "config.yaml")
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
