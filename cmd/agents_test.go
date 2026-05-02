package cmd

import (
	"bytes"
	"strings"
	"testing"

	"gaal/internal/engine/render"
)

func TestRenderAgentsSummary(t *testing.T) {
	entries := []render.AgentEntry{
		{Name: "claude-code", Installed: true},
		{Name: "cursor", Installed: true},
		{Name: "windsurf", Installed: false},
	}
	var buf bytes.Buffer
	if err := renderAgentsSummary(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Installed:") {
		t.Errorf("missing Installed, got:\n%s", out)
	}
	if !strings.Contains(out, "claude-code") {
		t.Errorf("missing claude-code, got:\n%s", out)
	}
	if !strings.Contains(out, "Available:") {
		t.Errorf("missing Available, got:\n%s", out)
	}
	if !strings.Contains(out, "windsurf") {
		t.Errorf("missing windsurf, got:\n%s", out)
	}
}
