//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// ansiEscape is the CSI introducer (ESC + '['). Any pterm-emitted color or
// styling produces a sequence beginning with these two bytes — its presence
// is the simplest TTY-rendering oracle we have without parsing escape grammar.
const ansiEscape = "\x1b["

// TestTTY_ColorEnabledUnderPty allocates a pseudo-TTY for the in-container
// gaal process (`docker exec -t` with TERM=xterm-256color) and asserts that
// pterm-styled output contains ANSI escape sequences. Regression for #149:
// the rest of the suite always runs with TERM=dumb and no -t, so a future
// change that broke pterm's color path under a real TTY would otherwise
// ship green.
func TestTTY_ColorEnabledUnderPty(t *testing.T) {
	env := newTestEnv(t)
	cfgPath := env.writeProjectConfig(t, newConfig().String())
	env.mustGaal(t, cfgPath, "sync")

	// `gaal agents` is a heavy pterm consumer (table, colored installed/source
	// columns) so a TTY-vs-pipe difference is easy to detect.
	gaalEnv := env.gaalEnv()
	gaalEnv["TERM"] = "xterm-256color"
	res := env.c.ExecTTY(t, gaalEnv, env.workdir, "gaal", "-c", cfgPath, "agents")
	if res.ExitCode != 0 {
		t.Fatalf("gaal agents under pty failed: exit=%d\n%s",
			res.ExitCode, res.Stdout)
	}
	if !strings.Contains(res.Stdout, ansiEscape) {
		t.Errorf("expected ANSI escape (%q) in pty-allocated output; got:\n%q",
			ansiEscape, res.Stdout)
	}
}

// TestTTY_ColorDisabledWithoutPty is the inverse: same command, no -t, the
// suite-default TERM=dumb. Output must NOT contain ANSI escapes — pterm
// auto-detects the absence of a TTY and falls back to plain text.
func TestTTY_ColorDisabledWithoutPty(t *testing.T) {
	env := newTestEnv(t)
	cfgPath := env.writeProjectConfig(t, newConfig().String())
	env.mustGaal(t, cfgPath, "sync")

	res := env.mustGaal(t, cfgPath, "agents")
	if strings.Contains(res.Stdout, ansiEscape) {
		t.Errorf("expected no ANSI escape (%q) in non-TTY output; got:\n%q",
			ansiEscape, res.Stdout)
	}
}

// TestTTY_VerboseProducesMoreThanSummary validates the summary-first
// contract from #107: the default text output is more compact than the
// --verbose flavour. Asserts on byte length to avoid coupling to specific
// line content.
func TestTTY_VerboseProducesMoreThanSummary(t *testing.T) {
	env := newTestEnv(t)
	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	summary := env.mustGaal(t, cfgPath, "status")
	verbose := env.mustGaal(t, cfgPath, "-v", "status")

	if len(verbose.Stdout) <= len(summary.Stdout) {
		t.Errorf("expected verbose status to be longer than summary status\n"+
			"summary (%d bytes):\n%s\nverbose (%d bytes):\n%s",
			len(summary.Stdout), summary.Stdout,
			len(verbose.Stdout), verbose.Stdout)
	}
}
