package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GlobalConfigFilePath / userConfigFilePath
// ---------------------------------------------------------------------------

func TestGlobalConfigFilePath_Linux(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("POSIX-only test")
	}
	got := GlobalConfigFilePath()
	if got != "/etc/gaal/config.yaml" {
		t.Errorf("got %q, want /etc/gaal/config.yaml", got)
	}
}

func TestGlobalConfigFilePath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	got := GlobalConfigFilePath()
	if !strings.HasSuffix(got, `\gaal\config.yaml`) {
		t.Errorf("got %q, want suffix \\gaal\\config.yaml", got)
	}
}

func TestUserConfigFilePath(t *testing.T) {
	got := userConfigFilePath()
	if got == "" {
		t.Fatal("userConfigFilePath returned empty string")
	}
	if !strings.HasSuffix(got, filepath.Join("gaal", "config.yaml")) {
		t.Errorf("got %q, expected suffix gaal/config.yaml", got)
	}
}

func TestUserConfigFilePath_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got := userConfigFilePath()
	want := filepath.Join(home, ".config", "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUserConfigFilePath_Exported(t *testing.T) {
	got := UserConfigFilePath()
	want := userConfigFilePath()
	if got != want {
		t.Errorf("exported UserConfigFilePath returned %q, want %q", got, want)
	}
}

func TestUserConfigFilePath_DarwinUsesXDGConfigHome(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got := userConfigFilePath()
	want := filepath.Join(xdg, "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// userConfigFilePath — fallback when HOME is invalid
// ---------------------------------------------------------------------------

func TestUserConfigFilePath_FallbackWhenHomeBroken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME env var does not control os.UserConfigDir on Windows")
	}
	// Unset both HOME and XDG_CONFIG_HOME; os.UserConfigDir will fail on Linux
	// but the function should still return a non-empty path via the fallback.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got := userConfigFilePath()
	// The fallback uses an empty home so the path will look odd, but it must
	// not be empty and must end with the expected suffix.
	if got == "" {
		t.Fatal("userConfigFilePath returned empty string even with broken HOME")
	}
	if !strings.HasSuffix(got, filepath.Join("gaal", "config.yaml")) {
		t.Errorf("got %q, expected suffix gaal/config.yaml", got)
	}
}

// ---------------------------------------------------------------------------
// UserConfigFilePath (exported) — platform-specific behaviour
// ---------------------------------------------------------------------------

func TestUserConfigFilePath_LinuxUsesXDGConfigHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got := UserConfigFilePath()
	want := filepath.Join(xdg, "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUserConfigFilePath_LinuxDefaultsToConfigDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got := UserConfigFilePath()
	want := filepath.Join(home, ".config", "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUserConfigFilePath_WindowsUsesAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	got := UserConfigFilePath()
	if got == "" {
		t.Fatal("UserConfigFilePath returned empty string on Windows")
	}
	if !strings.HasSuffix(got, filepath.Join("gaal", "config.yaml")) {
		t.Errorf("got %q, expected suffix gaal\\config.yaml", got)
	}
	// os.UserConfigDir on Windows returns %AppData% — verify the path sits under it.
	appData := os.Getenv("APPDATA")
	if appData != "" && !strings.HasPrefix(got, appData) {
		t.Errorf("got %q, expected path under APPDATA=%q", got, appData)
	}
}

func TestUserConfigFilePath_DarwinExported_UsesXDG(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	xdg := filepath.Join(t.TempDir(), "xdg-config")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got := UserConfigFilePath()
	want := filepath.Join(xdg, "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUserConfigFilePath_DarwinExported_NoXDG(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	got := UserConfigFilePath()
	want := filepath.Join(home, ".config", "gaal", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
