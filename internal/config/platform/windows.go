//go:build windows

package platform

import (
	"os"
	"path/filepath"
)

const defaultProgramData = `C:\ProgramData`

// GlobalConfigFilePath returns the system-wide read-only config path on Windows.
func GlobalConfigFilePath() string {
	pd := os.Getenv(envProgramData)
	if pd == "" {
		pd = defaultProgramData
	}
	return filepath.Join(pd, configDirName, configFileName)
}

// userConfigDir returns the user config directory on Windows (%AppData%).
func userConfigDir() (string, error) {
	return os.UserConfigDir()
}
