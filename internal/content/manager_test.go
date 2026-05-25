package content

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gaal/internal/config"
)

func TestManagerSync_ProjectWorkspaceInstructionFiles(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("shared guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{
			{
				Agents: []string{"claude-code"},
				Scope:  "project",
				Root:   "workspace",
				Paths:  map[string]string{"AGENTS.md": "CLAUDE.md"},
			},
			{
				Agents: []string{"codex"},
				Scope:  "project",
				Root:   "workspace",
				Paths:  map[string]string{"AGENTS.md": "AGENTS.md"},
			},
		},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		got, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if string(got) != "shared guidance" {
			t.Errorf("%s = %q", name, got)
		}
	}
}

func TestManagerStatusReportsMissingAndDirtyContent(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{{
			Agents: []string{"claude-code"},
			Scope:  "project",
			Root:   "workspace",
			Paths: map[string]string{
				"AGENTS.md": "CLAUDE.md",
				"README.md": "README.md",
			},
		}},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	byTarget := map[string]PathStatus{}
	for _, p := range statuses[0].Paths {
		byTarget[filepath.Base(p.Target)] = p
	}
	if !byTarget["CLAUDE.md"].Dirty {
		t.Errorf("expected CLAUDE.md to be dirty, got %+v", byTarget["CLAUDE.md"])
	}
	if byTarget["README.md"].Present {
		t.Errorf("expected README.md to be absent, got %+v", byTarget["README.md"])
	}
}

func TestCopyPathDirectorySkipsVCSMetadata(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(source, ".git", "objects")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "pack"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "commands")
	if err := copyPath(source, dst); err != nil {
		t.Fatalf("copyPath: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Errorf("expected copied file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Errorf("expected .git to be skipped, got err=%v", err)
	}
}
