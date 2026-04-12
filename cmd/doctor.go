package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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

	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		telemetry.TrackError("doctor", err)
		cfg = &config.Config{}
	}

	// Telemetry field is excluded from config merging, so read it
	// directly from the user config file.
	cfg.Telemetry = loadUserTelemetryConfig()

	eng := engine.NewWithOptions(cfg, engineOpts)
	report := eng.Doctor(ops.DoctorOptions{Offline: doctorOffline})

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
		"telemetry": "Telemetry",
		"skills":    "Skills",
		"mcps":      "MCP",
		"agents":    "Agents",
	}
	sections := []string{"telemetry", "skills", "mcps", "agents"}
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
