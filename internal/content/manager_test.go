package content

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getgaal/gaal/internal/config"
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

// ── roots: fan-out ───────────────────────────────────────────────────────

func TestManagerSync_Roots_ExplicitList_CopiesIntoEachRoot(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("shared guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	for _, sub := range []string{"frontend", "api"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			t.Fatalf("seed %s: %v", sub, err)
		}
	}

	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{{
			Agents: []string{"claude-code"},
			Scope:  "project",
			Root:   "workspace",
			Roots:  []string{"frontend", "api"},
			Paths:  map[string]string{"AGENTS.md": "CLAUDE.md"},
		}},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	for _, sub := range []string{"frontend", "api"} {
		got, err := os.ReadFile(filepath.Join(workDir, sub, "CLAUDE.md"))
		if err != nil {
			t.Fatalf("reading %s/CLAUDE.md: %v", sub, err)
		}
		if string(got) != "shared guidance" {
			t.Errorf("%s/CLAUDE.md = %q", sub, got)
		}
	}
	// The umbrella root itself must not receive a copy.
	if _, err := os.Stat(filepath.Join(workDir, "CLAUDE.md")); err == nil {
		t.Error("content copied to the umbrella root; roots: should target only the declared sub-repos")
	}
}

func TestManagerSync_Roots_Wildcard_CopiesIntoEveryVCSChild(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("shared guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	for _, sub := range []string{"frontend", "api"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub, ".git"), 0o755); err != nil {
			t.Fatalf("seed %s/.git: %v", sub, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(workDir, "not-a-repo"), 0o755); err != nil {
		t.Fatalf("seed not-a-repo: %v", err)
	}

	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{{
			Agents: []string{"claude-code"},
			Scope:  "project",
			Root:   "workspace",
			Roots:  []string{"*"},
			Paths:  map[string]string{"AGENTS.md": "CLAUDE.md"},
		}},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	for _, sub := range []string{"frontend", "api"} {
		if _, err := os.Stat(filepath.Join(workDir, sub, "CLAUDE.md")); err != nil {
			t.Errorf("expected content at %s/CLAUDE.md: %v", sub, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "not-a-repo", "CLAUDE.md")); err == nil {
		t.Error("content copied into a non-VCS directory; roots: [\"*\"] must only match VCS children")
	}
}

func TestManagerSync_NoRoots_UnchangedBehaviour(t *testing.T) {
	// Regression guard: no roots: must behave exactly as before the field
	// existed — one copy, directly under workDir.
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("shared guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{{
			Agents: []string{"claude-code"},
			Scope:  "project",
			Root:   "workspace",
			Paths:  map[string]string{"AGENTS.md": "CLAUDE.md"},
		}},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "CLAUDE.md")); err != nil {
		t.Errorf("expected content at workDir/CLAUDE.md: %v", err)
	}
}

func TestManagerStatus_Roots_ReportsSubRootPerEntry(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("shared guidance"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	for _, sub := range []string{"frontend", "api"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			t.Fatalf("seed %s: %v", sub, err)
		}
	}
	entries := []config.ConfigContent{{
		Source: source,
		Targets: []config.ConfigContentTarget{{
			Agents: []string{"claude-code"},
			Scope:  "project",
			Root:   "workspace",
			Roots:  []string{"frontend", "api"},
			Paths:  map[string]string{"AGENTS.md": "CLAUDE.md"},
		}},
	}}

	m := NewManager(entries, t.TempDir(), t.TempDir(), workDir, "", false)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	seen := map[string]bool{}
	for _, st := range m.Status(context.Background()) {
		seen[st.SubRoot] = true
	}
	for _, want := range []string{"frontend", "api"} {
		if !seen[want] {
			t.Errorf("Status did not report SubRoot %q; got %v", want, seen)
		}
	}
}
