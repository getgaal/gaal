package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTip_WhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	WriteTip(&buf, true)
	out := buf.String()
	if !strings.Contains(out, "-o table") {
		t.Errorf("missing table hint, got:\n%s", out)
	}
	if !strings.Contains(out, "-o json") {
		t.Errorf("missing json hint, got:\n%s", out)
	}
}

func TestWriteTip_WhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	WriteTip(&buf, false)
	if buf.Len() != 0 {
		t.Errorf("expected no output when disabled, got: %q", buf.String())
	}
}
