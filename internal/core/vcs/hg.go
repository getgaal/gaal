package vcs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/runner"
)

// VcsMercurial implements VCS for Mercurial repositories.
type VcsMercurial struct{}

func (m *VcsMercurial) Clone(ctx context.Context, url, path, version string) error {
	if err := requireBinary("hg"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	slog.DebugContext(ctx, "cloning", "url", url, "path", shortPath(path), "version", version)

	args := []string{"clone"}
	if version != "" {
		args = append(args, "-r", version)
	}
	args = append(args, url, path)

	return runner.Run(ctx, "cloning "+shortPath(path), "", "hg", args...)
}

func (m *VcsMercurial) Update(ctx context.Context, path, version string) error {
	if err := requireBinary("hg"); err != nil {
		return err
	}
	slog.DebugContext(ctx, "pulling", "path", shortPath(path))
	if err := runner.Run(ctx, "pulling "+shortPath(path), path, "hg", "pull"); err != nil {
		return err
	}

	args := []string{"update"}
	if version != "" {
		args = append(args, "-r", version)
	}

	return runner.Run(ctx, "updating "+shortPath(path), path, "hg", args...)
}

func (m *VcsMercurial) IsCloned(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".hg"))
	return err == nil
}

func (m *VcsMercurial) CurrentVersion(ctx context.Context, path string) (string, error) {
	if err := requireBinary("hg"); err != nil {
		return "", err
	}
	out, err := cmdOutput(ctx, path, "hg", "id", "-i")
	return strings.TrimSpace(out), err
}

// HasChanges reports whether the working directory has local modifications.
func (m *VcsMercurial) HasChanges(ctx context.Context, path string) (bool, error) {
	if err := requireBinary("hg"); err != nil {
		return false, err
	}
	slog.DebugContext(ctx, "checking for local changes", "path", shortPath(path))
	out, err := cmdOutput(ctx, path, "hg", "status", "-mard")
	if err != nil {
		return false, fmt.Errorf("hg status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}
