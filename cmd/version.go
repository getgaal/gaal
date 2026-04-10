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
	Use:   "version",
	Short: "Print version information",
	RunE: func(_ *cobra.Command, _ []string) error {
		if outputFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(struct {
				Version   string `json:"version"`
				BuildTime string `json:"build_time"`
			}{Version, BuildTime})
		}
		fmt.Printf("gaal %s (built: %s)\n", Version, BuildTime)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
