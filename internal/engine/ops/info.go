package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/pterm/pterm"

	"gaal/internal/config"
	"gaal/internal/core/agent"
	"gaal/internal/engine/render"
	"gaal/internal/mcp"
	"gaal/internal/repo"
	"gaal/internal/skill"
)

// Info renders a detailed view for the given package type.
// pkg must be one of "repo", "skill", "mcp", or "agent".
// filter is an optional name/source substring; if non-empty only matching
// entries are shown. Matching is case-insensitive.
// format controls the output: FormatTable (pterm) or FormatJSON.
func Info(ctx context.Context, repos *repo.Manager, skills *skill.Manager, mcps *mcp.Manager, cfg *config.Config, home, workDir, stateDir, pkg, filter string, format render.OutputFormat) error {
	slog.DebugContext(ctx, "info requested", "package", pkg, "filter", filter, "format", format)

	report, err := Collect(ctx, repos, skills, mcps, home, workDir, stateDir)
	if err != nil {
		return err
	}

	w := os.Stdout

	switch format {
	case render.FormatJSON:
		return renderInfoJSON(w, pkg, filter, report)
	case render.FormatTable:
		switch pkg {
		case "repo":
			return renderRepoInfo(w, cfg.Repositories, report.Repositories, filter)
		case "skill":
			return renderSkillInfo(w, cfg.Skills, report.Skills, filter)
		case "mcp":
			return renderMCPInfo(w, cfg.MCPs, report.MCPs, filter)
		case "agent":
			return renderAgentInfo(w, report.Agents, filter)
		default:
			return fmt.Errorf("unknown package type %q (want: repo, skill, mcp, agent)", pkg)
		}
	}

	// Default: text format.
	switch pkg {
	case "repo":
		return renderRepoInfoText(w, cfg.Repositories, report.Repositories, filter)
	case "skill":
		return renderSkillInfoText(w, cfg.Skills, report.Skills, filter, home, workDir)
	case "mcp":
		return renderMCPInfoText(w, cfg.MCPs, report.MCPs, filter)
	case "agent":
		return renderAgentInfoText(w, report.Agents, filter)
	default:
		return fmt.Errorf("unknown package type %q (want: repo, skill, mcp, agent)", pkg)
	}
}

// renderInfoJSON serialises only the section of the report matching pkg,
// after applying the optional filter, as indented JSON.
func renderInfoJSON(w io.Writer, pkg, filter string, report *render.StatusReport) error {
	var payload any
	switch pkg {
	case "repo":
		filtered := make([]render.RepoEntry, 0)
		for _, e := range report.Repositories {
			if matchFilter(e.Path, filter) {
				filtered = append(filtered, e)
			}
		}
		payload = struct {
			Repositories []render.RepoEntry `json:"repositories"`
		}{filtered}
	case "skill":
		filtered := make([]render.SkillEntry, 0)
		for _, e := range report.Skills {
			if matchFilter(e.Source, filter) {
				filtered = append(filtered, e)
			}
		}
		payload = struct {
			Skills []render.SkillEntry `json:"skills"`
		}{filtered}
	case "mcp":
		filtered := make([]render.MCPEntry, 0)
		for _, e := range report.MCPs {
			if matchFilter(e.Name, filter) {
				filtered = append(filtered, e)
			}
		}
		payload = struct {
			MCPs []render.MCPEntry `json:"mcps"`
		}{filtered}
	case "agent":
		filtered := make([]render.AgentEntry, 0)
		for _, e := range report.Agents {
			if matchFilter(e.Name, filter) {
				filtered = append(filtered, e)
			}
		}
		payload = struct {
			Agents []render.AgentEntry `json:"agents"`
		}{filtered}
	default:
		return fmt.Errorf("unknown package type %q (want: repo, skill, mcp, agent)", pkg)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// infoSection writes a styled section header directly to w.
func infoSection(w io.Writer, title string, count int, color pterm.Color) {
	styled := pterm.NewStyle(pterm.Bold, color).Sprintf("── %s  (%d) ──", title, count)
	fmt.Fprintf(w, "\n%s\n\n", styled)
}

// kvLine renders a key-value line with a fixed-width bold key column.
// ANSI codes are not counted so the key is padded by visible rune count.
func kvLine(key, val string) string {
	pad := max(0, 10-len([]rune(key)))
	styledKey := pterm.NewStyle(pterm.Bold, pterm.FgLightWhite).Sprint(key)
	return fmt.Sprintf(" %s%s  %s", styledKey, strings.Repeat(" ", pad), val)
}

// infoCard holds a single card's title and content lines before rendering.
type infoCard struct {
	title string
	lines []string
}

// visibleLen returns the number of visible runes in s (ANSI codes stripped).
func visibleLen(s string) int {
	return len([]rune(pterm.RemoveColorFromString(s)))
}

// padRight pads s with trailing spaces so its visible width equals width.
func padRight(s string, width int) string {
	n := visibleLen(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// infoBoxes renders all cards with uniform box width (the widest card wins).
// This ensures every box in a section has the same visual width.
//
// pterm sizes a box to: inner = max(max_content_line_visW + 2, title_visW + 4)
// where +2 is the 1-space padding on each content side and +4 accounts for the
// '─' and space pterm inserts on each side of the title inside the border.
// We compute the global maximum inner width, then pad every content line so
// that contentW + 2 == globalMaxInner for all cards.
func infoBoxes(out io.Writer, cards []infoCard) {
	// 1. Compute the effective pterm box inner width for each card, then take the max.
	maxInner := 0
	for _, c := range cards {
		// Title forces: inner >= visibleLen(title) + 4
		if n := visibleLen(c.title) + 4; n > maxInner {
			maxInner = n
		}
		// Each content line forces: inner >= visibleLen(line) + 2
		for _, l := range c.lines {
			if n := visibleLen(l) + 2; n > maxInner {
				maxInner = n
			}
		}
	}

	// 2. Target content width that makes box inner == maxInner for every card.
	targetW := maxInner - 2
	if targetW < 0 {
		targetW = 0
	}

	// 3. Pad every content line and render.
	for _, c := range cards {
		padded := make([]string, len(c.lines))
		for i, l := range c.lines {
			padded[i] = padRight(l, targetW)
		}
		// If the card has no lines, add one spacer so the box has the right width.
		if len(padded) == 0 {
			padded = []string{strings.Repeat(" ", targetW)}
		}
		box := pterm.DefaultBox.
			WithTitle(c.title).
			WithTitleTopLeft().
			Sprint(strings.Join(padded, "\n"))
		fmt.Fprintln(out, box)
	}
}

// matchFilter reports whether name matches filter.
// An empty filter always matches. Matching is case-insensitive substring.
func matchFilter(name, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(
		strings.ToLower(name),
		strings.ToLower(filter),
	)
}

// renderRepoInfo prints a detailed card for each repository entry.
func renderRepoInfo(w io.Writer, cfgRepos map[string]config.ConfigRepo, entries []render.RepoEntry, filter string) error {
	slog.Debug("rendering repo info", "count", len(entries), "filter", filter)

	if len(entries) == 0 {
		infoSection(w, "Repositories", 0, pterm.FgCyan)
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no repositories configured"))
		return nil
	}

	// Stable output: sort by path; apply filter.
	sorted := make([]render.RepoEntry, 0, len(entries))
	for _, e := range entries {
		if matchFilter(e.Path, filter) {
			sorted = append(sorted, e)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	if len(sorted) == 0 {
		infoSection(w, "Repositories", 0, pterm.FgCyan)
		fmt.Fprintln(w, pterm.FgYellow.Sprintf("  no repository matches %q", filter))
		return nil
	}

	infoSection(w, "Repositories", len(sorted), pterm.FgCyan)

	cards := make([]infoCard, 0, len(sorted))
	for _, e := range sorted {
		cfg := cfgRepos[e.Path]
		lines := []string{
			kvLine("Type", e.Type),
			kvLine("URL", e.URL),
		}

		version := orDefault(cfg.Version, "default (HEAD)")
		if e.Current != "" {
			version = fmt.Sprintf("%s  →  current: %s",
				version, pterm.FgCyan.Sprint(e.Current))
		}
		lines = append(lines, kvLine("Version", version))

		if e.Dirty {
			lines = append(lines, kvLine("Dirty", pterm.FgYellow.Sprint("yes — uncommitted local changes")))
		}
		if e.Error != "" {
			lines = append(lines, kvLine("Error", pterm.FgRed.Sprint(e.Error)))
		}

		statusStr := render.StatusCell(e.Status, e.Error)
		title := fmt.Sprintf(" %s   %s ",
			pterm.NewStyle(pterm.Bold).Sprint(e.Path), statusStr)
		cards = append(cards, infoCard{title: title, lines: lines})
	}
	infoBoxes(w, cards)
	return nil
}

// renderSkillInfo prints a detailed card for each skill config entry.
func renderSkillInfo(w io.Writer, cfgSkills []config.ConfigSkill, entries []render.SkillEntry, filter string) error {
	slog.Debug("rendering skill info", "count", len(cfgSkills), "filter", filter)

	if len(cfgSkills) == 0 {
		infoSection(w, "Skills", 0, pterm.FgGreen)
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no skills configured"))
		return nil
	}

	// Apply filter.
	filtered := make([]config.ConfigSkill, 0, len(cfgSkills))
	for _, sc := range cfgSkills {
		if matchFilter(sc.Source, filter) {
			filtered = append(filtered, sc)
		}
	}
	if len(filtered) == 0 {
		infoSection(w, "Skills", 0, pterm.FgGreen)
		fmt.Fprintln(w, pterm.FgYellow.Sprintf("  no skill matches %q", filter))
		return nil
	}
	infoSection(w, "Skills", len(filtered), pterm.FgGreen)

	// Group runtime entries by source so we can correlate them with config.
	bySource := make(map[string][]render.SkillEntry, len(entries))
	for _, e := range entries {
		bySource[e.Source] = append(bySource[e.Source], e)
	}

	cards := make([]infoCard, 0, len(filtered))
	for _, sc := range filtered {
		skillEntries := bySource[sc.Source]
		lines := []string{}

		// ── Spec ────────────────────────────────────────────────────────────
		agents := strings.Join(sc.Agents, ", ")
		if len(sc.Agents) == 0 || (len(sc.Agents) == 1 && sc.Agents[0] == "*") {
			agents = pterm.FgDarkGray.Sprint("all detected")
		}
		lines = append(lines, kvLine("Agents", agents))

		scope := "project"
		if sc.Global {
			scope = pterm.FgCyan.Sprint("global")
		}
		lines = append(lines, kvLine("Scope", scope))

		selected := pterm.FgDarkGray.Sprint("all")
		if len(sc.Select) > 0 {
			selected = strings.Join(sc.Select, ", ")
		}
		lines = append(lines, kvLine("Select", selected))

		// ── Per-agent state ────────────────────────────────────────────────
		if len(skillEntries) > 0 {
			lines = append(lines, "")
			for _, e := range skillEntries {
				agentStatus := render.StatusCell(e.Status, e.Error)
				lines = append(lines, fmt.Sprintf("  %s   %s",
					pterm.NewStyle(pterm.Bold, pterm.FgLightBlue).Sprint(e.Agent), agentStatus))

				treeItems := buildSkillTree(e)
				for _, item := range treeItems {
					lines = append(lines, item)
				}
				if e.Error != "" {
					lines = append(lines, fmt.Sprintf("    %s %s",
						pterm.FgRed.Sprint("✗"), pterm.FgRed.Sprint(e.Error)))
				}
			}
		}

		title := fmt.Sprintf(" %s ", pterm.NewStyle(pterm.Bold).Sprint(sc.Source))
		cards = append(cards, infoCard{title: title, lines: lines})
	}
	infoBoxes(w, cards)
	return nil
}

// buildSkillTree returns tree-formatted lines for a skill's installed/missing/modified files.
func buildSkillTree(e render.SkillEntry) []string {
	type item struct {
		label string
	}
	all := make([]item, 0, len(e.Installed)+len(e.Missing)+len(e.Modified))
	for _, n := range e.Installed {
		all = append(all, item{pterm.FgGreen.Sprint("✓ " + n)})
	}
	for _, n := range e.Missing {
		all = append(all, item{pterm.FgYellow.Sprint("~ " + n + " (missing)")})
	}
	for _, n := range e.Modified {
		all = append(all, item{pterm.FgYellow.Sprint("⚠ " + n + " (modified)")})
	}

	lines := make([]string, 0, len(all))
	for i, it := range all {
		prefix := pterm.FgDarkGray.Sprint("    ├── ")
		if i == len(all)-1 {
			prefix = pterm.FgDarkGray.Sprint("    └── ")
		}
		lines = append(lines, prefix+it.label)
	}
	return lines
}

// renderAgentInfo prints a detailed card for each registered agent.
func renderAgentInfo(w io.Writer, entries []render.AgentEntry, filter string) error {
	slog.Debug("rendering agent info", "count", len(entries), "filter", filter)

	filtered := make([]render.AgentEntry, 0, len(entries))
	for _, e := range entries {
		if matchFilter(e.Name, filter) {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == 0 {
		infoSection(w, "Supported Agents", 0, pterm.FgYellow)
		if filter != "" {
			fmt.Fprintln(w, pterm.FgYellow.Sprintf("  no agent matches %q", filter))
		} else {
			fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no agents registered"))
		}
		return nil
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Installed != filtered[j].Installed {
			return filtered[i].Installed
		}
		return filtered[i].Name < filtered[j].Name
	})

	infoSection(w, "Supported Agents", len(filtered), pterm.FgYellow)

	builtinSet := make(map[string]struct{}, len(agent.Names()))
	for _, n := range agent.Names() {
		builtinSet[n] = struct{}{}
	}

	cards := make([]infoCard, 0, len(filtered))
	for _, e := range filtered {
		lines := []string{}

		installedStr := pterm.FgDarkGray.Sprint("no")
		if e.Installed {
			installedStr = pterm.FgGreen.Sprint("yes")
		}
		lines = append(lines, kvLine("Installed", installedStr))

		if e.ProjectSkillsViaGeneric {
			lines = append(lines, kvLine("Project", pterm.FgDarkGray.Sprintf("via generic convention (%s)", e.ProjectSkillsDir)))
		} else if e.ProjectSkillsDir != "" {
			lines = append(lines, kvLine("Project", pterm.FgCyan.Sprint(e.ProjectSkillsDir)))
		} else {
			lines = append(lines, kvLine("Project", pterm.FgDarkGray.Sprint("via generic convention")))
		}
		if e.GlobalSkillsViaGeneric {
			lines = append(lines, kvLine("Global", pterm.FgDarkGray.Sprintf("via generic convention (%s)", e.GlobalSkillsDir)))
		} else if e.GlobalSkillsDir != "" {
			lines = append(lines, kvLine("Global", pterm.FgCyan.Sprint(e.GlobalSkillsDir)))
		} else {
			lines = append(lines, kvLine("Global", pterm.FgDarkGray.Sprint("via generic convention")))
		}
		if e.ProjectMCPConfigFile != "" {
			lines = append(lines, kvLine("MCP cfg", pterm.FgGreen.Sprint(e.ProjectMCPConfigFile)))
		} else {
			lines = append(lines, kvLine("MCP cfg", pterm.FgDarkGray.Sprint("not supported")))
		}

		source := pterm.FgGreen.Sprint("builtin")
		if e.Source != "" {
			if e.Source == "user" {
				source = pterm.FgCyan.Sprint("user")
			}
		} else if _, ok := builtinSet[e.Name]; !ok {
			source = pterm.FgCyan.Sprint("user")
		}
		lines = append(lines, kvLine("Source", source))

		title := fmt.Sprintf(" %s ", pterm.NewStyle(pterm.Bold).Sprint(e.Name))
		cards = append(cards, infoCard{title: title, lines: lines})
	}
	infoBoxes(w, cards)
	return nil
}

// renderMCPInfo prints a detailed card for each MCP config entry.
func renderMCPInfo(w io.Writer, cfgMCPs []config.ConfigMcp, entries []render.MCPEntry, filter string) error {
	slog.Debug("rendering mcp info", "count", len(cfgMCPs), "filter", filter)

	if len(cfgMCPs) == 0 {
		infoSection(w, "MCP Configs", 0, pterm.FgMagenta)
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no MCP configs configured"))
		return nil
	}

	// Apply filter.
	filtered := make([]config.ConfigMcp, 0, len(cfgMCPs))
	for _, mc := range cfgMCPs {
		if matchFilter(mc.Name, filter) {
			filtered = append(filtered, mc)
		}
	}
	if len(filtered) == 0 {
		infoSection(w, "MCP Configs", 0, pterm.FgMagenta)
		fmt.Fprintln(w, pterm.FgYellow.Sprintf("  no MCP matches %q", filter))
		return nil
	}
	infoSection(w, "MCP Configs", len(filtered), pterm.FgMagenta)

	// Index runtime entries by name.
	byName := make(map[string]render.MCPEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	cards := make([]infoCard, 0, len(filtered))
	for _, mc := range filtered {
		e := byName[mc.Name]
		lines := []string{
			kvLine("Target", mc.Target),
		}

		if mc.Source != "" {
			lines = append(lines, kvLine("Source", mc.Source))
		}

		mergeStr := pterm.FgDarkGray.Sprint("false")
		if mc.MergeEnabled() {
			mergeStr = pterm.FgGreen.Sprint("true")
		}
		lines = append(lines, kvLine("Merge", mergeStr))

		// ── Inline definition ──────────────────────────────────────────────
		if mc.Inline != nil {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("  %s",
				pterm.NewStyle(pterm.Bold, pterm.FgLightWhite).Sprint("Inline:")))
			lines = append(lines, kvLine("  Command", pterm.FgCyan.Sprint(mc.Inline.Command)))

			if len(mc.Inline.Args) > 0 {
				lines = append(lines, kvLine("  Args",
					pterm.FgLightCyan.Sprint(strings.Join(mc.Inline.Args, " "))))
			}
			if len(mc.Inline.Env) > 0 {
				envPairs := make([]string, 0, len(mc.Inline.Env))
				for k, v := range mc.Inline.Env {
					envPairs = append(envPairs, pterm.FgLightWhite.Sprint(k)+"="+v)
				}
				sort.Strings(envPairs)
				lines = append(lines, kvLine("  Env", strings.Join(envPairs, ", ")))
			}
		}

		// ── Runtime state ─────────────────────────────────────────────────
		if e.Dirty {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("  %s",
				pterm.FgYellow.Sprint("⚠  local changes detected in target file")))
		}
		if e.Error != "" {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("  %s", pterm.FgRed.Sprint("✗  "+e.Error)))
		}

		statusStr := render.StatusCell(e.Status, e.Error)
		title := fmt.Sprintf(" %s   %s ",
			pterm.NewStyle(pterm.Bold).Sprint(mc.Name), statusStr)
		cards = append(cards, infoCard{title: title, lines: lines})
	}
	infoBoxes(w, cards)
	return nil
}
