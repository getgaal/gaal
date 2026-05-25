package secfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWrite_NewFileIs0o600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")
	if err := Write(path, []byte(`{"k":1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != Mode {
		t.Errorf("mode = %o, want %o", got, Mode)
	}
}

func TestWrite_TightensExistingLooseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(path, []byte("orig"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := Write(path, []byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != Mode {
		t.Errorf("mode = %o, want %o (loose file should be tightened)", got, Mode)
	}
}

func TestOpenAppend_TightensExistingLooseFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("orig\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, err := OpenAppend(path)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	defer f.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != Mode {
		t.Errorf("mode = %o, want %o", got, Mode)
	}
}
