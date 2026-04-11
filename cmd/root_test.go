package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApplyOptions_SandboxRedirectsUserDirs(t *testing.T) {
	originalSandboxDir := sandboxDir
	t.Cleanup(func() {
		sandboxDir = originalSandboxDir
	})

	sandboxDir = t.TempDir()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home-outside-sandbox"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config-outside-sandbox"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache-outside-sandbox"))
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "userprofile-outside-sandbox"))
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "appdata-outside-sandbox"))
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "localappdata-outside-sandbox"))

	opts, err := applyOptions()
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if got, want := opts.WorkDir, filepath.Join(sandboxDir, "workspace"); got != want {
		t.Fatalf("WorkDir = %q, want %q", got, want)
	}
	if got := os.Getenv("HOME"); got != sandboxDir {
		t.Fatalf("HOME = %q, want %q", got, sandboxDir)
	}

	if runtime.GOOS == "windows" {
		if got, want := os.Getenv("USERPROFILE"), sandboxDir; got != want {
			t.Fatalf("USERPROFILE = %q, want %q", got, want)
		}
		if got, want := os.Getenv("APPDATA"), filepath.Join(sandboxDir, "AppData", "Roaming"); got != want {
			t.Fatalf("APPDATA = %q, want %q", got, want)
		}
		if got, want := os.Getenv("LOCALAPPDATA"), filepath.Join(sandboxDir, "AppData", "Local"); got != want {
			t.Fatalf("LOCALAPPDATA = %q, want %q", got, want)
		}
		return
	}

	if got, want := os.Getenv("XDG_CONFIG_HOME"), filepath.Join(sandboxDir, ".config"); got != want {
		t.Fatalf("XDG_CONFIG_HOME = %q, want %q", got, want)
	}
	if got, want := os.Getenv("XDG_CACHE_HOME"), filepath.Join(sandboxDir, ".cache"); got != want {
		t.Fatalf("XDG_CACHE_HOME = %q, want %q", got, want)
	}
}
