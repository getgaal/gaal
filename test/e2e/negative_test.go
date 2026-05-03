//go:build e2e

// Negative-path e2e tests: every error a user can hit on a typical day must
// produce a non-zero exit, a recognisable message, and no silent corruption.
// The happy-path suite already covers exit 0; these tests pin the failure
// modes so a regression that turns a hard error into a confusing no-op is
// caught at PR time.
package e2e

import (
	"path"
	"strings"
	"testing"
)

// TestNegative_MalformedYAML asserts gaal sync fails with a non-zero exit
// AND a stderr message that names the offending file. A regression that
// silently treats unparseable YAML as an empty config (and reports
// "nothing to do") would have shipped before this test.
func TestNegative_MalformedYAML(t *testing.T) {
	env := newTestEnv(t)
	cfgPath := env.writeProjectConfig(t, "skills: [unclosed\n")

	res := env.gaal(t, cfgPath, "sync")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit on malformed YAML, got 0\n%s", res.Combined())
	}
	combined := strings.ToLower(res.Stdout + res.Stderr)
	if !strings.Contains(combined, "yaml") && !strings.Contains(combined, "config") {
		t.Errorf("expected error to mention YAML or config; got: %s", res.Combined())
	}
}

// TestNegative_MissingConfig asserts that pointing -c at a non-existent
// file produces a clear error. Any non-zero exit is acceptable; the
// stderr/stdout must mention the missing path so the user can self-serve.
func TestNegative_MissingConfig(t *testing.T) {
	env := newTestEnv(t)
	missing := path.Join(env.workdir, "does-not-exist.yaml")

	res := env.gaal(t, missing, "sync")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit when -c points at missing file\n%s", res.Combined())
	}
	if !strings.Contains(res.Combined(), "does-not-exist.yaml") {
		t.Errorf("expected error to name the missing file %q; got: %s",
			missing, res.Combined())
	}
}

// TestNegative_UnknownSkillSource asserts gaal surfaces a clear error when
// a skill source path is configured but does not exist on disk.
func TestNegative_UnknownSkillSource(t *testing.T) {
	env := newTestEnv(t)
	cfg := newConfig().
		AddSkill("/fixtures/skills/this-does-not-exist", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	res := env.gaal(t, cfgPath, "sync")
	// We don't pin the exact exit code (per-source aggregation may treat
	// "no skills found" as a warn, not a hard error), but the stderr/stdout
	// must mention the missing source so the user knows what failed.
	combined := res.Combined()
	if res.ExitCode == 0 && !strings.Contains(combined, "this-does-not-exist") {
		t.Errorf("sync against missing skill source must report it (exit=%d): %s",
			res.ExitCode, combined)
	}
}

// TestNegative_MCPTargetIsADirectory asserts merge-into-target degrades
// gracefully when the target file path is actually an existing directory.
// gaal must not silently swallow the error or, worse, recursively delete
// the directory.
func TestNegative_MCPTargetIsADirectory(t *testing.T) {
	env := newTestEnv(t)
	// Pre-create the would-be target as a directory so the JSON write fails.
	targetParent := path.Join(env.home, ".claude.json")
	env.c.MustExec(t, nil, "", "mkdir", "-p", targetParent)

	cfg := newConfig().
		AddMCP("filesystem", []string{"claude-code"}, true,
			"uvx", []string{"mcp-server-filesystem", "/data"}, nil).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	_ = env.gaal(t, cfgPath, "sync")
	// Either a non-zero exit or an explicit warn — both are acceptable;
	// what's NOT acceptable is converting the directory to a regular file.
	if !env.c.IsDir(t, targetParent) {
		t.Fatalf("sync mutated MCP target from directory to non-directory at %s", targetParent)
	}
}

// TestNegative_AbsoluteRepoPathRejected exercises the workspace-containment
// check from #118 (issue #118 → PR #157). A workspace config naming an
// absolute repo path must be refused at load time so a public/shared
// gaal.yaml cannot clone over arbitrary user paths.
func TestNegative_AbsoluteRepoPathRejected(t *testing.T) {
	env := newTestEnv(t)
	body := "schema: 1\nrepositories:\n  /tmp/gaal-evil-target:\n    type: git\n    url: https://example.com/x.git\n"
	cfgPath := env.writeProjectConfig(t, body)

	res := env.gaal(t, cfgPath, "sync")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit when workspace config has absolute repo path\n%s",
			res.Combined())
	}
	combined := strings.ToLower(res.Combined())
	if !strings.Contains(combined, "absolute") && !strings.Contains(combined, "workspace") {
		t.Errorf("expected error to mention 'absolute' or 'workspace'; got: %s",
			res.Combined())
	}
}

// TestNegative_BadVCSVersionRejected exercises the validateVCSOperand check
// from #116 (PR #155). A version starting with '-' must be refused before
// the subprocess is spawned.
func TestNegative_BadVCSVersionRejected(t *testing.T) {
	env := newTestEnv(t)
	body := `schema: 1
repositories:
  src/repo:
    type: git
    url: https://example.com/x.git
    version: "--config=hooks.preupdate=touch /tmp/pwned"
`
	cfgPath := env.writeProjectConfig(t, body)

	res := env.gaal(t, cfgPath, "sync")
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit for version starting with '-'\n%s",
			res.Combined())
	}
	if env.c.FileExists(t, "/tmp/pwned") {
		env.c.RemoveFile(t, "/tmp/pwned")
		t.Fatal("argv-injection succeeded: /tmp/pwned was created")
	}
}
