package render

import (
	"fmt"
	"io"
)

// summaryRenderer implements Renderer producing a compact count-based summary.
type summaryRenderer struct{}

func (sr *summaryRenderer) Render(w io.Writer, r *StatusReport) error {
	repoCounts := repoSummaryCounts(r.Repositories)
	skillCounts := skillSummaryCounts(r.Skills)
	mcpCounts := mcpSummaryCounts(r.MCPs)

	installedAgents := 0
	for _, a := range r.Agents {
		if a.Installed {
			installedAgents++
		}
	}

	hasRepos := len(r.Repositories) > 0
	hasSkills := skillCounts.total > 0
	hasMCPs := mcpCounts.total > 0
	hasAgents := len(r.Agents) > 0

	if !hasRepos && !hasSkills && !hasMCPs && !hasAgents {
		fmt.Fprintln(w, "no managed resources")
		return nil
	}

	drift := 0

	if hasRepos {
		icon := "✓"
		dirtyNote := ""
		if repoCounts.dirty > 0 {
			dirtyNote = fmt.Sprintf(" (%d dirty)", repoCounts.dirty)
		}
		if repoCounts.cloned < repoCounts.total {
			icon = "!"
			drift += repoCounts.total - repoCounts.cloned
		}
		fmt.Fprintf(w, "%s %d/%d repos cloned%s\n", icon, repoCounts.cloned, repoCounts.total, dirtyNote)
	}

	if hasSkills {
		icon := "✓"
		if skillCounts.installed < skillCounts.total {
			icon = "!"
			drift += skillCounts.total - skillCounts.installed
		}
		fmt.Fprintf(w, "%s %d/%d skills installed\n", icon, skillCounts.installed, skillCounts.total)
	}

	if hasMCPs {
		icon := "✓"
		if mcpCounts.registered < mcpCounts.total {
			icon = "!"
			drift += mcpCounts.total - mcpCounts.registered
		}
		fmt.Fprintf(w, "%s %d/%d MCP servers registered\n", icon, mcpCounts.registered, mcpCounts.total)
	}

	if hasAgents {
		fmt.Fprintf(w, "✓ %d agents configured\n", installedAgents)
	}

	fmt.Fprintf(w, "%d drift\n", drift)
	return nil
}

type repoCounts struct {
	total  int
	cloned int
	dirty  int
}

func repoSummaryCounts(entries []RepoEntry) repoCounts {
	var c repoCounts
	c.total = len(entries)
	for _, e := range entries {
		switch e.Status {
		case StatusOK:
			c.cloned++
		case StatusDirty:
			c.cloned++
			c.dirty++
		}
	}
	return c
}

type skillCounts struct {
	total     int
	installed int
}

// skillSummaryCounts counts unique skill names across all non-unmanaged entries.
// A skill name is counted as "installed" if it appears in any entry's Installed
// list, and "missing" if it appears only in Missing lists.
func skillSummaryCounts(entries []SkillEntry) skillCounts {
	installedNames := make(map[string]bool)
	missingNames := make(map[string]bool)

	for _, e := range entries {
		if e.Status == StatusUnmanaged {
			continue
		}
		for _, name := range e.Installed {
			installedNames[name] = true
		}
		for _, name := range e.Missing {
			missingNames[name] = true
		}
	}

	// A name that appears in both installed and missing is considered installed.
	for name := range installedNames {
		delete(missingNames, name)
	}

	var c skillCounts
	c.installed = len(installedNames)
	c.total = len(installedNames) + len(missingNames)
	return c
}

type mcpCounts struct {
	total      int
	registered int
}

func mcpSummaryCounts(entries []MCPEntry) mcpCounts {
	var c mcpCounts
	for _, e := range entries {
		if e.Status == StatusUnmanaged {
			continue
		}
		c.total++
		switch e.Status {
		case StatusOK, StatusPresent, StatusDirty:
			c.registered++
		}
	}
	return c
}
