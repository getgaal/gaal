//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// GlobalConfigFilePath returns the system-wide read-only config path on macOS.
func GlobalConfigFilePath() string {
	return globalConfigPathUnix
}

// userConfigDir returns the user config directory on macOS.
// Prefers XDG_CONFIG_HOME when set, otherwise falls back to ~/.config to match
// the conventions of other CLI tools (diverging from os.UserConfigDir which
// would return ~/Library/Application Support).
func userConfigDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv(envXDGConfigHome)); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultConfigHome), nil
}
