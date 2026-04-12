package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"gaal/internal/config"
	"gaal/internal/engine"
	"gaal/internal/logger"
	"gaal/internal/telemetry"
)

var (
	cfgFile      string
	verbose      bool
	noBanner     bool
	sandboxDir   string
	logFile      string
	outputFormat string

	// engineOpts is populated by PersistentPreRunE and shared by all sub-commands.
	engineOpts engine.Options
)

var rootCmd = &cobra.Command{
	Use:   "gaal",
	Short: "Multi-protocol local repository and skill/MCP manager",
	Long: `gaal maintains a local base of multi-protocol repositories,
installs agent skills (SKILL.md collections) and manages MCP server
configurations.

Run once (one-shot mode) or continuously as a service with --service.`,
	SilenceUsage: true,
	// PersistentPreRunE runs before every sub-command (sync, status, …) and
	// before RunE on the root command itself. It is the single place where the
	// logger, banner and sandbox are initialised so no sub-command needs to repeat it.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// The built-in `completion` sub-commands produce pure shell script on stdout.
		// Any extra output (banner, logs) would corrupt the script and make it
		// unsourceable, so we skip all initialisation for them.
		if cmd.HasParent() && cmd.Parent().Name() == "completion" {
			return nil
		}
		if !noBanner && outputFormat != "json" {
			printBanner()
		}
		if err := setupLogger(); err != nil {
			return err
		}
		opts, err := applyOptions()
		if err != nil {
			return err
		}
		engineOpts = opts

		// Telemetry: resolve consent state and initialise.
		if !skipTelemetry(cmd) {
			userCfg := loadMergedTelemetryConfig()
			var promptFn func() (bool, error)
			if term.IsTerminal(int(os.Stdin.Fd())) {
				promptFn = showConsentPrompt
			}
			if _, err := telemetry.Init(userCfg, promptFn, Version); err != nil {
				slog.Debug("telemetry init failed", "err", err)
			}
		}

		return nil
	},
	// PersistentPostRunE runs after every sub-command. Waits briefly for
	// in-flight telemetry events to complete before the process exits.
	PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
		telemetry.Shutdown()
		return nil
	},
	// No RunE: invoking gaal without a sub-command prints the banner (via
	// PersistentPreRunE) then lists the available commands.
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

// Execute is the entry-point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		telemetry.Shutdown()
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "gaal.yaml", "configuration file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&noBanner, "no-banner", false, "suppress the ASCII-art banner")
	rootCmd.PersistentFlags().StringVar(&sandboxDir, "sandbox", "", "redirect all writes to this directory (safe for tests)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "write structured JSON logs to this file (in addition to console)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format: table, json")
}

// applyOptions builds engine.Options, applying sandbox mode when requested.
// When --sandbox is set, gaal rewrites the relevant user-directory environment
// variables so that config loading, skill dirs, caches and MCP targets stay
// inside the sandbox. Nothing outside the sandbox directory is touched.
func applyOptions() (engine.Options, error) {
	if sandboxDir == "" {
		return engine.Options{}, nil
	}

	workDir := filepath.Join(sandboxDir, "workspace")
	configDir := filepath.Join(sandboxDir, ".config")
	cacheDir := filepath.Join(sandboxDir, ".cache")
	roamingAppDataDir := filepath.Join(sandboxDir, "AppData", "Roaming")
	localAppDataDir := filepath.Join(sandboxDir, "AppData", "Local")
	dirs := []string{sandboxDir, workDir, configDir, cacheDir}
	if runtime.GOOS == "windows" {
		dirs = append(dirs, roamingAppDataDir, localAppDataDir)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return engine.Options{}, fmt.Errorf("creating sandbox directory %q: %w", d, err)
		}
	}

	// Redirect the OS-specific user directory environment variables so that all
	// gaal-managed paths resolve inside the sandbox.
	if err := os.Setenv("HOME", sandboxDir); err != nil {
		return engine.Options{}, fmt.Errorf("setting sandbox HOME: %w", err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Setenv("USERPROFILE", sandboxDir); err != nil {
			return engine.Options{}, fmt.Errorf("setting sandbox USERPROFILE: %w", err)
		}
		if err := os.Setenv("APPDATA", roamingAppDataDir); err != nil {
			return engine.Options{}, fmt.Errorf("setting sandbox APPDATA: %w", err)
		}
		if err := os.Setenv("LOCALAPPDATA", localAppDataDir); err != nil {
			return engine.Options{}, fmt.Errorf("setting sandbox LOCALAPPDATA: %w", err)
		}
	} else {
		if err := os.Setenv("XDG_CONFIG_HOME", configDir); err != nil {
			return engine.Options{}, fmt.Errorf("setting sandbox XDG_CONFIG_HOME: %w", err)
		}
		if err := os.Setenv("XDG_CACHE_HOME", cacheDir); err != nil {
			return engine.Options{}, fmt.Errorf("setting sandbox XDG_CACHE_HOME: %w", err)
		}
	}

	slog.Info("sandbox mode active", "home", sandboxDir, "workspace", workDir, "configDir", configDir, "cacheDir", cacheDir)
	return engine.Options{WorkDir: workDir}, nil
}

// setupLogger initialises the global logger using the package-level flags.
// Console output is always active (colored when attached to a TTY).
// When --log-file is set, a JSON handler is added that writes to that file.
func setupLogger() error {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	_, err := logger.Setup(level, logFile)
	return err
}

// skipTelemetry returns true for commands that should never trigger
// telemetry or the consent prompt.
func skipTelemetry(cmd *cobra.Command) bool {
	name := cmd.Name()
	return name == "version" || name == "schema" ||
		(cmd.HasParent() && cmd.Parent().Name() == "completion")
}

// loadMergedTelemetryConfig reads the telemetry field from the merged config
// chain (global -> user -> workspace). Errors are silently ignored; the
// caller treats nil as "no consent recorded".
func loadMergedTelemetryConfig() *bool {
	cfg, err := config.LoadChain(cfgFile)
	if err != nil {
		return nil
	}
	return cfg.Telemetry
}

// showConsentPrompt displays the opt-in telemetry prompt.
func showConsentPrompt() (bool, error) {
	fmt.Println()
	fmt.Println("gaal can send anonymous usage pings to help us understand adoption.")
	fmt.Println("No config contents, file paths, or identifiers are ever sent.")
	fmt.Println("See PRIVACY_POLICY.md for details.")
	fmt.Println()

	result, err := pterm.DefaultInteractiveConfirm.
		WithDefaultValue(false).
		WithDefaultText("Enable anonymous telemetry?").
		Show()
	if err != nil {
		return false, err
	}
	fmt.Println()
	return result, nil
}
