package vcs

import (
	"testing"
)

func TestNew_KnownTypes(t *testing.T) {
	types := []string{"git", "hg", "svn", "bzr", "tar", "zip"}
	for _, vcsType := range types {
		t.Run(vcsType, func(t *testing.T) {
			v, err := New(vcsType)
			if err != nil {
				t.Fatalf("New(%q) returned error: %v", vcsType, err)
			}
			if v == nil {
				t.Fatalf("New(%q) returned nil", vcsType)
			}
		})
	}
}

func TestNew_UnknownType(t *testing.T) {
	_, err := New("cvs")
	if err == nil {
		t.Fatal("expected error for unknown VCS type 'cvs'")
	}
}

func TestNew_ArchiveFormat(t *testing.T) {
	for _, format := range []string{"tar", "zip"} {
		v, err := New(format)
		if err != nil {
			t.Fatalf("New(%q): %v", format, err)
		}
		arch, ok := v.(*VcsArchive)
		if !ok {
			t.Fatalf("expected *VcsArchive for format %q", format)
		}
		if arch.Format != format {
			t.Errorf("expected Format=%q, got %q", format, arch.Format)
		}
	}
}
