package vcs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckEmptyDestination_MissingPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := CheckEmptyDestination(dir); err != nil {
		t.Fatalf("missing path should be allowed, got: %v", err)
	}
}

func TestCheckEmptyDestination_EmptyDir(t *testing.T) {
	if err := CheckEmptyDestination(t.TempDir()); err != nil {
		t.Fatalf("empty directory should be allowed, got: %v", err)
	}
}

func TestCheckEmptyDestination_NonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err := CheckEmptyDestination(dir)
	if err == nil {
		t.Fatal("expected error for non-empty directory")
	}
	var ne *NonEmptyDestinationError
	if !errors.As(err, &ne) {
		t.Fatalf("expected *NonEmptyDestinationError, got %T: %v", err, err)
	}
	if ne.Path != dir {
		t.Errorf("Path = %q, want %q", ne.Path, dir)
	}
	found := false
	for _, name := range ne.Entries {
		if name == "data.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Entries to include %q, got %v", "data.txt", ne.Entries)
	}
}

func TestCheckEmptyDestination_HiddenFileCounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err := CheckEmptyDestination(dir)
	var ne *NonEmptyDestinationError
	if !errors.As(err, &ne) {
		t.Fatalf("hidden file must count as non-empty, got %T: %v", err, err)
	}
}

func TestCheckEmptyDestination_PathIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := CheckEmptyDestination(file); err == nil {
		t.Fatal("expected error when destination is an existing file")
	}
}

func TestNonEmptyDestinationError_Message(t *testing.T) {
	err := &NonEmptyDestinationError{
		Path:    "/home/u/.claude",
		Entries: []string{"plans", ".credentials.json", "settings.json"},
	}
	msg := err.Error()
	for _, want := range []string{
		"/home/u/.claude",
		"not empty",
		"plans",
		".credentials.json",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q\n  got: %s", want, msg)
		}
	}
}

func TestNonEmptyDestinationError_TruncatesEntries(t *testing.T) {
	entries := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, "file")
	}
	err := &NonEmptyDestinationError{Path: "/x", Entries: entries}
	msg := err.Error()
	if !strings.Contains(msg, "more") {
		t.Errorf("expected truncation hint in message, got: %s", msg)
	}
}

func TestNonEmptyDestinationError_AsTarget(t *testing.T) {
	err := &NonEmptyDestinationError{Path: "/x", Entries: []string{"a"}}
	var target *NonEmptyDestinationError
	if !errors.As(err, &target) {
		t.Fatal("errors.As did not match NonEmptyDestinationError")
	}
}
