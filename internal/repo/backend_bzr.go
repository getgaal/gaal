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

// VcsBazaar implements VCS for Bazaar repositories.
type VcsBazaar struct{}

func (b *VcsBazaar) Clone(ctx context.Context, url, path, version string) error {
	if err := requireBinary("bzr"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	slog.DebugContext(ctx, "branching", "url", url, "path", shortPath(path), "version", version)

	args := []string{"branch"}
	if version != "" {
		args = append(args, "-r", "tag:"+version)
	}
	args = append(args, url, path)

	return runner.Run(ctx, "branching "+shortPath(path), "", "bzr", args...)
}

func (b *VcsBazaar) Update(ctx context.Context, path, version string) error {
	if err := requireBinary("bzr"); err != nil {
		return err
	}
	slog.DebugContext(ctx, "pulling", "path", shortPath(path), "version", version)
	args := []string{"pull", "--overwrite"}
	if version != "" {
		args = append(args, "-r", "tag:"+version)
	}
	return runner.Run(ctx, "pulling "+shortPath(path), path, "bzr", args...)
}

func (b *VcsBazaar) IsCloned(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".bzr"))
	return err == nil
}

func (b *VcsBazaar) CurrentVersion(ctx context.Context, path string) (string, error) {
	if err := requireBinary("bzr"); err != nil {
		return "", err
	}
	out, err := cmdOutput(ctx, path, "bzr", "revno")
	return strings.TrimSpace(out), err
}
