//go:build e2e

// Hermetic VCS coverage — issue #143.
//
// Until this file landed the e2e suite never exercised the actual VCS
// backends (git via go-git, hg/svn/bzr/tar/zip via subprocess). Every
// `repositories:` entry the suite ever wrote was a no-op because nothing
// in the test configs used one. A regression in internal/core/vcs/git.go
// (broken ref resolution, wrong checksum, partial clone recovery) would
// ship green.
//
// This file covers the **git** backend hermetically: it bootstraps a bare
// repo inside the container, clones from it via the gaal git backend, and
// verifies update-after-upstream-change. The hg/svn/bzr/tar/zip backends
// require additional packages and a HTTP fileserver; tracked as follow-up
// scope under #143.
package e2e

import (
	"path"
	"testing"
)

// gitConfigEnv returns the env vars needed to make `git commit` work in a
// fresh container — git complains without a configured user.
var gitConfigEnv = Env{
	"GIT_AUTHOR_NAME":     "gaal-e2e",
	"GIT_AUTHOR_EMAIL":    "e2e@example.test",
	"GIT_COMMITTER_NAME":  "gaal-e2e",
	"GIT_COMMITTER_EMAIL": "e2e@example.test",
}

// initBareGitRepo creates a bare git repo at <root>/<name>.git with one
// commit (a README) so it can be cloned. Returns the absolute path to the
// bare repo, suitable for use as a gaal repository url.
func initBareGitRepo(t *testing.T, env *testEnv, root, name string) string {
	t.Helper()
	bare := path.Join(root, name+".git")
	work := path.Join(root, name+"-work")

	env.c.MustExec(t, nil, "", "git", "init", "--bare", bare)
	env.c.MustExec(t, nil, "", "git", "init", work)
	env.c.WriteFile(t, path.Join(work, "README.md"), "# initial\n")
	env.c.MustExec(t, gitConfigEnv, work, "git", "add", "README.md")
	env.c.MustExec(t, gitConfigEnv, work, "git", "commit", "-m", "initial")
	// Default branch name varies across git versions — force "main".
	env.c.MustExec(t, gitConfigEnv, work, "git", "branch", "-M", "main")
	env.c.MustExec(t, gitConfigEnv, work, "git", "remote", "add", "origin", bare)
	env.c.MustExec(t, gitConfigEnv, work, "git", "push", "-u", "origin", "main")
	// Re-point HEAD on the bare repo so go-git's PlainClone resolves the
	// default branch. `git init --bare` defaults HEAD to whatever
	// init.defaultBranch is (often "master") which leaves it unborn after
	// pushing "main"; clone then fails with "reference not found".
	env.c.MustExec(t, nil, "", "git", "--git-dir", bare,
		"symbolic-ref", "HEAD", "refs/heads/main")
	return bare
}

// addCommitToBare makes a new commit on the bare repo's main branch via a
// fresh worktree. Used to test update-after-upstream-change. Returns the
// short hash of the new commit.
func addCommitToBare(t *testing.T, env *testEnv, bare, fileName, content string) {
	t.Helper()
	work := bare + "-update-work"
	env.c.MustExec(t, nil, "", "rm", "-rf", work)
	env.c.MustExec(t, nil, "", "git", "clone", bare, work)
	env.c.WriteFile(t, path.Join(work, fileName), content)
	env.c.MustExec(t, gitConfigEnv, work, "git", "add", fileName)
	env.c.MustExec(t, gitConfigEnv, work, "git", "commit", "-m", "add "+fileName)
	env.c.MustExec(t, gitConfigEnv, work, "git", "push", "origin", "main")
}

// TestVCS_GitBackend_CloneAndCheckout asserts gaal can clone a hermetic
// bare repo via the git backend (go-git, no binary required) and lay out
// the repository at the configured path.
func TestVCS_GitBackend_CloneAndCheckout(t *testing.T) {
	env := newTestEnv(t)

	reposRoot := path.Join(env.home, "test-repos")
	env.c.MustExec(t, nil, "", "mkdir", "-p", reposRoot)
	bare := initBareGitRepo(t, env, reposRoot, "myrepo")

	cfg := newConfig().
		AddRepository("src/myrepo", "git", bare, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	// The README from the initial commit must exist at the expected path.
	dst := path.Join(env.workdir, "src", "myrepo", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected cloned README at %s after sync", dst)
	}
	content := env.c.ReadFile(t, dst)
	if content != "# initial\n" {
		t.Errorf("README content mismatch: got %q", content)
	}
}

// TestVCS_GitBackend_UpdateAfterUpstreamChange asserts a second sync picks
// up a new upstream commit. Regression target for the Update path —
// initial Clone is the easy case; making sure subsequent Updates actually
// pull is the surface that historically breaks (#125).
func TestVCS_GitBackend_UpdateAfterUpstreamChange(t *testing.T) {
	env := newTestEnv(t)

	reposRoot := path.Join(env.home, "test-repos")
	env.c.MustExec(t, nil, "", "mkdir", "-p", reposRoot)
	bare := initBareGitRepo(t, env, reposRoot, "updaterepo")

	cfg := newConfig().
		AddRepository("src/updaterepo", "git", bare, "").
		String()
	cfgPath := env.writeProjectConfig(t, cfg)

	env.mustGaal(t, cfgPath, "sync")

	// Push a new commit to the bare repo and re-sync.
	addCommitToBare(t, env, bare, "NEWFILE.md", "added later\n")
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "updaterepo", "NEWFILE.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("expected NEWFILE.md after second sync (update path); not found")
	}
	if got := env.c.ReadFile(t, dst); got != "added later\n" {
		t.Errorf("NEWFILE.md content mismatch: got %q", got)
	}
}

// TestVCS_GitBackend_VersionPin clones the repo at a specific commit hash
// and asserts only the pinned commit's content is present (the later one
// is not). Pins #125's "version-aware checkout" half.
func TestVCS_GitBackend_VersionPin(t *testing.T) {
	env := newTestEnv(t)

	reposRoot := path.Join(env.home, "test-repos")
	env.c.MustExec(t, nil, "", "mkdir", "-p", reposRoot)
	bare := initBareGitRepo(t, env, reposRoot, "pinrepo")

	// Capture the initial commit hash before adding more.
	initial := env.c.MustExec(t, nil, "",
		"git", "--git-dir", bare, "rev-parse", "HEAD").Stdout
	initial = trimNewline(initial)

	addCommitToBare(t, env, bare, "LATER.md", "later commit\n")

	cfg := newConfig().
		AddRepository("src/pinrepo", "git", bare, initial).
		String()
	cfgPath := env.writeProjectConfig(t, cfg)
	env.mustGaal(t, cfgPath, "sync")

	dst := path.Join(env.workdir, "src", "pinrepo", "README.md")
	if !env.c.FileExists(t, dst) {
		t.Fatalf("README missing after pinned sync at %s", dst)
	}
	// The later file MUST NOT be present — version pin held.
	later := path.Join(env.workdir, "src", "pinrepo", "LATER.md")
	if env.c.FileExists(t, later) {
		t.Errorf("version pin (%s) did not hold: LATER.md from a later commit is present",
			initial)
	}
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
