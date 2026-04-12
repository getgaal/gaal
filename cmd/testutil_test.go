package cmd

import (
	"bytes"
	"os"
	"testing"
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
