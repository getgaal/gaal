package engine

import (
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

//go:embed init_template.yaml
var initTemplate []byte

// Init writes the documented gaal.yaml skeleton to dest.
// When force is false and dest already exists, an error is returned so the
// caller can surface an actionable message without silently overwriting work.
func (e *Engine) Init(dest string, force bool) error {
	slog.Debug("init", "dest", dest, "force", force)

	if !force {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s already exists — use --force to overwrite", dest)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checking %s: %w", dest, err)
		}
	}

	if err := os.WriteFile(dest, initTemplate, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}

	slog.Info("config file created", "path", dest)
	return nil
}
