package cmd

import (
	"testing"
)

func TestRunSync_DryRunAndServiceConflict(t *testing.T) {
	// Save and restore global flags.
	origDryRun := dryRun
	origService := service
	origCfgFile := cfgFile
	t.Cleanup(func() {
		dryRun = origDryRun
		service = origService
		cfgFile = origCfgFile
	})

	dryRun = true
	service = true
	cfgFile = "testdata/nonexistent.yaml"

	err := runSync(nil, nil)
	if err == nil {
		t.Fatal("expected error when --dry-run and --service are both set")
	}
	want := "--dry-run and --service are incompatible"
	if got := err.Error(); got != want+": a dry-run service loop is meaningless" {
		t.Fatalf("unexpected error message: %q", got)
	}
}
