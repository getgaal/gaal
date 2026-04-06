package repo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gaal/internal/runner"
)

// VcsSVN implements VCS for Subversion repositories.
type VcsSVN struct{}

func (s *VcsSVN) Clone(ctx context.Context, url, path, version string) error {
	if err := requireBinary("svn"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	slog.DebugContext(ctx, "checkout", "url", url, "path", shortPath(path), "version", version)

	args := []string{"checkout"}
	if version != "" {
		args = append(args, "-r", version)
	}
	args = append(args, url, path)

	return runner.Run(ctx, "checkout "+shortPath(path), "", "svn", args...)
}

func (s *VcsSVN) Update(ctx context.Context, path, version string) error {
	if err := requireBinary("svn"); err != nil {
		return err
	}
	slog.DebugContext(ctx, "updating", "path", shortPath(path), "version", version)
	args := []string{"update"}
	if version != "" {
		args = append(args, "-r", version)
	}
	return runner.Run(ctx, "updating "+shortPath(path), path, "svn", args...)
}

func (s *VcsSVN) IsCloned(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".svn"))
	return err == nil
}

func (s *VcsSVN) CurrentVersion(ctx context.Context, path string) (string, error) {
	if err := requireBinary("svnversion"); err != nil {
		return "", err
	}
	out, err := cmdOutput(ctx, path, "svnversion", ".")
	return strings.TrimSpace(out), err
}
