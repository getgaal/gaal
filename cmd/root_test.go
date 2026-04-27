package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
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

func TestSkipTelemetry(t *testing.T) {
	completionParent := &cobra.Command{Use: "completion"}

	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{
			name: "version",
			cmd:  &cobra.Command{Use: "version"},
			want: true,
		},
		{
			name: "schema",
			cmd:  &cobra.Command{Use: "schema"},
			want: true,
		},
		{
			name: "init",
			cmd:  &cobra.Command{Use: "init"},
			want: false,
		},
		{
			name: "sync",
			cmd:  &cobra.Command{Use: "sync"},
			want: false,
		},
		{
			name: "status",
			cmd:  &cobra.Command{Use: "status"},
			want: false,
		},
		{
			name: "subcommand of completion",
			cmd: func() *cobra.Command {
				child := &cobra.Command{Use: "bash"}
				completionParent.AddCommand(child)
				return child
			}(),
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipTelemetry(tc.cmd); got != tc.want {
				t.Errorf("skipTelemetry(%q) = %v, want %v", tc.cmd.Name(), got, tc.want)
			}
		})
	}
}
