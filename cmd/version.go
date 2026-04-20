package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// These variables are optionally injected at build time via ldflags:
//
//	go build -ldflags "-X gaal/cmd.Version=1.0.0 -X gaal/cmd.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var versionCmd = &cobra.Command{
	Use:         "version",
	Short:       "Print version information",
	Annotations: map[string]string{"config": "optional"},
	RunE: func(_ *cobra.Command, _ []string) error {
		if outputFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(struct {
				Version string `json:"version"`
				Built   string `json:"built"`
			}{Version, BuildTime})
		}
		fmt.Printf("gaal %s\nbuilt %s\n", Version, BuildTime)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
