package cmd

import (
	"fmt"
	"os"
)

// asciiArt is the GAAL banner printed on interactive invocations.
const asciiArt = "" +
	"  ██████╗  █████╗  █████╗ ██╗     \n" +
	" ██╔════╝ ██╔══██╗██╔══██╗██║     \n" +
	" ██║  ███╗███████║███████║██║     \n" +
	" ██║   ██║██╔══██║██╔══██║██║     \n" +
	" ╚██████╔╝██║  ██║██║  ██║███████╗\n" +
	"  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝\n"

const (
	ansiCyan  = "\033[36m"
	ansiDim   = "\033[2m"
	ansiReset = "\033[0m"
)

// printBanner writes the GAAL ASCII-art header to stdout.
// It is a no-op when stdout is not a TTY (pipes, redirections, CI).
func printBanner() {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Print(ansiCyan + asciiArt + ansiReset)
	fmt.Printf(ansiDim+"  Repository · Skills · MCP — %s\n\n"+ansiReset, Version)
}
