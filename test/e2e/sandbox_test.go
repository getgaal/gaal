//go:build e2e

// Coverage for the --sandbox flag — the documented "safe to try gaal"
// entry point that redirects HOME, XDG_CONFIG_HOME, XDG_CACHE_HOME (and
// USERPROFILE / APPDATA / LOCALAPPDATA on Windows) so nothing escapes the
// requested directory. A regression here would silently write to the real
// user HOME on first try.
//
// This test completes the leftover scope of #119 (issue #119 / PR #158)
// which only fixed the os.Getwd() leak in countInstalledAgents — the
// end-to-end "sandbox really redirects everything" assertion lives here.
package e2e

import (
	"path"
	"strings"
	"testing"
)

// TestSandbox_RedirectsAllWrites runs `gaal --sandbox /sb sync` against a
// global-scope skill and asserts:
//   - the synced skill landed UNDER /sb (specifically /sb/.claude/skills/...)
//   - nothing was written under the real test HOME
//   - the suite's home dir is unchanged
func TestSandbox_RedirectsAllWrites(t *testing.T) {
	env := newTestEnv(t)

	// The sandbox dir is its own freshly-allocated path inside the
	// container — distinct from env.home so we can prove writes do not
	// land at env.home.
	sandbox := env.home + "-sandbox"
	env.c.MustExec(t, nil, "", "mkdir", "-p", sandbox)
	t.Cleanup(func() { env.c.Exec(t, nil, "", "rm", "-rf", sandbox) })

	// Global-scope skill so the install would normally land at $HOME/.claude/.
	// With --sandbox /sb, it must land at /sb/.claude/ instead.
	cfg := newConfig().
		AddSkill(localSkillsRoot+"/stub-skill", []string{"claude-code"}, true).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	// Snapshot the real HOME entries BEFORE sync so we can detect any leak.
	homeBefore := env.c.ListDir(t, env.home)
	homeBeforeSet := make(map[string]struct{}, len(homeBefore))
	for _, e := range homeBefore {
		homeBeforeSet[e] = struct{}{}
	}

	full := []string{"gaal", "--no-banner", "--sandbox", sandbox, "-c", cfgPath, "sync"}
	res := env.c.Exec(t, env.gaalEnv(), env.workdir, full...)
	if res.ExitCode != 0 {
		t.Fatalf("gaal --sandbox sync failed: exit=%d\n%s", res.ExitCode, res.Combined())
	}

	// 1. Skill must exist under the sandbox dir.
	sandboxSkillDir := path.Join(sandbox, ".claude", "skills", "stub-skill")
	if !env.c.IsDir(t, sandboxSkillDir) {
		t.Errorf("expected synced skill under sandbox at %s; not found", sandboxSkillDir)
	}

	// 2. Real HOME must not have grown a .claude dir as a side effect of
	//    the sandboxed sync.
	homeAfter := env.c.ListDir(t, env.home)
	for _, e := range homeAfter {
		if _, existed := homeBeforeSet[e]; existed {
			continue
		}
		// New entry under HOME after sandboxed sync.
		// Allow gaal.yaml-adjacent entries the test itself may have placed,
		// but anything looking like an agent cache/skill dir is a leak.
		if strings.HasPrefix(e, ".") {
			t.Errorf("sandboxed sync leaked into real HOME: new entry %s\n  full HOME listing: %v",
				e, homeAfter)
		}
	}

	// 3. The sandbox dir itself MUST contain at least the .claude entry —
	//    sanity check that the sandbox redirection actually applied (not
	//    just "no leak because nothing happened").
	sbEntries := env.c.ListDir(t, sandbox)
	foundClaude := false
	for _, e := range sbEntries {
		if e == ".claude" {
			foundClaude = true
			break
		}
	}
	if !foundClaude {
		t.Errorf("sandbox dir %s did not receive .claude/; entries: %v",
			sandbox, sbEntries)
	}
}
