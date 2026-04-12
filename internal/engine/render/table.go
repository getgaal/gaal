package render

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
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

// StatusCell renders a StatusCode as a coloured pterm string.
// This exported wrapper allows other packages to reuse the same style.
func StatusCell(code StatusCode, errMsg string) string {
	return statusCell(code, errMsg)
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
	// Prefer the first data row (index 1) to avoid ANSI in the header,
	// but fall back to the header when there are no data rows (empty table):
	// pterm renders an empty string for the data row, giving rowWidth = 0.
	refLine := lines[0]
	for _, candidate := range lines[1:] {
		if strings.ContainsRune(pterm.RemoveColorFromString(candidate), '│') {
			refLine = candidate
			break
		}
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
		// pterm injects an empty string when there are no data rows — skip it.
		if i > 0 && strings.TrimSpace(pterm.RemoveColorFromString(line)) == "" {
			continue
		}
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

// aggregatedSkill rolls up all per-(source, agent) SkillEntry rows for a
// single skill name. See [aggregateSkillsByName] for semantics.
type aggregatedSkill struct {
	Name      string
	Sources   []string
	Status    StatusCode
	Agents    []string
	AllAgents bool
	Error     string
}

// aggregateSkillsByName groups per-(source, agent) skill entries into one
// row per unique skill name. The result is sorted alphabetically by name.
//
// For each skill name, "targeted agents" is the set of agents that list the
// skill in Installed ∪ Missing ∪ Modified — i.e. agents gaal expected to
// manage the skill for. AllAgents is true when the skill is installed in
// every one of those targeted agents (including "installed but dirty"),
// which the renderer shows as `*` in the AGENTS column.
func aggregateSkillsByName(entries []SkillEntry) []aggregatedSkill {
	type bucket struct {
		sources   map[string]struct{}
		targeted  map[string]struct{}
		installed map[string]struct{}
		dirty     bool
		errored   bool
		errMsg    string
	}
	byName := map[string]*bucket{}

	get := func(name string) *bucket {
		b, ok := byName[name]
		if !ok {
			b = &bucket{
				sources:   map[string]struct{}{},
				targeted:  map[string]struct{}{},
				installed: map[string]struct{}{},
			}
			byName[name] = b
		}
		return b
	}

	markEntry := func(e SkillEntry, name string, installed bool) {
		b := get(name)
		b.sources[e.Source] = struct{}{}
		b.targeted[e.Agent] = struct{}{}
		if installed {
			b.installed[e.Agent] = struct{}{}
		}
		if e.Status == StatusError {
			b.errored = true
			if b.errMsg == "" {
				b.errMsg = e.Error
			}
		}
	}

	for _, e := range entries {
		for _, name := range e.Installed {
			markEntry(e, name, true)
		}
		for _, name := range e.Missing {
			markEntry(e, name, false)
		}
		for _, name := range e.Modified {
			// Modified implies installed, but may not appear in Installed
			// (status is reported as dirty, not ok). Track it as installed
			// and flag the bucket as dirty.
			markEntry(e, name, true)
			get(name).dirty = true
		}
	}

	out := make([]aggregatedSkill, 0, len(byName))
	for name, b := range byName {
		s := aggregatedSkill{
			Name:    name,
			Sources: keysSorted(b.sources),
			Agents:  keysSorted(b.installed),
			Error:   b.errMsg,
		}
		s.AllAgents = len(b.installed) > 0 && len(b.installed) == len(b.targeted)
		switch {
		case b.errored:
			s.Status = StatusError
		case b.dirty:
			s.Status = StatusDirty
		case s.AllAgents:
			s.Status = StatusOK
		default:
			s.Status = StatusPartial
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func keysSorted(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (tr *tableRenderer) skillTable(w io.Writer, entries []SkillEntry, termW int) error {
	aggregated := aggregateSkillsByName(entries)
	tr.section(w, "Skills", len(aggregated))

	// Fixed col: STATUS(14). SKILL, SOURCE, AGENTS share the rest.
	vw := varColWidth(termW, 4, 3, 14)
	if vw < 12 {
		vw = 12
	}

	data := pterm.TableData{{"SKILL", "SOURCE", "STATUS", "AGENTS"}}
	for _, s := range aggregated {
		agents := "—"
		switch {
		case s.AllAgents:
			agents = "*"
		case len(s.Agents) > 0:
			agents = strings.Join(s.Agents, ", ")
		}
		data = append(data, []string{
			trunc(s.Name, vw),
			trunc(strings.Join(s.Sources, ", "), vw),
			statusCell(s.Status, s.Error),
			trunc(agents, vw),
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
	// Fixed: AGENT(20) + INSTALLED(11) = 31; 3 variable path columns share the rest.
	vw := varColWidth(termW, 5, 3, 31)
	if vw < 14 {
		vw = 14
	}

	data := pterm.TableData{{"AGENT", "INSTALLED", "PROJECT SKILLS DIR", "GLOBAL SKILLS DIR", "PROJECT MCP CONFIG"}}
	for _, e := range entries {
		installed := pterm.FgDarkGray.Sprint("—")
		if e.Installed {
			installed = pterm.FgGreen.Sprint("✓")
		}
		mcpCfg := e.ProjectMCPConfigFile
		if mcpCfg == "" {
			mcpCfg = "—"
		}
		data = append(data, []string{
			e.Name,
			installed,
			trunc(e.ProjectSkillsDir, vw),
			trunc(e.GlobalSkillsDir, vw),
			trunc(mcpCfg, vw),
		})
	}
	return tr.ptermTable(w, data)
}
