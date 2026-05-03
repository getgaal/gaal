package vcs

import (
	"context"
	"strings"
	"testing"
)

func TestValidateVCSOperand(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		value   string
		wantErr bool
	}{
		{"empty allowed", "version", "", false},
		{"normal tag", "version", "v1.2.3", false},
		{"branch with slash", "version", "release/2026", false},
		{"https url", "url", "https://github.com/owner/repo", false},
		{"ssh url", "url", "ssh://git@host:22/repo", false},
		{"flag injection version", "version", "--config=hooks.preupdate=touch /tmp/x", true},
		{"single dash flag", "version", "-r", true},
		{"flag injection url", "url", "--config-dir=/tmp/evil", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVCSOperand(tt.kind, tt.value)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.kind) {
				t.Errorf("error %q should mention kind %q", err.Error(), tt.kind)
			}
		})
	}
}

func TestRequireBinary_Found(t *testing.T) {
	if err := requireBinary("go"); err != nil {
		t.Fatalf("expected no error for 'go', got: %v", err)
	}
}

func TestRequireBinary_Missing(t *testing.T) {
	err := requireBinary("this-binary-does-not-exist-gaal-test")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

func TestShortPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two components", "/foo/bar", "foo/bar"},
		{"three components", "/a/b/c", "b/c"},
		{"single component", "file", "file"},
		{"two unix", "a/b", "a/b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortPath(tc.input)
			if got != tc.want {
				t.Errorf("shortPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCmdOutput_Success(t *testing.T) {
	ctx := context.Background()
	out, err := cmdOutput(ctx, t.TempDir(), "echo", "hello")
	if err != nil {
		t.Fatalf("cmdOutput echo: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestCmdOutput_MissingBinary(t *testing.T) {
	ctx := context.Background()
	_, err := cmdOutput(ctx, t.TempDir(), "this-binary-does-not-exist-gaal-test")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

// ---------------------------------------------------------------------------
// cmdOutput — non-zero exit and context cancellation
// ---------------------------------------------------------------------------

func TestCmdOutput_NonZeroExit(t *testing.T) {
	binDir := makeFakeBin(t, "fail-cmd", "exit 1")
	t.Setenv("PATH", binDir)
	_, err := cmdOutput(context.Background(), t.TempDir(), "fail-cmd")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestCmdOutput_ContextCancelled(t *testing.T) {
	binDir := makeFakeBin(t, "slow-cmd", "sleep 30")
	t.Setenv("PATH", binDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before starting
	_, err := cmdOutput(ctx, t.TempDir(), "slow-cmd")
	if err == nil {
		t.Fatal("expected error for pre-cancelled context")
	}
}
