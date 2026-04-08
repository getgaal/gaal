package vcs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// makeFakeBin writes a minimal executable named `name` that executes `script`
// and returns the directory path. The caller must add that dir to PATH.
// On Windows a .bat wrapper is used because shell scripts are not executable.
func makeFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		bin := filepath.Join(binDir, name+".bat")
		os.WriteFile(bin, []byte("@echo off\n"+script+"\n"), 0o755)
	} else {
		bin := filepath.Join(binDir, name)
		os.WriteFile(bin, []byte("#!/bin/sh\n"+script+"\n"), 0o755)
	}
	return binDir
}
