package vcs

import (
	"context"
	"fmt"
)

// VCS is the interface all version-control backends must implement.
type VCS interface {
	// Clone clones a repository from url into path, checking out version.
	// If version is empty the default branch is used.
	Clone(ctx context.Context, url, path, version string) error

	// Update fetches and checks out version in an already-cloned repo at path.
	Update(ctx context.Context, path, version string) error

	// IsCloned reports whether path contains a valid local working copy.
	IsCloned(path string) bool

	// CurrentVersion returns a human-readable description of the working
	// copy's current state (tag, branch, commit hash, revision, …).
	CurrentVersion(ctx context.Context, path string) (string, error)

	// HasChanges reports whether the working copy at path contains local
	// modifications (tracked files added, deleted, or modified). Untracked
	// files are ignored for VCS types that support the distinction.
	// Returns false when the working copy is clean (immutable state intact).
	HasChanges(ctx context.Context, path string) (bool, error)
}

// Compile-time assertions: if any struct stops satisfying VCS, the build fails
// with a clear error pointing to the missing method.
var (
	_ VCS = (*VcsGit)(nil)
	_ VCS = (*VcsMercurial)(nil)
	_ VCS = (*VcsSVN)(nil)
	_ VCS = (*VcsBazaar)(nil)
	_ VCS = (*VcsArchive)(nil)
)

// New returns the VCS implementation for vcsType.
func New(vcsType string) (VCS, error) {
	switch vcsType {
	case "git":
		return &VcsGit{}, nil
	case "hg":
		return &VcsMercurial{}, nil
	case "svn":
		return &VcsSVN{}, nil
	case "bzr":
		return &VcsBazaar{}, nil
	case "tar", "zip":
		return &VcsArchive{Format: vcsType}, nil
	default:
		return nil, fmt.Errorf("unsupported VCS type: %q", vcsType)
	}
}

// NewShallow is like New but creates a shallow-clone variant of the backend
// when the type supports it (currently git only). Suitable for skill caches:
// clones with depth=1 and updates with hard-reset to origin HEAD.
func NewShallow(vcsType string) (VCS, error) {
	if vcsType == "git" {
		return &VcsGit{Shallow: true}, nil
	}
	return New(vcsType)
}
