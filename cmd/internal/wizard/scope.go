// Package wizard hosts the interactive TUI helpers used by the gaal init
// command. Nothing in this package owns business logic: every piece of data
// it renders comes from internal/engine/ops.
package wizard

import (
	"fmt"

	"github.com/pterm/pterm"

	"gaal/internal/engine/ops"
)

// Mode is the high-level init strategy chosen by the user.
type Mode string

const (
	// ModeEmpty writes the documented empty template.
	ModeEmpty Mode = "empty"
	// ModeImport runs the audit-driven selection wizard.
	ModeImport Mode = "import"
)

// SelectMode prompts the user to choose between starting from an empty
// template or importing detected skills and MCP servers.
func SelectMode() (Mode, error) {
	emptyLabel := "empty — start from a documented skeleton"
	importLabel := "import — select from skills and MCP servers detected on this machine"

	choice, err := pterm.DefaultInteractiveSelect.
		WithOptions([]string{importLabel, emptyLabel}).
		WithDefaultText("How would you like to create gaal.yaml?").
		Show()
	if err != nil {
		return "", fmt.Errorf("mode prompt: %w", err)
	}
	if choice == emptyLabel {
		return ModeEmpty, nil
	}
	return ModeImport, nil
}

// SelectScope prompts the user to choose between a project-scoped and a
// global-scoped gaal configuration. The returned scope is guaranteed to be
// one of ops.ScopeProject / ops.ScopeGlobal.
func SelectScope(projectPath, globalPath string) (ops.Scope, error) {
	projectLabel := fmt.Sprintf("project — writes %s", projectPath)
	globalLabel := fmt.Sprintf("global  — writes %s", globalPath)

	choice, err := pterm.DefaultInteractiveSelect.
		WithOptions([]string{projectLabel, globalLabel}).
		WithDefaultText("Where should this configuration apply?").
		Show()
	if err != nil {
		return "", fmt.Errorf("scope prompt: %w", err)
	}

	if choice == projectLabel {
		return ops.ScopeProject, nil
	}
	return ops.ScopeGlobal, nil
}
