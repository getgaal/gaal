package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/engine/render"
	"gaal/internal/telemetry"
)

var (
	agentsInstalled bool
)

var agentsCmd = &cobra.Command{
	Use:   "agents [name]",
	Short: "List registered coding agents or show details for one",
	Long: `Lists every registered agent and whether it is installed on this machine.

Pass an agent name to see a detailed view including search paths,
skill counts, and MCP configuration.

Examples:
  gaal agents                # all registered agents (installed first)
  gaal agents --installed    # only agents detected on this machine
  gaal agents cursor         # detailed view for one agent`,
	SilenceUsage: true,
	Annotations:  map[string]string{"config": "optional"},
	Args:         cobra.MaximumNArgs(1),
	RunE:         runAgents,
}

func init() {
	agentsCmd.Flags().BoolVarP(&agentsInstalled, "installed", "i", false, "show only installed agents")
	rootCmd.AddCommand(agentsCmd)
}

func runAgents(_ *cobra.Command, args []string) error {
	telemetry.Track("agents")
	eng := engine.NewWithOptions(&config.Config{}, engineOpts)
	w := os.Stdout
	format := engine.OutputFormat(outputFormat)

	if len(args) == 1 {
		return runAgentDetail(eng, w, args[0], format)
	}
	return runAgentList(eng, w, format)
}

func runAgentList(eng *engine.Engine, w io.Writer, format engine.OutputFormat) error {
	entries, err := eng.ListAgents()
	if err != nil {
		return err
	}

	if agentsInstalled {
		filtered := make([]render.AgentEntry, 0, len(entries))
		for _, e := range entries {
			if e.Installed {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	switch format {
	case engine.FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Agents []render.AgentEntry `json:"agents"`
		}{entries})
	case engine.FormatTable:
		return renderAgentsTable(w, entries)
	default:
		return renderAgentsText(w, entries)
	}
}

// renderAgentsText prints the agent list as space-aligned columns matching
// the sample in docs/content/cli/agents.mdx.
func renderAgentsText(w io.Writer, entries []render.AgentEntry) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, "no agents found")
		return nil
	}

	headers := []string{"NAME", "INSTALLED", "PROJECT_SKILLS", "GLOBAL_SKILLS", "MCP_CONFIG"}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		installed := "no"
		if e.Installed {
			installed = "yes"
		}
		mcpCfg := e.ProjectMCPConfigFile
		if mcpCfg == "" {
			mcpCfg = "—"
		}
		rows = append(rows, []string{
			e.Name,
			installed,
			dashIfEmpty(e.ProjectSkillsDir),
			dashIfEmpty(e.GlobalSkillsDir),
			mcpCfg,
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow := func(cells []string) {
		var b strings.Builder
		for i, c := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			if i == len(cells)-1 {
				b.WriteString(c)
			} else {
				b.WriteString(c)
				b.WriteString(strings.Repeat(" ", widths[i]-len(c)))
			}
		}
		fmt.Fprintln(w, b.String())
	}
	printRow(headers)
	for _, row := range rows {
		printRow(row)
	}
	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func renderAgentsTable(w io.Writer, entries []render.AgentEntry) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, pterm.FgDarkGray.Sprint("  no agents found"))
		return nil
	}

	styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── Agents  (%d) ──", len(entries))
	fmt.Fprintf(w, "\n%s\n", styled)

	data := pterm.TableData{{"NAME", "INSTALLED", "SOURCE"}}
	for _, e := range entries {
		installed := pterm.FgDarkGray.Sprint("—")
		if e.Installed {
			installed = pterm.FgGreen.Sprint("✓")
		}
		source := pterm.FgGreen.Sprint(e.Source)
		if e.Source == "user" {
			source = pterm.FgCyan.Sprint(e.Source)
		}

		data = append(data, []string{
			e.Name,
			installed,
			source,
		})
	}

	return render.BoxedTable(w, data)
}

func runAgentDetail(eng *engine.Engine, w io.Writer, name string, format engine.OutputFormat) error {
	detail, err := eng.AgentDetail(name)
	if err != nil {
		return err
	}

	switch format {
	case engine.FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Agent *render.AgentDetail `json:"agent"`
		}{detail})
	case engine.FormatTable:
		return renderAgentDetailCard(w, detail)
	default:
		return renderAgentDetailText(w, detail)
	}
}

// renderAgentDetailText prints the agent detail as a plain indented block
// matching docs/content/cli/agents.mdx (detailed view).
func renderAgentDetailText(w io.Writer, d *render.AgentDetail) error {
	label := "not installed"
	if d.Installed {
		label = "installed"
	}
	fmt.Fprintf(w, "%s (%s)\n", d.Name, label)

	source := d.Source
	if source == "" {
		source = "built-in"
	}
	if source == "builtin" {
		source = "built-in"
	}
	fmt.Fprintf(w, "  %-16s %s\n", "source:", source)

	byLabel := map[string][]render.AgentPath{}
	labelOrder := []string{}
	for _, p := range d.Paths {
		if _, ok := byLabel[p.Label]; !ok {
			labelOrder = append(labelOrder, p.Label)
		}
		byLabel[p.Label] = append(byLabel[p.Label], p)
	}

	if paths := byLabel["project"]; len(paths) > 0 {
		fmt.Fprintf(w, "  %-16s %s\n", "project skills:", paths[0].Path)
	}
	if paths := byLabel["global"]; len(paths) > 0 {
		fmt.Fprintf(w, "  %-16s %s\n", "global skills:", paths[0].Path)
	}

	if d.MCPSupport {
		marker := "(not found)"
		if d.MCPExists {
			marker = "(exists)"
		}
		fmt.Fprintf(w, "  %-16s %s %s\n", "mcp config:", d.MCPConfig, marker)
	}

	writeExtraPaths := func(label string, paths []render.AgentPath) {
		if len(paths) <= 1 {
			return
		}
		extras := make([]string, 0, len(paths)-1)
		for _, p := range paths[1:] {
			extras = append(extras, p.Path)
		}
		fmt.Fprintf(w, "  audit search (%s):\n    %s\n", label, strings.Join(extras, ", "))
	}
	writeExtraPaths("project", byLabel["project"])
	writeExtraPaths("global", byLabel["global"])

	if pm := byLabel["package-manager"]; len(pm) > 0 {
		paths := make([]string, 0, len(pm))
		for _, p := range pm {
			paths = append(paths, p.Path)
		}
		fmt.Fprintf(w, "  package-manager search:\n    %s\n", strings.Join(paths, ", "))
	}
	// Keep label order deterministic even when an unexpected label appears.
	for _, name := range labelOrder {
		switch name {
		case "project", "global", "package-manager":
			continue
		}
		paths := byLabel[name]
		list := make([]string, 0, len(paths))
		for _, p := range paths {
			list = append(list, p.Path)
		}
		fmt.Fprintf(w, "  %s:\n    %s\n", name, strings.Join(list, ", "))
	}

	for _, warn := range d.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warn)
	}
	return nil
}

func renderAgentDetailCard(w io.Writer, d *render.AgentDetail) error {
	styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── Agent: %s ──", d.Name)
	fmt.Fprintf(w, "\n%s\n\n", styled)

	kvPad := 12
	kv := func(key, val string) string {
		pad := kvPad - len([]rune(key))
		if pad < 0 {
			pad = 0
		}
		styledKey := pterm.NewStyle(pterm.Bold, pterm.FgLightWhite).Sprint(key)
		return fmt.Sprintf(" %s%s  %s", styledKey, strings.Repeat(" ", pad), val)
	}

	installedStr := pterm.FgDarkGray.Sprint("no")
	if d.Installed {
		installedStr = pterm.FgGreen.Sprint("yes")
	}
	fmt.Fprintln(w, kv("Installed", installedStr))

	sourceStr := pterm.FgGreen.Sprint(d.Source)
	if d.Source == "user" {
		sourceStr = pterm.FgCyan.Sprint(d.Source)
	}
	fmt.Fprintln(w, kv("Source", sourceStr))

	mcpStr := pterm.FgDarkGray.Sprint("not supported")
	if d.MCPSupport {
		existsMarker := pterm.FgYellow.Sprint("(not found)")
		if d.MCPExists {
			existsMarker = pterm.FgGreen.Sprint("(exists)")
		}
		mcpStr = fmt.Sprintf("%s  %s", d.MCPConfig, existsMarker)
	}
	fmt.Fprintln(w, kv("MCP config", mcpStr))

	fmt.Fprintln(w)
	fmt.Fprintln(w, pterm.NewStyle(pterm.Bold, pterm.FgLightWhite).Sprint(" Search paths:"))
	for _, p := range d.Paths {
		existsMarker := pterm.FgYellow.Sprint("✗")
		if p.Exists {
			existsMarker = pterm.FgGreen.Sprintf("✓ %d skills", p.SkillCount)
		}
		labelColor := pterm.FgCyan
		if p.Label == "global" {
			labelColor = pterm.FgGreen
		} else if p.Label == "package-manager" {
			labelColor = pterm.FgYellow
		}
		fmt.Fprintf(w, "   %s  %s  %s\n",
			labelColor.Sprintf("%-16s", p.Label),
			p.Path,
			existsMarker)
	}

	if len(d.Warnings) > 0 {
		fmt.Fprintln(w)
		for _, warn := range d.Warnings {
			fmt.Fprintf(w, " %s\n", pterm.FgYellow.Sprint("⚠  "+warn))
		}
	}

	fmt.Fprintln(w)
	return nil
}
