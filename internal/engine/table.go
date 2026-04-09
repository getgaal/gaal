package engine

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/pterm/pterm"
)

// statusLabel maps a StatusCode to a human-readable display string.
var statusLabel = map[StatusCode]string{
	StatusOK:        "synced",
	StatusDirty:     "dirty",
	StatusNotCloned: "not cloned",
	StatusPartial:   "partial",
	StatusPresent:   "present",
	StatusAbsent:    "absent",
	StatusError:     "error",
}

// statusCell renders a StatusCode as a coloured pterm string.
// errMsg is included verbatim when the code is StatusError.
func statusCell(code StatusCode, errMsg string) string {
	label, ok := statusLabel[code]
	if !ok {
		label = string(code)
	}
	switch code {
	case StatusOK, StatusPresent:
		return pterm.FgGreen.Sprint("✓ " + label)
	case StatusDirty:
		return pterm.FgYellow.Sprint("⚠ " + label)
	case StatusNotCloned, StatusAbsent, StatusPartial:
		return pterm.FgYellow.Sprint("~ " + label)
	case StatusError:
		msg := label
		if errMsg != "" {
			msg = errMsg
		}
		return pterm.FgRed.Sprint("✗ " + msg)
	default:
		return label
	}
}

// trunc truncates s to max visible runes, appending "…" when shortened.
// s must be a plain string (no embedded ANSI codes).
func trunc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// varColWidth computes the maximum visual width for variable-length columns.
//   - termW    : terminal width in characters
//   - numCols  : total number of columns in the table
//   - numVar   : number of variable-width columns (to share remaining space)
//   - fixedSum : total visual width consumed by fixed-width columns
func varColWidth(termW, numCols, numVar, fixedSum int) int {
	// Boxed pterm table overhead per row: 3 chars per column + 1 (outer right border).
	overhead := 3*numCols + 1
	avail := termW - overhead - fixedSum
	if avail < numVar*12 {
		avail = numVar * 12
	}
	return avail / numVar
}

// tableRenderer implements Renderer using pterm tables with adaptive column widths.
type tableRenderer struct{}

func (tr *tableRenderer) Render(w io.Writer, r *StatusReport) error {
	slog.Debug("rendering table output")

	termW := pterm.GetTerminalWidth()
	if termW < 60 {
		termW = 120
	}

	if err := tr.repoTable(w, r.Repositories, termW); err != nil {
		return err
	}
	if err := tr.skillTable(w, r.Skills, termW); err != nil {
		return err
	}
	if err := tr.mcpTable(w, r.MCPs, termW); err != nil {
		return err
	}
	if err := tr.agentTable(w, r.Agents, termW); err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

// section writes a styled section header directly to w.
func (tr *tableRenderer) section(w io.Writer, title string, count int) {
	styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── %s  (%d) ──", title, count)
	fmt.Fprintf(w, "\n%s\n", styled)
}

// buildBorderLine builds a horizontal border of the given rune width,
// placing junction at each position listed in junctions, surrounded by
// left and right corner runes, and filled with fill everywhere else.
func buildBorderLine(width int, junctions []int, left, right, junction, fill rune) string {
	runes := make([]rune, width)
	for i := range runes {
		runes[i] = fill
	}
	for _, pos := range junctions {
		if pos >= 0 && pos < width {
			runes[pos] = junction
		}
	}
	return string(left) + string(runes) + string(right)
}

// ptermTable renders data as a boxed table with proper Unicode box-drawing crossings.
//
// Strategy: delegate alignment to pterm (no box, no header separator), then
// manually build the top/header-separator/bottom border lines with ┬ ┼ ┴
// at every column-separator position, and wrap each content row with │ …│.
func (tr *tableRenderer) ptermTable(w io.Writer, data pterm.TableData) error {
	s, err := pterm.DefaultTable.
		WithHasHeader(true).
		WithSeparator(" │ ").
		WithHeaderStyle(pterm.NewStyle(pterm.Bold, pterm.FgLightWhite)).
		WithData(data).
		Srender()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 0 {
		fmt.Fprintln(w)
		return nil
	}

	// Use a non-styled line to locate │ rune positions.
	// Prefer the first data row (index 1) to avoid ANSI in the header.
	refLine := lines[0]
	if len(lines) > 1 {
		refLine = lines[1]
	}
	cleanRef := []rune(pterm.RemoveColorFromString(refLine))
	rowWidth := len(cleanRef)

	var sepPositions []int
	for i, r := range cleanRef {
		if r == '│' {
			sepPositions = append(sepPositions, i)
		}
	}

	top := buildBorderLine(rowWidth, sepPositions, '┌', '┐', '┬', '─')
	headerSep := buildBorderLine(rowWidth, sepPositions, '├', '┤', '┼', '─')
	bottom := buildBorderLine(rowWidth, sepPositions, '└', '┘', '┴', '─')

	fmt.Fprintln(w, top)
	for i, line := range lines {
		fmt.Fprintf(w, "│%s│\n", line)
		if i == 0 {
			fmt.Fprintln(w, headerSep)
		}
	}
	fmt.Fprintln(w, bottom)
	return nil
}

func (tr *tableRenderer) repoTable(w io.Writer, entries []RepoEntry, termW int) error {
	tr.section(w, "Repositories", len(entries))
	// Fixed cols: TYPE(8) + STATUS(14 visible) = 22
	// Variable cols: PATH and INFO share the rest (55% / 45%).
	vw := varColWidth(termW, 4, 2, 22)
	pathMax := vw * 55 / 100
	infoMax := vw * 45 / 100
	if pathMax < 15 {
		pathMax = 15
	}
	if infoMax < 15 {
		infoMax = 15
	}

	data := pterm.TableData{{"PATH", "TYPE", "STATUS", "VERSION / URL"}}
	for _, e := range entries {
		var info string
		switch e.Status {
		case StatusNotCloned:
			info = e.URL
		case StatusOK:
			info = e.Current + " → " + e.Want
		case StatusDirty:
			info = e.Current + " → " + e.Want + pterm.FgYellow.Sprint(" (local changes)")
		case StatusError:
			info = e.Error
		}
		data = append(data, []string{
			trunc(e.Path, pathMax),
			e.Type,
			statusCell(e.Status, e.Error),
			trunc(info, infoMax),
		})
	}
	return tr.ptermTable(w, data)
}

func (tr *tableRenderer) skillTable(w io.Writer, entries []SkillEntry, termW int) error {
	tr.section(w, "Skills", len(entries))
	// Fixed cols: AGENT(20) + STATUS(14) = 34
	// Variable cols: SOURCE, INSTALLED, MISSING, MODIFIED share the rest equally.
	vw := varColWidth(termW, 6, 4, 34)
	if vw < 12 {
		vw = 12
	}

	data := pterm.TableData{{"SOURCE", "AGENT", "STATUS", "INSTALLED", "MISSING", "MODIFIED"}}
	for _, e := range entries {
		installed := strings.Join(e.Installed, ", ")
		missing := strings.Join(e.Missing, ", ")
		modified := strings.Join(e.Modified, ", ")
		if installed == "" {
			installed = "—"
		}
		if missing == "" {
			missing = "—"
		}
		if modified == "" {
			modified = "—"
		} else {
			modified = pterm.FgYellow.Sprint(modified)
		}
		data = append(data, []string{
			trunc(e.Source, vw),
			e.Agent,
			statusCell(e.Status, e.Error),
			trunc(installed, vw),
			trunc(missing, vw),
			trunc(modified, vw),
		})
	}
	return tr.ptermTable(w, data)
}

func (tr *tableRenderer) mcpTable(w io.Writer, entries []MCPEntry, termW int) error {
	tr.section(w, "MCP Configs", len(entries))
	// Fixed cols: NAME(20) + STATUS(14) = 34
	// Variable col: TARGET takes the remaining space.
	vw := varColWidth(termW, 3, 1, 34)
	if vw < 20 {
		vw = 20
	}

	data := pterm.TableData{{"NAME", "STATUS", "TARGET"}}
	for _, e := range entries {
		target := trunc(e.Target, vw)
		if e.Dirty {
			target += pterm.FgYellow.Sprint(" (local changes)")
		}
		data = append(data, []string{
			e.Name,
			statusCell(e.Status, e.Error),
			target,
		})
	}
	return tr.ptermTable(w, data)
}

func (tr *tableRenderer) agentTable(w io.Writer, entries []AgentEntry, termW int) error {
	tr.section(w, "Supported Agents", len(entries))
	// Fixed: AGENT(20) = 20; 3 variable path columns share the rest.
	vw := varColWidth(termW, 4, 3, 20)
	if vw < 14 {
		vw = 14
	}

	data := pterm.TableData{{"AGENT", "PROJECT SKILLS DIR", "GLOBAL SKILLS DIR", "MCP CONFIG"}}
	for _, e := range entries {
		mcpCfg := e.MCPConfigFile
		if mcpCfg == "" {
			mcpCfg = "—"
		}
		data = append(data, []string{
			e.Name,
			trunc(e.ProjectSkillsDir, vw),
			trunc(e.GlobalSkillsDir, vw),
			trunc(mcpCfg, vw),
		})
	}
	return tr.ptermTable(w, data)
}
