package vcs

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// requireBinary returns a clear error if the named executable is not found
// in PATH, so the user knows exactly which tool to install.
func requireBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("binary %q not found in PATH: install it to use the %s backend", name, name)
	}
	return nil
}

// cmdOutput runs a command and returns its captured stdout (no spinner — query only).
func cmdOutput(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// shortPath returns the last two path components for display.
func shortPath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) <= 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
