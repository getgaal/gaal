package vcs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"gaal/internal/urlx"
)

// VcsGit implements the VCS interface for Git repositories using go-git
// (pure Go — no git binary required).
//
// Authentication: HTTPS public repositories work without configuration.
// Private repos (SSH or authenticated HTTPS) are not yet supported.
type VcsGit struct {
	// Shallow requests a depth-1 clone (suitable for skill caches that never
	// need history). Update will hard-reset to origin/HEAD instead of pulling.
	Shallow bool
}

func (g *VcsGit) depth() int {
	if g.Shallow {
		return 1
	}
	return 0
}

func (g *VcsGit) Clone(ctx context.Context, url, path, version string) error {
	if err := urlx.ValidateRepoURL(url); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	safeURL := urlx.Redact(url)
	slog.DebugContext(ctx, "cloning", "url", safeURL, "path", shortPath(path), "version", version)

	// Stage into a sibling temp dir then atomically rename. Defends
	// against partial-clone leftovers (Ctrl-C, network drop, full disk):
	// the destination is either the previous tree or the new one, never
	// a half-written .git. #125.
	staging, err := os.MkdirTemp(parent, ".gaal-git-tmp-*")
	if err != nil {
		return fmt.Errorf("creating clone staging dir: %w", err)
	}
	cleanupStage := true
	defer func() {
		if cleanupStage {
			_ = os.RemoveAll(staging)
		}
	}()

	r, err := gogit.PlainCloneContext(ctx, staging, false, &gogit.CloneOptions{
		URL:               url,
		RecurseSubmodules: gogit.DefaultSubmoduleRecursionDepth,
		Tags:              gogit.AllTags,
		Depth:             g.depth(),
	})
	if err != nil {
		return fmt.Errorf("cloning %s: %w", safeURL, err)
	}

	if version != "" {
		if err := checkoutVersion(r, version); err != nil {
			return err
		}
	}

	// If path exists (e.g. a previously broken clone left behind), remove
	// it — the manager only calls Clone when IsCloned reports false, which
	// now means the previous content is invalid as a git repo.
	if _, statErr := os.Stat(path); statErr == nil {
		slog.DebugContext(ctx, "removing previous broken clone", "path", shortPath(path))
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("removing previous clone: %w", err)
		}
	}
	if err := os.Rename(staging, path); err != nil {
		return fmt.Errorf("renaming staging clone: %w", err)
	}
	cleanupStage = false
	return nil
}

func (g *VcsGit) Update(ctx context.Context, url, path, version string) error {
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("opening repository at %s: %w", shortPath(path), err)
	}

	if url != "" {
		if remote, rerr := r.Remote("origin"); rerr == nil && len(remote.Config().URLs) > 0 {
			actual := remote.Config().URLs[0]
			if normalizeGitURL(actual) != normalizeGitURL(url) {
				return &RemoteURLMismatchError{
					Path:          path,
					ConfiguredURL: url,
					RemoteURL:     actual,
				}
			}
		}
	}

	slog.DebugContext(ctx, "fetching", "path", shortPath(path))

	fetchErr := r.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		Tags:       gogit.AllTags,
		Force:      true,
		Prune:      true,
	})
	if fetchErr != nil && !errors.Is(fetchErr, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching: %w", fetchErr)
	}

	if version != "" {
		return checkoutVersion(r, version)
	}

	if g.Shallow {
		// Shallow / cache mode: hard-reset to remote HEAD so force-pushes and
		// history rewrites are always reflected.
		return g.resetToRemoteHEAD(ctx, r)
	}

	// No pinned version: fast-forward the current tracking branch.
	w, err := r.Worktree()
	if err != nil {
		return err
	}
	if err = w.PullContext(ctx, &gogit.PullOptions{
		RemoteName: "origin",
		Force:      true,
	}); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("pulling: %w", err)
	}
	return nil
}

// resetToRemoteHEAD hard-resets the working tree to the tip of origin's default
// branch. It tries refs/remotes/origin/HEAD first, then falls back to
// origin/main and origin/master.
func (g *VcsGit) resetToRemoteHEAD(ctx context.Context, r *gogit.Repository) error {
	var hash plumbing.Hash
	var found bool
	for _, refName := range []string{"HEAD", "main", "master"} {
		ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin", refName), true)
		if err == nil {
			hash = ref.Hash()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("could not resolve origin HEAD")
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}
	slog.DebugContext(ctx, "hard-resetting to remote HEAD", "hash", hash)
	return w.Reset(&gogit.ResetOptions{
		Commit: hash,
		Mode:   gogit.HardReset,
	})
}

// IsCloned reports whether path holds a valid git working copy. Older
// versions only checked for the existence of a .git entry, which would
// also succeed for half-cloned destinations (Ctrl-C, network drop) and
// for non-git directories that happened to have a .git file/dir inside.
// We now open the repository and verify it has a HEAD reference; on
// failure the caller (repo.Manager.syncOne) will fall back to Clone,
// which atomically replaces the broken tree. #125.
func (g *VcsGit) IsCloned(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return false
	}
	r, err := gogit.PlainOpen(path)
	if err != nil {
		slog.Debug("git: IsCloned: PlainOpen failed, treating as not cloned", "path", shortPath(path), "err", err)
		return false
	}
	if _, err := r.Head(); err != nil {
		// A repo with no HEAD is either freshly initialised (no commits
		// yet) or partially fetched. Either way it is not in a state
		// where Update would succeed; force a re-clone.
		slog.Debug("git: IsCloned: HEAD missing, treating as not cloned", "path", shortPath(path), "err", err)
		return false
	}
	return true
}

func (g *VcsGit) CurrentVersion(_ context.Context, path string) (string, error) {
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return "", fmt.Errorf("opening repository: %w", err)
	}

	head, err := r.Head()
	if err != nil {
		return "", fmt.Errorf("reading HEAD: %w", err)
	}

	// Prefer a tag name when HEAD coincides with a tag.
	if tag := tagAtCommit(r, head.Hash()); tag != "" {
		return tag, nil
	}

	// Return branch name or short hash (detached HEAD).
	if head.Name().IsBranch() {
		return head.Name().Short(), nil
	}
	h := head.Hash().String()
	if len(h) > 8 {
		h = h[:8]
	}
	return h, nil
}

// HasChanges reports whether the working tree contains modifications to
// tracked files (staged or unstaged). Untracked files are ignored.
func (g *VcsGit) HasChanges(ctx context.Context, path string) (bool, error) {
	slog.DebugContext(ctx, "checking for local changes", "path", shortPath(path))

	r, err := gogit.PlainOpen(path)
	if err != nil {
		return false, fmt.Errorf("opening repository: %w", err)
	}
	w, err := r.Worktree()
	if err != nil {
		return false, fmt.Errorf("reading worktree: %w", err)
	}
	st, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("reading status: %w", err)
	}
	return !st.IsClean(), nil
}

// checkoutVersion resolves version (branch, tag, or commit hash) inside r
// and checks it out. Resolution order: local branch → remote branch → tag → commit hash.
func checkoutVersion(r *gogit.Repository, version string) error {
	w, err := r.Worktree()
	if err != nil {
		return err
	}

	// 1. Local branch.
	if err = w.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(version),
		Force:  true,
	}); err == nil {
		return nil
	}

	// 2. Remote-tracking branch — create a local branch pointing at origin/<version>.
	if ref, e := r.Reference(plumbing.NewRemoteReferenceName("origin", version), true); e == nil {
		return w.Checkout(&gogit.CheckoutOptions{
			Hash:   ref.Hash(),
			Branch: plumbing.NewBranchReferenceName(version),
			Create: true,
			Force:  true,
		})
	}

	// 3. Tag.
	if err = w.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewTagReferenceName(version),
		Force:  true,
	}); err == nil {
		return nil
	}

	// 4. Commit hash.
	if hash := plumbing.NewHash(version); !hash.IsZero() {
		return w.Checkout(&gogit.CheckoutOptions{Hash: hash, Force: true})
	}

	return fmt.Errorf("could not resolve %q as a branch, tag, or commit", version)
}

// errTagFound is used as a sentinel to break out of go-git's ForEach iterator.
var errTagFound = errors.New("tag found")

// tagAtCommit returns the short name of the first tag (lightweight or annotated)
// that points to hash, or "" if none is found.
func tagAtCommit(r *gogit.Repository, hash plumbing.Hash) string {
	tags, err := r.Tags()
	if err != nil {
		return ""
	}
	var found string
	_ = tags.ForEach(func(ref *plumbing.Reference) error {
		// Lightweight tag: the ref hash is the commit hash directly.
		if ref.Hash() == hash {
			found = ref.Name().Short()
			return errTagFound
		}
		// Annotated tag: the ref points to a tag object; dereference to get the commit.
		if obj, e := r.TagObject(ref.Hash()); e == nil && obj.Target == hash {
			found = ref.Name().Short()
			return errTagFound
		}
		return nil
	})
	return found
}
