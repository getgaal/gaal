//go:build e2e

package e2e

import (
	"path"
	"strings"
	"testing"
)

// requireCLILayer skips the test when the heavy CLI verification layer is
// not enabled. The image build skips installing the agent CLIs unless
// GAAL_E2E_CLI=1, so trying to invoke `claude` or `codex` would only fail
// in an opaque "not found" — better to skip cleanly.
func requireCLILayer(t *testing.T) {
	t.Helper()
	if !envCLI {
		t.Skip("CLI verification layer disabled (set GAAL_E2E_CLI=1 to enable)")
	}
}

// TestCLI_ClaudeMCPListShowsSyncedServer is the classic "round-trip" test:
// gaal writes the entry, the real claude CLI reads it back, the entry
// must appear in `claude mcp list` output.
func TestCLI_ClaudeMCPListShowsSyncedServer(t *testing.T) {
	requireCLILayer(t)
	env := newTestEnv(t)

	cfg := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := suite.MustExec(t, env.gaalEnv(), env.workdir, "claude", "mcp", "list")
	if !strings.Contains(res.Stdout+res.Stderr, "filesystem") {
		t.Fatalf("expected `claude mcp list` to mention the synced server\n%s",
			res.Combined())
	}
}

// TestCLI_CodexReadsTOMLWritten verifies the TOML codex writes is
// re-parseable by the codex CLI itself. We don't assume which codex
// subcommand surfaces MCP servers — version drift makes that brittle —
// so we use `codex --help` as a smoke check that the binary still runs
// against the gaal-written config and exits 0. A broken TOML would make
// codex bail out on startup.
func TestCLI_CodexReadsTOMLWritten(t *testing.T) {
	requireCLILayer(t)
	env := newTestEnv(t)
	env.c.MustExec(t, nil, "", "mkdir", "-p", path.Join(env.home, ".codex"))

	cfg := newConfig().
		AddMCP("git", []string{"codex"}, true,
			"uvx", []string{"mcp-server-git"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	res := suite.Exec(t, env.gaalEnv(), env.workdir, "codex", "--help")
	if res.ExitCode != 0 {
		t.Fatalf("codex --help failed after gaal-written config: exit=%d\n%s",
			res.ExitCode, res.Combined())
	}
}

// TestCLI_PruneRemovesEntryFromCLIView is the destructive twin of
// TestCLI_ClaudeMCPListShowsSyncedServer: after a sync --prune drops the
// entry, `claude mcp list` must no longer show it.
func TestCLI_PruneRemovesEntryFromCLIView(t *testing.T) {
	requireCLILayer(t)
	env := newTestEnv(t)

	cfg1 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		AddMCP("git", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-git"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg1)
	env.mustGaal(t, cfgPath, "sync")

	cfg2 := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	env.c.WriteFile(t, cfgPath, cfg2)
	env.mustGaal(t, cfgPath, "sync", "--prune")

	res := suite.MustExec(t, env.gaalEnv(), env.workdir, "claude", "mcp", "list")
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "filesystem") {
		t.Fatalf("expected `claude mcp list` to still mention filesystem\n%s",
			res.Combined())
	}
	if strings.Contains(combined, "git") {
		t.Fatalf("expected `claude mcp list` to no longer mention git after prune\n%s",
			res.Combined())
	}
}
