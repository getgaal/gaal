package platform

import (
	"log/slog"
	"os"
	"path/filepath"
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
//   windows.go  (build: windows)
//   darwin.go   (build: darwin)
//   unix.go    (build: !windows && !darwin)

// UserConfigFilePath is the exported accessor for the per-user config path.
// It is used by callers outside this package (e.g. the init wizard and
// telemetry) that need to resolve the per-user config destination before a
// Config is loaded.
func UserConfigFilePath() string {
	return userConfigFilePath()
}

// UserConfigDir is the exported accessor for the per-user config directory.
// It delegates to the OS-specific userConfigDir() and is the single source of
// truth for any code that needs to resolve the user config directory without
// also appending the config filename (e.g. registry.go).
func UserConfigDir() (string, error) {
	slog.Debug("resolving user config directory")
	return userConfigDir()
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
