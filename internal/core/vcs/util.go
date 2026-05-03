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

// validateVCSOperand rejects user-controlled subprocess operands (URL, version
// pin) that begin with "-", which would be parsed as a CLI flag by hg/svn/bzr.
// Combined with the "--" end-of-options separator on the argv, this defends
// against flag-injection attacks like:
//
//	version: "--config=hooks.preupdate=touch /tmp/pwned"   (hg)
//	version: "--config-dir=/tmp/evil"                       (svn)
//
// kind is used for the error message ("url", "version", etc).
func validateVCSOperand(kind, value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s %q must not start with '-' (would be parsed as a flag)", kind, value)
	}
	return nil
}
