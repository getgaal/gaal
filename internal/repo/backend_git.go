package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// VcsGit implements the VCS interface for Git repositories using go-git
// (pure Go — no git binary required).
//
// Authentication: HTTPS public repositories work without configuration.
// Private repos (SSH or authenticated HTTPS) are not yet supported.
type VcsGit struct{}

func (g *VcsGit) Clone(ctx context.Context, url, path, version string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	slog.DebugContext(ctx, "cloning", "url", url, "path", shortPath(path), "version", version)

	r, err := gogit.PlainCloneContext(ctx, path, false, &gogit.CloneOptions{
		URL:               url,
		RecurseSubmodules: gogit.DefaultSubmoduleRecursionDepth,
		Tags:              gogit.AllTags,
	})
	if err != nil {
		return fmt.Errorf("cloning %s: %w", url, err)
	}

	if version != "" {
		return checkoutVersion(r, version)
	}
	return nil
}

func (g *VcsGit) Update(ctx context.Context, path, version string) error {
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("opening repository at %s: %w", shortPath(path), err)
	}

	slog.DebugContext(ctx, "fetching", "path", shortPath(path))

	fetchErr := r.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		Tags:       gogit.AllTags,
		Force:      true,
	})
	if fetchErr != nil && !errors.Is(fetchErr, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetching: %w", fetchErr)
	}

	if version != "" {
		return checkoutVersion(r, version)
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

func (g *VcsGit) IsCloned(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
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
