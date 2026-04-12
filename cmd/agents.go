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

	if format == engine.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Agents []render.AgentEntry `json:"agents"`
		}{entries})
	}

	return renderAgentsTable(w, entries)
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

	if format == engine.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Agent *render.AgentDetail `json:"agent"`
		}{detail})
	}

	return renderAgentDetailCard(w, detail)
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
