package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"gaal/internal/config"
)

var schemaFile string

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the JSON Schema for the gaal configuration file",
	Long: `Generate and print the JSON Schema (draft-07) that describes the structure of
a gaal YAML configuration file.

The schema can be used by IDEs (GoLand, VS Code via yaml.schemas) and LLMs
to validate and auto-complete configuration files.

Examples:

  # Print to stdout
  gaal schema

  # Write to a file for IDE integration
  gaal schema -f schema.json`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		slog.Debug("generating config JSON Schema")
		data, err := config.GenerateSchema()
		if err != nil {
			return fmt.Errorf("generating schema: %w", err)
		}

		if schemaFile == "" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		}

		slog.Debug("writing schema to file", "path", schemaFile)
		if err := os.WriteFile(schemaFile, data, 0o644); err != nil {
			return fmt.Errorf("writing schema to %q: %w", schemaFile, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Schema written to %s\n", schemaFile)
		return nil
	},
}

func init() {
	schemaCmd.Flags().StringVarP(&schemaFile, "file", "f", "", "Write schema to `FILE` instead of stdout")
	rootCmd.AddCommand(schemaCmd)
}
