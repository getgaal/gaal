package vcs

import (
	"context"
	"testing"
)

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
