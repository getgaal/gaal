//go:build !windows && !darwin

package config

import "os"

// GlobalConfigFilePath returns the system-wide read-only config path on Linux
// and other POSIX systems.
func GlobalConfigFilePath() string {
	return globalConfigPathUnix
}

// userConfigDir returns the user config directory, delegating to
// os.UserConfigDir() which on Linux honours $XDG_CONFIG_HOME and falls back
// to ~/.config.
func userConfigDir() (string, error) {
	return os.UserConfigDir()
}
