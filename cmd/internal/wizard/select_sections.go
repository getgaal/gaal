package wizard

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pterm/pterm"

	"gaal/internal/engine/ops"
)

// ErrCancelled is returned by SelectSections when the user aborts the prompt.
var ErrCancelled = errors.New("wizard cancelled")

// SelectSections presents the candidates as a flat pterm interactive
// multiselect. Section grouping is preserved visually by prefixing every
// option with its owning agent name, and by printing a static "info header"
// above the prompt that lists detected agents (plus delegators for generic).
//
// Everything is preselected by default so the user can confirm with Enter.
func SelectSections(candidates ops.Candidates, scope ops.Scope) (ops.Plan, error) {
	// Print a compact info header above the prompt.
	printHeader(candidates)

	options, byLabel := buildOptionList(candidates)
	if len(options) == 0 {
		return ops.Plan{}, nil
	}

	selected, err := pterm.DefaultInteractiveMultiselect.
		WithOptions(options).
		WithDefaultOptions(options).
		WithFilter(false).
		WithDefaultText("Select the agent configuration to import").
		WithCheckmark(&pterm.Checkmark{Checked: pterm.Green("✓"), Unchecked: " "}).
		Show()
	if err != nil {
		return ops.Plan{}, fmt.Errorf("multiselect: %w", err)
	}

	if len(selected) == 0 {
		return ops.Plan{}, ErrCancelled
	}

	chosen := make([]ops.Candidate, 0, len(selected))
	for _, label := range selected {
		if c, ok := byLabel[label]; ok {
			chosen = append(chosen, c)
		}
	}
	return ops.BuildPlan(chosen, scope), nil
}

// printHeader renders a compact summary of what was detected: one line per
// agent with a counter. It is purely informational and never accepts input.
func printHeader(c ops.Candidates) {
	pterm.DefaultBasicText.Println()
	pterm.DefaultBasicText.Println(pterm.Bold.Sprint("Detected configuration"))
	for _, sec := range c.Sections {
		line := fmt.Sprintf("  %s  %s",
			pterm.Bold.Sprint(sec.AgentName),
			pterm.Gray(fmt.Sprintf("(%d skill%s, %d mcp%s)",
				len(sec.Skills), plural(len(sec.Skills)),
				len(sec.MCPs), plural(len(sec.MCPs)))))
		pterm.DefaultBasicText.Println(line)
		if len(sec.GenericDelegators) > 0 {
			pterm.DefaultBasicText.Println("    " +
				pterm.Gray("delegated by: "+strings.Join(sec.GenericDelegators, ", ")))
		}
	}
	pterm.DefaultBasicText.Println()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// buildOptionList returns the ordered option labels for the multiselect and
// a lookup table mapping each label back to its Candidate.
//
// Labels are made unique by prefixing them with "<agent> · " and suffixing
// with a fingerprint when needed, so pterm can round-trip selection → label.
func buildOptionList(c ops.Candidates) ([]string, map[string]ops.Candidate) {
	options := []string{}
	byLabel := map[string]ops.Candidate{}

	add := func(base string, cand ops.Candidate) {
		label := base
		// Guarantee uniqueness even if two candidates share everything.
		for suffix := 2; ; suffix++ {
			if _, clash := byLabel[label]; !clash {
				break
			}
			label = fmt.Sprintf("%s (#%d)", base, suffix)
		}
		options = append(options, label)
		byLabel[label] = cand
	}

	for _, sec := range c.Sections {
		for _, s := range sec.Skills {
			base := fmt.Sprintf("%s · skill %s", sec.AgentName, s.SkillName)
			add(base, s)
		}
		for _, m := range sec.MCPs {
			base := fmt.Sprintf("%s · mcp   %s", sec.AgentName, m.MCPName)
			add(base, m)
		}
	}
	return options, byLabel
}
