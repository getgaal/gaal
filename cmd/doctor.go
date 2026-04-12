package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/engine/ops"
	"gaal/internal/telemetry"
)

var (
	doctorOffline  bool
	doctorNoUpsell bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration health and agent status",
	Long: `Runs sanity checks on your gaal configuration:

  - Validates gaal.yaml structure
  - Checks skill source reachability (GitHub repos, local paths)
  - Verifies MCP target files are valid JSON
  - Reports installed agent status
  - Shows telemetry configuration state

Use --offline to skip network checks (skill source reachability).
Use --no-upsell to suppress the Community Edition message.

Exit codes:
  0  all checks passed
  1  warnings found
  2  errors found`,
	SilenceUsage: true,
	RunE:         runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorOffline, "offline", false, "skip network checks (skill source reachability)")
	doctorCmd.Flags().BoolVar(&doctorNoUpsell, "no-upsell", false, "suppress the Community Edition message")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	telemetry.Track("doctor")

	cfg := resolvedCfg
	if cfg == nil {
		cfg = &config.ResolvedConfig{Config: &config.Config{}}
	}

	eng := engine.NewWithOptions(cfg.Config, engineOpts)
	report := eng.Doctor(ops.DoctorOptions{Offline: doctorOffline, ConfigFiles: cfg.SourcePaths(), Levels: cfg.Levels})

	if outputFormat == "json" {
		return renderDoctorJSON(report, !doctorNoUpsell)
	}
	renderDoctorTable(report)
	if !doctorNoUpsell {
		renderCommunityBlock()
	}

	if report.ExitCode != 0 {
		os.Exit(report.ExitCode)
	}
	return nil
}

func renderDoctorTable(report *ops.DoctorReport) {
	sectionDisplay := map[string]string{
		"config":    "Config",
		"telemetry": "Telemetry",
		"skills":    "Skills",
		"mcps":      "MCP",
		"agents":    "Agents",
	}
	sections := []string{"config", "telemetry", "skills", "mcps", "agents"}
	for _, section := range sections {
		var sectionFindings []ops.Finding
		for _, f := range report.Findings {
			if f.Section == section {
				sectionFindings = append(sectionFindings, f)
			}
		}
		if len(sectionFindings) == 0 {
			continue
		}

		title := sectionDisplay[section]
		styled := pterm.NewStyle(pterm.Bold, pterm.FgCyan).Sprintf("── %s ──", title)
		fmt.Printf("\n%s\n", styled)

		for _, f := range sectionFindings {
			var icon string
			switch f.Severity {
			case ops.SeverityInfo:
				icon = pterm.FgGreen.Sprint("✓")
			case ops.SeverityWarning:
				icon = pterm.FgYellow.Sprint("⚠")
			case ops.SeverityError:
				icon = pterm.FgRed.Sprint("✗")
			}
			fmt.Printf("  %s  %s\n", icon, f.Message)
		}
		if section == "config" && len(report.ConfigLevels) > 0 {
			renderConfigLevels(report.ConfigLevels)
		}
	}

	fmt.Println()
	switch report.ExitCode {
	case 0:
		pterm.Success.Println("All checks passed")
	case 1:
		pterm.Warning.Println("Checks completed with warnings")
	case 2:
		pterm.Error.Println("Checks completed with errors")
	}
}

// renderConfigLevels prints a tree of the three configuration levels (global,
// user, workspace) showing, for each level, its file path and a compact
// summary of the entries it defines.
func renderConfigLevels(levels []ops.ConfigLevelSummary) {
	for i, lvl := range levels {
		connector := "├─"
		if i == len(levels)-1 {
			connector = "└─"
		}
		label := pterm.NewStyle(pterm.Bold).Sprintf("%-10s", lvl.Label)
		if !lvl.Loaded {
			fmt.Printf("  %s %s %s\n", connector, label, pterm.FgGray.Sprint("(not found)"))
			continue
		}
		var parts []string
		if lvl.Repos > 0 {
			parts = append(parts, fmt.Sprintf("%d repos", lvl.Repos))
		}
		if lvl.Skills > 0 {
			parts = append(parts, fmt.Sprintf("%d skills", lvl.Skills))
		}
		if lvl.MCPs > 0 {
			parts = append(parts, fmt.Sprintf("%d MCPs", lvl.MCPs))
		}
		var schemaStr string
		if lvl.Schema == nil {
			schemaStr = pterm.FgYellow.Sprint("schema missing")
		} else {
			schemaStr = pterm.FgGreen.Sprintf("schema: %d", *lvl.Schema)
		}
		parts = append(parts, schemaStr)
		pathStr := pterm.FgGray.Sprint(lvl.Path)
		fmt.Printf("  %s %s %s  %s\n", connector, label, pathStr, strings.Join(parts, " · "))
	}
}

func renderDoctorJSON(report *ops.DoctorReport, showUpsell bool) error {
	output := struct {
		Findings []ops.Finding `json:"findings"`
		ExitCode int           `json:"exit_code"`
		Upsell   *string       `json:"upsell,omitempty"`
	}{
		Findings: report.Findings,
		ExitCode: report.ExitCode,
	}
	if showUpsell {
		msg := "When your team needs governance, drift detection, or approvals, see gaal Community Edition: https://getgaal.com"
		output.Upsell = &msg
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return err
	}
	if report.ExitCode != 0 {
		os.Exit(report.ExitCode)
	}
	return nil
}

func renderCommunityBlock() {
	fmt.Println()
	fmt.Println(pterm.FgCyan.Sprint("→ ") + "When your team needs governance, drift detection, or approvals, see")
	fmt.Println("  gaal Community Edition: " + pterm.FgCyan.Sprint("https://getgaal.com"))
}
