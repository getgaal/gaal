package cmd

import (
	"bytes"
	"os"
	"testing"

	"gaal/internal/config"
)

// captureStdout redirects os.Stdout to an os.Pipe for the duration of fn,
// then restores it and returns everything that was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf.ReadFrom(r) //nolint:errcheck
	}()

	fn()
	w.Close()
	os.Stdout = orig
	<-done
	r.Close()
	return buf.String()
}

// setConfig sets cfgFile and loads resolvedCfg from path, mimicking what
// PersistentPreRunE does at runtime. Registers cleanup to restore both.
func setConfig(t *testing.T, path string) {
	t.Helper()
	orig := cfgFile
	origResolved := resolvedCfg
	t.Cleanup(func() {
		cfgFile = orig
		resolvedCfg = origResolved
	})
	cfgFile = path
	rc, err := config.LoadChain(path)
	if err != nil {
		t.Fatalf("setConfig: %v", err)
	}
	resolvedCfg = rc
}
