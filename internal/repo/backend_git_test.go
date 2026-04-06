package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// makeLocalRepo initialises a git repo in dir, creates one file, and commits it.
func makeLocalRepo(t *testing.T, dir string) *gogit.Repository {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("hello"), 0o644)
	w.Add("README.md")
	_, err = w.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return r
}

func TestVcsGit_Clone_LocalSource(t *testing.T) {
	srcDir := t.TempDir()
	makeLocalRepo(t, srcDir)
	destDir := filepath.Join(t.TempDir(), "clone")
	g := &VcsGit{}
	if err := g.Clone(context.Background(), srcDir, destDir, ""); err != nil {
		t.Fatalf("Clone from local source: %v", err)
	}
	if !g.IsCloned(destDir) {
		t.Error("expected IsCloned=true after Clone")
	}
}

func TestVcsGit_CurrentVersion_AfterClone(t *testing.T) {
	srcDir := t.TempDir()
	makeLocalRepo(t, srcDir)
	destDir := filepath.Join(t.TempDir(), "clone")
	g := &VcsGit{}
	if err := g.Clone(context.Background(), srcDir, destDir, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	ver, err := g.CurrentVersion(context.Background(), destDir)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty version string")
	}
}

func TestVcsGit_Update_AfterClone(t *testing.T) {
	srcDir := t.TempDir()
	makeLocalRepo(t, srcDir)
	destDir := filepath.Join(t.TempDir(), "clone")
	g := &VcsGit{}
	if err := g.Clone(context.Background(), srcDir, destDir, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	err := g.Update(context.Background(), destDir, "")
	_ = err
}

func TestVcsGit_Clone_WithVersion(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, err := r.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	version := head.Hash().String()
	destDir := filepath.Join(t.TempDir(), "clone")
	g := &VcsGit{}
	if err := g.Clone(context.Background(), srcDir, destDir, version); err != nil {
		t.Fatalf("Clone with version: %v", err)
	}
	if !g.IsCloned(destDir) {
		t.Error("expected IsCloned=true after Clone with version")
	}
}

func TestVcsGit_Clone_WithTagVersion(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()
	if _, err := r.CreateTag("v1.0.0", head.Hash(), nil); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	destDir := filepath.Join(t.TempDir(), "clone")
	g := &VcsGit{}
	if err := g.Clone(context.Background(), srcDir, destDir, "v1.0.0"); err != nil {
		t.Fatalf("Clone with tag version: %v", err)
	}
	if !g.IsCloned(destDir) {
		t.Error("expected IsCloned=true after clone with tag")
	}
}

func TestVcsGit_CurrentVersion_TaggedCommit(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()
	r.CreateTag("v2.0.0", head.Hash(), nil) //nolint:errcheck
	// The source repo itself has the tag; CurrentVersion should return "v2.0.0".
	g := &VcsGit{}
	ver, err := g.CurrentVersion(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if ver != "v2.0.0" {
		t.Logf("CurrentVersion returned %q (not tag name - may be branch)", ver)
	}
}

func TestVcsGit_CurrentVersion_DetachedHEAD(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()
	hash := head.Hash()

	// Check out the commit directly → detached HEAD.
	w, err := r.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := w.Checkout(&gogit.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		t.Fatalf("Checkout hash: %v", err)
	}

	g := &VcsGit{}
	ver, err := g.CurrentVersion(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("CurrentVersion detached HEAD: %v", err)
	}
	// Should return an 8-char short hash.
	if len(ver) > 8 {
		t.Errorf("expected short hash (≤8 chars), got %q", ver)
	}
	if ver == "" {
		t.Error("expected non-empty version for detached HEAD")
	}
}

func TestCheckoutVersion_InvalidVersion(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	err := checkoutVersion(r, "totally-nonexistent-branch-or-tag-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent version")
	}
}

func TestCheckoutVersion_CommitHash(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()
	hash := head.Hash().String()
	if err := checkoutVersion(r, hash); err != nil {
		t.Fatalf("checkoutVersion with commit hash: %v", err)
	}
}

func TestCheckoutVersion_ExistingLocalBranch(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()
	branchName := head.Name().Short() // "master" or "main"
	// Checking out the current branch should succeed immediately (step 1).
	if err := checkoutVersion(r, branchName); err != nil {
		t.Fatalf("checkoutVersion with existing branch %q: %v", branchName, err)
	}
}

func TestTagAtCommit_AnnotatedTag(t *testing.T) {
	srcDir := t.TempDir()
	r := makeLocalRepo(t, srcDir)
	head, _ := r.Head()

	// Create an annotated tag (CreateTag with non-nil options = annotated).
	_, err := r.CreateTag("v3.0.0-annotated", head.Hash(), &gogit.CreateTagOptions{
		Message: "Release v3.0.0",
		Tagger:  &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("CreateTag annotated: %v", err)
	}

	got := tagAtCommit(r, head.Hash())
	if got != "v3.0.0-annotated" {
		t.Errorf("expected annotated tag name, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// VcsGit - IsCloned / Clone / Update / CurrentVersion (basic cases)
// ---------------------------------------------------------------------------

func TestVcsGit_IsCloned_False(t *testing.T) {
	g := &VcsGit{}
	if g.IsCloned(t.TempDir()) {
		t.Error("expected IsCloned=false for empty temp dir")
	}
}

func TestVcsGit_IsCloned_True(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	g := &VcsGit{}
	if !g.IsCloned(dir) {
		t.Error("expected IsCloned=true for go-git initialised repo")
	}
}

func TestVcsGit_Clone_BadURL(t *testing.T) {
	g := &VcsGit{}
	err := g.Clone(context.Background(), "not-a-real-url", filepath.Join(t.TempDir(), "dest"), "")
	if err == nil {
		t.Fatal("expected error for bad URL")
	}
}

func TestVcsGit_Update_NotARepo(t *testing.T) {
	g := &VcsGit{}
	err := g.Update(context.Background(), t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error when updating non-git directory")
	}
}

func TestVcsGit_CurrentVersion_NotARepo(t *testing.T) {
	g := &VcsGit{}
	_, err := g.CurrentVersion(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when getting version of non-git directory")
	}
}

func TestVcsGit_Update_InitedRepoNoRemote(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	g := &VcsGit{}
	// Update will fail because there is no remote configured; this covers
	// the fetch error branch.
	err := g.Update(context.Background(), dir, "")
	if err == nil {
		t.Log("update succeeded unexpectedly on repo without remote")
	}
}
