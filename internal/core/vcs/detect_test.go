package vcs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectType_Remote_Git(t *testing.T) {
	cases := []string{
		"https://github.com/owner/repo",
		"https://gitlab.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"https://bitbucket.org/owner/repo",
	}
	for _, src := range cases {
		got := DetectType(src)
		if got != "git" {
			t.Errorf("DetectType(%q) = %q, want git", src, got)
		}
	}
}

func TestDetectType_Remote_Archive(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"https://example.com/project.tar.gz", "tar"},
		{"https://example.com/project.tgz", "tar"},
		{"https://example.com/project.zip", "zip"},
	}
	for _, c := range cases {
		got := DetectType(c.src)
		if got != c.want {
			t.Errorf("DetectType(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestDetectType_Local_Git(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	if got := DetectType(dir); got != "git" {
		t.Errorf("DetectType with .git = %q, want git", got)
	}
}

func TestDetectType_Local_Hg(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hg"), 0o755)
	if got := DetectType(dir); got != "hg" {
		t.Errorf("DetectType with .hg = %q, want hg", got)
	}
}

func TestDetectType_Local_Svn(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".svn"), 0o755)
	if got := DetectType(dir); got != "svn" {
		t.Errorf("DetectType with .svn = %q, want svn", got)
	}
}

func TestDetectType_Local_Bzr(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bzr"), 0o755)
	if got := DetectType(dir); got != "bzr" {
		t.Errorf("DetectType with .bzr = %q, want bzr", got)
	}
}

func TestDetectType_Local_NoMarker_DefaultsToGit(t *testing.T) {
	dir := t.TempDir() // empty directory
	if got := DetectType(dir); got != "git" {
		t.Errorf("DetectType with no marker = %q, want git", got)
	}
}

func TestDetectType_RelativePath_Treated_As_Local(t *testing.T) {
	// A relative path starting with ./ should be treated as local.
	// We can't resolve it to anything useful, but it must not be "tar" or "zip".
	got := DetectType("./some-dir")
	if got == "tar" || got == "zip" {
		t.Errorf("DetectType(./some-dir) = %q, expected a VCS type", got)
	}
}

func TestNewShallow_Git(t *testing.T) {
	b, err := NewShallow("git")
	if err != nil {
		t.Fatalf("NewShallow(git): %v", err)
	}
	g, ok := b.(*VcsGit)
	if !ok {
		t.Fatalf("expected *VcsGit, got %T", b)
	}
	if !g.Shallow {
		t.Error("expected Shallow=true")
	}
}

func TestNewShallow_NonGit_DelegatesToNew(t *testing.T) {
	b, err := NewShallow("hg")
	if err != nil {
		t.Fatalf("NewShallow(hg): %v", err)
	}
	if _, ok := b.(*VcsMercurial); !ok {
		t.Errorf("expected *VcsMercurial, got %T", b)
	}
}

func TestNewShallow_Unknown(t *testing.T) {
	_, err := NewShallow("cvs")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}
