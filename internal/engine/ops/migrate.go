package ops

import (
	"fmt"
	"net/url"

	"gaal/internal/config"
)

// MigrateResult holds the summary of what would be migrated.
type MigrateResult struct {
	Target       string
	URL          string
	DryRun       bool
	Repositories int
	Skills       int
	MCPs         int
}

// Migrate validates the configuration and prints a migration summary.
// Community Edition is not yet available, so this is a stub that reserves
// the CLI surface and validates readiness.
func Migrate(cfg *config.Config, target, rawURL string, dryRun bool) (*MigrateResult, error) {
	if target != "community" {
		return nil, fmt.Errorf("unknown migration target %q (supported: community)", target)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid URL %q: must include scheme and host (e.g. https://community.example.com)", rawURL)
	}

	return &MigrateResult{
		Target:       target,
		URL:          rawURL,
		DryRun:       dryRun,
		Repositories: len(cfg.Repositories),
		Skills:       len(cfg.Skills),
		MCPs:         len(cfg.MCPs),
	}, nil
}
