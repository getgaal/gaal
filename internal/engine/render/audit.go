package render

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/pterm/pterm"
)

// AuditRenderer renders an AuditReport.
type AuditRenderer interface {
	Render(w io.Writer, r *AuditReport) error
}

// NewAuditRenderer returns the appropriate AuditRenderer for the given format.
func NewAuditRenderer(format OutputFormat) AuditRenderer {
	switch format {
	case FormatJSON:
		return &auditJSONRenderer{}
	case FormatTable:
		return &auditTableRenderer{}
	default:
		return &auditTextRenderer{}
	}
}

// ── JSON renderer ────────────────────────────────────────────────────────────

type auditJSONRenderer struct{}

func (j *auditJSONRenderer) Render(w io.Writer, r *AuditReport) error {
	slog.Debug("rendering audit json output")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ── Text renderer ───────────────────────────────────────────────────────────
//
// Matches the sample in cli/audit.mdx:
//
//	discovered:
//	  skills (project)
//	    .claude/skills/code-review     (claude-code)
//	  mcps
//	    ~/.config/claude/claude_desktop_config.json
//	      filesystem, git, github

type auditTextRenderer struct{}

func (tr *auditTextRenderer) Render(w io.Writer, r *AuditReport) error {
	slog.Debug("rendering audit text output")

	fmt.Fprintln(w, "discovered:")
	tr.skillGroups(w, r.Skills, r.Home)
	tr.mcpGroups(w, r.MCPs, r.Home)
	return nil
}

func (tr *auditTextRenderer) skillGroups(w io.Writer, skills []AuditSkillEntry, home string) {
	groups := map[string][]AuditSkillEntry{}
	for _, s := range skills {
		groups[s.Source] = append(groups[s.Source], s)
	}

	// Fixed order for stable output; labels mirror the docs example wording.
	order := []struct {
		key, label string
	}{
		{"project", "skills (project)"},
		{"global", "skills (global)"},
		{"package-manager", "skills (package manager)"},
	}
	for _, g := range order {
		entries := groups[g.key]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

		pathWidth := 0
		for _, e := range entries {
			if n := len(shortenHome(e.Path, home)); n > pathWidth {
				pathWidth = n
			}
		}

		fmt.Fprintf(w, "  %s\n", g.label)
		for _, e := range entries {
			path := shortenHome(e.Path, home)
			fmt.Fprintf(w, "    %s  (%s)\n", padText(path, pathWidth), e.Agent)
		}
	}
}

func (tr *auditTextRenderer) mcpGroups(w io.Writer, mcps []AuditMCPEntry, home string) {
	if len(mcps) == 0 {
		return
	}

	// Collapse entries by config file so the same file (reachable via several
	// agents) renders once with the combined server list.
	type fileGroup struct {
		path    string
		servers map[string]struct{}
	}
	byFile := map[string]*fileGroup{}
	order := []string{}
	for _, e := range mcps {
		g, ok := byFile[e.ConfigFile]
		if !ok {
			g = &fileGroup{path: e.ConfigFile, servers: map[string]struct{}{}}
			byFile[e.ConfigFile] = g
			order = append(order, e.ConfigFile)
		}
		for _, s := range e.Servers {
			g.servers[s] = struct{}{}
		}
	}

	fmt.Fprintln(w, "  mcps")
	for _, path := range order {
		g := byFile[path]
		servers := make([]string, 0, len(g.servers))
		for s := range g.servers {
			servers = append(servers, s)
		}
		sort.Strings(servers)
		fmt.Fprintf(w, "    %s\n", shortenHome(g.path, home))
		if len(servers) > 0 {
			fmt.Fprintf(w, "      %s\n", strings.Join(servers, ", "))
		}
	}
}

// ── Table renderer ───────────────────────────────────────────────────────────

type auditTableRenderer struct{}

func (tr *auditTableRenderer) Render(w io.Writer, r *AuditReport) error {
	slog.Debug("rendering audit table output")

	termW := pterm.GetTerminalWidth()
	if termW < 60 {
		termW = 120
	}

	if err := tr.skillTable(w, r.Skills, r.Home, termW); err != nil {
		return err
	}
	if err := tr.mcpTable(w, r.MCPs, r.Home, termW); err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

func (tr *auditTableRenderer) section(w io.Writer, title string, count int) {
	styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── %s  (%d) ──", title, count)
	fmt.Fprintf(w, "\n%s\n", styled)
}

func (tr *auditTableRenderer) skillTable(w io.Writer, entries []AuditSkillEntry, home string, termW int) error {
	tr.section(w, "Discovered Skills", len(entries))

	if len(entries) == 0 {
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no skills discovered"))
		return nil
	}

	// 4 columns: NAME | AGENT | SOURCE | PATH
	// Fixed: AGENT(20) + SOURCE(16) = 36; overhead for 4 cols = 3*4+1 = 13
	// NAME gets 30% of the remaining space, PATH gets 70%.
	overhead := 3*4 + 1
	remaining := termW - overhead - 36
	if remaining < 48 {
		remaining = 48
	}
	nameMax := remaining * 30 / 100
	pathMax := remaining - nameMax
	if nameMax < 18 {
		nameMax = 18
	}
	if pathMax < 28 {
		pathMax = 28
	}

	data := pterm.TableData{{"NAME", "AGENT", "SOURCE", "PATH"}}
	for _, e := range entries {
		data = append(data, []string{
			trunc(e.Name, nameMax),
			e.Agent,
			sourceCell(e.Source),
			trunc(shortenHome(e.Path, home), pathMax),
		})
	}

	tbl := &tableRenderer{}
	return tbl.ptermTable(w, data)
}

func (tr *auditTableRenderer) mcpTable(w io.Writer, entries []AuditMCPEntry, home string, termW int) error {
	tr.section(w, "Discovered MCP Servers", len(entries))

	if len(entries) == 0 {
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no MCP servers discovered"))
		return nil
	}

	// Fixed cols: AGENT(22) = 22; variable cols: CONFIG FILE and SERVERS share the rest.
	vw := varColWidth(termW, 3, 2, 22)
	if vw < 14 {
		vw = 14
	}

	data := pterm.TableData{{"AGENT", "CONFIG FILE", "SERVERS"}}
	for _, e := range entries {
		servers := strings.Join(e.Servers, ", ")
		data = append(data, []string{
			e.Agent,
			trunc(shortenHome(e.ConfigFile, home), vw),
			trunc(servers, vw),
		})
	}

	tbl := &tableRenderer{}
	return tbl.ptermTable(w, data)
}

// shortenHome replaces the home directory prefix of a path with "~/".
func shortenHome(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	// Ensure home ends without separator for clean prefix matching.
	prefix := strings.TrimRight(home, "/\\")
	if strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"\\") {
		return "~" + path[len(prefix):]
	}
	return path
}

// sourceCell renders a source label with a distinctive colour.
func sourceCell(source string) string {
	switch source {
	case "project":
		return pterm.FgCyan.Sprint(source)
	case "global":
		return pterm.FgGreen.Sprint(source)
	case "package-manager":
		return pterm.FgYellow.Sprint(source)
	default:
		return source
	}
}
