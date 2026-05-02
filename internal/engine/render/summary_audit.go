package render

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// summaryAuditRenderer implements AuditRenderer producing a compact count-based
// audit summary. It shows unique skill names, unique MCP server names, and the
// number of agents scanned.
type summaryAuditRenderer struct{}

func (r *summaryAuditRenderer) Render(w io.Writer, report *AuditReport) error {
	if len(report.Skills) == 0 && len(report.MCPs) == 0 {
		fmt.Fprintln(w, "No skills or MCP servers discovered.")
		return nil
	}

	// Count unique skill names.
	skillNames := map[string]struct{}{}
	for _, s := range report.Skills {
		skillNames[s.Name] = struct{}{}
	}

	// Count unique MCP server names across all entries.
	mcpNames := map[string]struct{}{}
	for _, m := range report.MCPs {
		for _, s := range m.Servers {
			mcpNames[s] = struct{}{}
		}
	}

	// Collect unique agents (from both skills and MCPs).
	agentNames := map[string]struct{}{}
	for _, s := range report.Skills {
		if s.Agent != "" {
			agentNames[s.Agent] = struct{}{}
		}
	}
	for _, m := range report.MCPs {
		if m.Agent != "" {
			agentNames[m.Agent] = struct{}{}
		}
	}

	agents := make([]string, 0, len(agentNames))
	for a := range agentNames {
		agents = append(agents, a)
	}
	sort.Strings(agents)

	fmt.Fprintf(w, "Found %s across %s.\n",
		pluralise(len(skillNames), "skill", "skills"),
		pluralise(len(agents), "agent", "agents"),
	)
	fmt.Fprintf(w, "Found %s.\n", pluralise(len(mcpNames), "MCP server", "MCP servers"))
	fmt.Fprintf(w, "%s scanned: %s\n",
		strings.Title(pluralise(len(agents), "agent", "agents")), //nolint:staticcheck
		strings.Join(agents, ", "),
	)

	return nil
}
