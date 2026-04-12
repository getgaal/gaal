package config

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
// expandPaths (via Load to exercise the full path)
// ---------------------------------------------------------------------------

func TestExpandPaths_TildeInSkillSource(t *testing.T) {
	home, _ := os.UserHomeDir()

	p := writeYAML(t, `
skills:
  - source: ~/my-skills
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, "my-skills")
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != want {
		t.Errorf("got %v, want %q", cfg.Skills, want)
	}
}

func TestExpandPaths_GitHubShorthandUnchanged(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != "owner/repo" {
		t.Errorf("GitHub shorthand should not be expanded, got %q", cfg.Skills[0].Source)
	}
}

func TestExpandPaths_HTTPSUnchanged(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: https://github.com/owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) == 0 || cfg.Skills[0].Source != "https://github.com/owner/repo" {
		t.Errorf("HTTPS URL should remain unchanged, got %q", cfg.Skills[0].Source)
	}
}

func TestExpandPaths_MCPTargetRelative(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gaal.yaml")
	os.WriteFile(p, []byte(`
mcps:
  - name: myserver
    target: configs/mcp.json
    inline:
      command: npx
`), 0o644)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(dir, "configs", "mcp.json")
	if len(cfg.MCPs) == 0 || cfg.MCPs[0].Target != want {
		t.Errorf("got %q, want %q", cfg.MCPs[0].Target, want)
	}
}

// ---------------------------------------------------------------------------
// isRemoteURL / isGitHubShorthand (unit tests on the helpers directly)
// ---------------------------------------------------------------------------

func TestIsRemoteURL(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://github.com/owner/repo", true},
		{"http://example.com/repo.git", true},
		{"git@github.com:owner/repo.git", true},
		{"ssh://user@host/repo.git", true},
		{"owner/repo", false},
		{"./local", false},
		{"~/local", false},
		{"/abs/path", false},
	}
	for _, tc := range cases {
		if got := isRemoteURL(tc.input); got != tc.want {
			t.Errorf("isRemoteURL(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsGitHubShorthand(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"owner/repo", true},
		{"my-org/my-tool", true},
		{"https://github.com/owner/repo", false},
		{"git@github.com:owner/repo.git", false},
		{"./local/path", false},
		{"../up/path", false},
		{"~/home/path", false},
		{"/abs/path", false},
		{"no-slash", false},
		{"too/many/parts", false},
	}
	for _, tc := range cases {
		if got := isGitHubShorthand(tc.input); got != tc.want {
			t.Errorf("isGitHubShorthand(%q) = %v, want %v", tc.input, got, tc.want)
		}
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
