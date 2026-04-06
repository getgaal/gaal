package cmd

import (
	"fmt"

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
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("gaal %s (built: %s)\n", Version, BuildTime)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
