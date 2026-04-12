package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"gaal/cmd/internal/wizard"
	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/engine/ops"
	"gaal/internal/telemetry"
)

var (
	forceInit     bool
	initScopeFlag string
	initImportAll bool
	initEmpty     bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap a gaal.yaml from the results of audit",
	Long: `Creates a gaal.yaml populated from the skills and MCP servers discovered on
this machine.

The command first asks whether the new configuration is project-scoped
(./gaal.yaml) or global-scoped (~/.config/gaal/config.yaml). It then runs an
audit and presents an interactive multi-select list, grouped by agent, of
everything it found under that scope. The generated file reflects your
selection and is ready for "gaal sync".

Use --scope and --import-all to run non-interactively (for CI or scripts).
Use --force to overwrite an existing configuration file.`,
	SilenceUsage: true,
	RunE:         runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "overwrite an existing configuration file")
	initCmd.Flags().StringVar(&initScopeFlag, "scope", "", `pre-select the scope without prompting ("project" or "global")`)
	initCmd.Flags().BoolVar(&initImportAll, "import-all", false, "non-interactive: import every detected skill and MCP")
	initCmd.Flags().BoolVar(&initEmpty, "empty", false, "non-interactive: write the documented empty skeleton")
	initCmd.MarkFlagsMutuallyExclusive("import-all", "empty")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	telemetry.Track("init")

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))

	mode, err := resolveInitMode(initEmpty, initImportAll, isTTY)
	if err != nil {
		return err
	}

	scope, err := resolveInitScope(initScopeFlag, isTTY)
	if err != nil {
		return err
	}

	dest, err := resolveInitDestination(scope)
	if err != nil {
		return err
	}

	if err := preflightDestination(dest, forceInit); err != nil {
		return err
	}

	eng := engine.NewWithOptions(&config.Config{}, engineOpts)

	if mode == wizard.ModeEmpty {
		if err := eng.Init(dest, forceInit); err != nil {
			return err
		}
		pterm.Success.Printfln("Created %s", dest)
		printNextSteps(dest)
		return nil
	}

	candidates, err := eng.BuildImportCandidates(ctx, scope)
	if err != nil {
		return fmt.Errorf("building import candidates: %w", err)
	}

	if len(candidates.Sections) == 0 {
		if err := eng.Init(dest, forceInit); err != nil {
			return err
		}
		pterm.Info.Printfln("No installed skills or MCP servers detected under %s scope.", scope)
		pterm.Info.Printfln("Wrote empty skeleton to %s", dest)
		printNextSteps(dest)
		return nil
	}

	plan, err := selectPlan(candidates, scope, isTTY, initImportAll)
	if err != nil {
		return err
	}

	pterm.DefaultBox.WithTitle("Preview").Println(previewText(dest, plan))

	if isTTY && !initImportAll {
		confirmed, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(true).
			WithDefaultText("Proceed?").
			Show()
		if err != nil {
			return fmt.Errorf("confirmation prompt: %w", err)
		}
		if !confirmed {
			pterm.Warning.Println("init cancelled")
			return errors.New("init cancelled")
		}
	}

	if err := eng.InitFromPlan(dest, plan, forceInit); err != nil {
		return err
	}
	pterm.Success.Printfln("Created %s", dest)
	if len(plan.Skills) > 0 {
		pterm.Info.Println(`Tip: each skill entry targets a single agent. To sync a skill to every installed agent, edit its "agents:" field to ["*"].`)
	}
	printNextSteps(dest)
	return nil
}

// resolveInitMode decides whether the wizard imports the audit result or
// writes an empty skeleton.
//
// Precedence: explicit flag (--empty or --import-all) > interactive prompt >
// error (non-TTY without a flag).
func resolveInitMode(empty, importAll, isTTY bool) (wizard.Mode, error) {
	switch {
	case empty:
		return wizard.ModeEmpty, nil
	case importAll:
		return wizard.ModeImport, nil
	}
	if !isTTY {
		return "", errors.New("init requires an interactive terminal, --empty, or --import-all")
	}
	return wizard.SelectMode()
}

// resolveInitScope decides which scope the wizard runs under.
//
// Precedence: --scope flag > interactive prompt > error.
func resolveInitScope(flagValue string, isTTY bool) (ops.Scope, error) {
	if flagValue != "" {
		switch strings.ToLower(flagValue) {
		case "project":
			return ops.ScopeProject, nil
		case "global":
			return ops.ScopeGlobal, nil
		default:
			return "", fmt.Errorf(`invalid --scope %q: must be "project" or "global"`, flagValue)
		}
	}
	if !isTTY {
		return "", errors.New("init requires an interactive terminal or --scope")
	}
	return wizard.SelectScope("./gaal.yaml", config.UserConfigFilePath())
}

// resolveInitDestination returns the file path to write.
//
// Precedence: explicit -c/--config (when the user passed it) > scope-specific
// default. When the user supplied an explicit config path, it is honoured as
// is, but a warning is logged if it looks inconsistent with the chosen scope.
func resolveInitDestination(scope ops.Scope) (string, error) {
	if cfgFile == "gaal.yaml" {
		if scope == ops.ScopeGlobal {
			return config.UserConfigFilePath(), nil
		}
		return "gaal.yaml", nil
	}

	switch scope {
	case ops.ScopeProject:
		if strings.HasPrefix(cfgFile, "~/") || strings.HasPrefix(cfgFile, `~\`) {
			slog.Warn("config path looks home-relative but scope is project", "path", cfgFile)
		}
	case ops.ScopeGlobal:
		if !strings.Contains(cfgFile, string(os.PathSeparator)) {
			slog.Warn("config path looks project-relative but scope is global", "path", cfgFile)
		}
	}
	return cfgFile, nil
}

func preflightDestination(dest string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists — use --force to overwrite", dest)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", dest, err)
	}
	return nil
}

func selectPlan(c ops.Candidates, scope ops.Scope, isTTY, importAll bool) (ops.Plan, error) {
	if importAll {
		flat := flattenAll(c)
		return ops.BuildPlan(flat, scope), nil
	}
	if !isTTY {
		return ops.Plan{}, errors.New("init requires --import-all in non-interactive mode")
	}
	plan, err := wizard.SelectSections(c, scope)
	if err != nil {
		if errors.Is(err, wizard.ErrCancelled) {
			return ops.Plan{}, errors.New("init cancelled")
		}
		return ops.Plan{}, err
	}
	return plan, nil
}

func flattenAll(c ops.Candidates) []ops.Candidate {
	var out []ops.Candidate
	for _, sec := range c.Sections {
		out = append(out, sec.Skills...)
		out = append(out, sec.MCPs...)
	}
	return out
}

func previewText(dest string, plan ops.Plan) string {
	agents := map[string]struct{}{}
	for _, s := range plan.Skills {
		for _, a := range s.Agents {
			agents[a] = struct{}{}
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Will write %s with:\n", dest)
	fmt.Fprintf(&sb, "  skills: %d %s (%d %s)\n",
		len(plan.Skills), pluralize(len(plan.Skills), "entry", "entries"),
		len(agents), pluralize(len(agents), "agent", "agents"))
	fmt.Fprintf(&sb, "  mcps:   %d %s\n",
		len(plan.MCPs), pluralize(len(plan.MCPs), "entry", "entries"))
	sb.WriteString("  repositories: empty")
	return sb.String()
}

func pluralize(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func printNextSteps(dest string) {
	fmt.Printf("\nNext steps:\n  1. Review %s\n  2. Run: gaal sync\n  3. Run: gaal status\n", dest)
}
