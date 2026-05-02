package render

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// RenderSyncBrief writes a compact one-shot sync summary to w. It shows
// change lines derived from the plan (cloned repos, installed/updated skills,
// added/updated MCP configs), followed by a per-agent rollup from the
// post-sync status, and a final duration line.
//
// Example with changes:
//
//	Cloned src/example.
//	Installed 2 new skills.
//	Updated memory-mcp config.
//	→ claude-code: 2 skills, 1 MCP server
//	→ cursor: 2 skills, 1 MCP server
//	✓ Synced in 1.2s.
//
// Example with no changes:
//
//	→ claude-code: 2 skills, 1 MCP server
//	✓ In sync (500ms).
func RenderSyncBrief(w io.Writer, plan *PlanReport, status *StatusReport, duration time.Duration) error {
	if plan == nil {
		plan = &PlanReport{}
	}
	if status == nil {
		status = &StatusReport{}
	}

	// Change lines from plan
	changes := collectChangeLines(plan)
	for _, line := range changes {
		fmt.Fprintln(w, line)
	}

	// Per-agent rollup from post-sync status
	rollup := buildAgentRollup(status)
	for _, line := range rollup {
		fmt.Fprintln(w, line)
	}

	// Duration line
	d := fmtDuration(duration)
	if plan.HasChanges {
		fmt.Fprintf(w, "✓ Synced in %s.\n", d)
	} else {
		fmt.Fprintf(w, "✓ In sync (%s).\n", d)
	}

	return nil
}

// collectChangeLines builds the list of human-readable change lines from the
// plan. Each class of change is represented by one aggregated line.
func collectChangeLines(plan *PlanReport) []string {
	var lines []string

	// Cloned repos (PlanClone actions)
	for _, r := range plan.Repositories {
		if r.Action == PlanClone {
			lines = append(lines, fmt.Sprintf("Cloned %s.", r.Path))
		}
	}

	// Updated repos (PlanUpdate actions)
	for _, r := range plan.Repositories {
		if r.Action == PlanUpdate {
			lines = append(lines, fmt.Sprintf("Updated %s.", r.Path))
		}
	}

	// Installed skills (aggregate count across all PlanCreate entries)
	installCount := 0
	for _, e := range plan.Skills {
		if e.Action == PlanCreate {
			installCount += len(e.Install)
		}
	}
	if installCount > 0 {
		lines = append(lines, fmt.Sprintf("Installed %s.", pluralise(installCount, "new skill", "new skills")))
	}

	// Updated skills (aggregate count across PlanUpdate entries)
	updateSkillCount := 0
	for _, e := range plan.Skills {
		if e.Action == PlanUpdate {
			updateSkillCount += len(e.Update)
		}
	}
	if updateSkillCount > 0 {
		lines = append(lines, fmt.Sprintf("Updated %s.", pluralise(updateSkillCount, "skill", "skills")))
	}

	// Added MCP configs (PlanCreate)
	for _, m := range plan.MCPs {
		if m.Action == PlanCreate {
			lines = append(lines, fmt.Sprintf("Added %s config.", m.Name))
		}
	}

	// Updated MCP configs (PlanUpdate)
	for _, m := range plan.MCPs {
		if m.Action == PlanUpdate {
			lines = append(lines, fmt.Sprintf("Updated %s config.", m.Name))
		}
	}

	return lines
}

// agentRollupEntry holds the per-agent counts for the rollup display.
type agentRollupEntry struct {
	name       string
	skillCount int
	mcpCount   int
}

// buildAgentRollup produces one "→ agent: N skills, M MCP servers" line per
// installed agent, ordered by agent name. Unmanaged skills are excluded.
func buildAgentRollup(status *StatusReport) []string {
	// Count skills per agent from status.Skills (exclude unmanaged).
	skillsByAgent := map[string]int{}
	for _, e := range status.Skills {
		if e.Status == StatusUnmanaged {
			continue
		}
		skillsByAgent[e.Agent] += len(e.Installed)
	}

	// Count managed MCPs (exclude unmanaged).
	mcpCount := 0
	for _, m := range status.MCPs {
		if m.Status != StatusUnmanaged {
			mcpCount++
		}
	}

	// Build rollup per installed agent.
	var entries []agentRollupEntry
	for _, a := range status.Agents {
		if !a.Installed {
			continue
		}
		entries = append(entries, agentRollupEntry{
			name:       a.Name,
			skillCount: skillsByAgent[a.Name],
			mcpCount:   mcpCount,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		parts := []string{
			pluralise(e.skillCount, "skill", "skills"),
		}
		if e.mcpCount > 0 {
			parts = append(parts, pluralise(e.mcpCount, "MCP server", "MCP servers"))
		}
		lines = append(lines, fmt.Sprintf("→ %s: %s", e.name, strings.Join(parts, ", ")))
	}
	return lines
}

// fmtDuration formats a duration for the brief sync output. Sub-second
// durations use milliseconds; >= 1 second uses one decimal place in seconds.
func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
