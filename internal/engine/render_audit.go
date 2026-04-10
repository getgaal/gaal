package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/pterm/pterm"
)

// AuditRenderer renders an AuditReport.
type AuditRenderer interface {
	Render(w io.Writer, r *AuditReport) error
}

// NewAuditRenderer returns the appropriate AuditRenderer for the given format.
func NewAuditRenderer(format OutputFormat) AuditRenderer {
	if format == FormatJSON {
		return &auditJSONRenderer{}
	}
	return &auditTableRenderer{}
}

// ── JSON renderer ────────────────────────────────────────────────────────────

type auditJSONRenderer struct{}

func (j *auditJSONRenderer) Render(w io.Writer, r *AuditReport) error {
	slog.Debug("rendering audit json output")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ── Table renderer ───────────────────────────────────────────────────────────

type auditTableRenderer struct{}

func (tr *auditTableRenderer) Render(w io.Writer, r *AuditReport) error {
	slog.Debug("rendering audit table output")

	termW := pterm.GetTerminalWidth()
	if termW < 60 {
		termW = 120
	}

	if err := tr.skillTable(w, r.Skills, termW); err != nil {
		return err
	}
	if err := tr.mcpTable(w, r.MCPs, termW); err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

func (tr *auditTableRenderer) section(w io.Writer, title string, count int) {
	styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── %s  (%d) ──", title, count)
	fmt.Fprintf(w, "\n%s\n", styled)
}

func (tr *auditTableRenderer) skillTable(w io.Writer, entries []AuditSkillEntry, termW int) error {
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
			trunc(e.Path, pathMax),
		})
	}

	tbl := &tableRenderer{}
	return tbl.ptermTable(w, data)
}

func (tr *auditTableRenderer) mcpTable(w io.Writer, entries []AuditMCPEntry, termW int) error {
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
			trunc(e.ConfigFile, vw),
			trunc(servers, vw),
		})
	}

	tbl := &tableRenderer{}
	return tbl.ptermTable(w, data)
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
