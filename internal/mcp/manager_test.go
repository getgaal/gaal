package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gaal/internal/config"
)

func TestMergeIntoTarget_CreatesNewFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	entry := serverEntry{Command: "npx", Args: []string{"my-server"}}
	if err := mergeIntoTarget(target, "my-server", entry); err != nil {
		t.Fatalf("mergeIntoTarget: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading target: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing output JSON: %v", err)
	}
	var servers map[string]serverEntry
	if err := json.Unmarshal(raw["mcpServers"], &servers); err != nil {
		t.Fatalf("parsing mcpServers: %v", err)
	}
	got, ok := servers["my-server"]
	if !ok {
		t.Fatal("expected 'my-server' key in mcpServers")
	}
	if got.Command != "npx" {
		t.Errorf("expected command=npx, got %q", got.Command)
	}
}

func TestMergeIntoTarget_MergesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	existing := `{"mcpServers":{"existing":{"command":"node"}}}`
	os.WriteFile(target, []byte(existing), 0o644)
	entry := serverEntry{Command: "python", Args: []string{"-m", "server"}}
	if err := mergeIntoTarget(target, "new-server", entry); err != nil {
		t.Fatalf("mergeIntoTarget: %v", err)
	}
	data, _ := os.ReadFile(target)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	var servers map[string]serverEntry
	json.Unmarshal(raw["mcpServers"], &servers)
	if _, ok := servers["existing"]; !ok {
		t.Error("expected 'existing' key to be preserved")
	}
	if _, ok := servers["new-server"]; !ok {
		t.Error("expected 'new-server' key after merge")
	}
}

func TestMergeIntoTarget_UpsertExistingEntry(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	existing := `{"mcpServers":{"myserver":{"command":"old-cmd"}}}`
	os.WriteFile(target, []byte(existing), 0o644)
	entry := serverEntry{Command: "new-cmd"}
	if err := mergeIntoTarget(target, "myserver", entry); err != nil {
		t.Fatalf("mergeIntoTarget: %v", err)
	}
	data, _ := os.ReadFile(target)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	var servers map[string]serverEntry
	json.Unmarshal(raw["mcpServers"], &servers)
	if servers["myserver"].Command != "new-cmd" {
		t.Errorf("expected command=new-cmd after upsert, got %q", servers["myserver"].Command)
	}
}

func TestMergeIntoTarget_NestedParentMissing_Skips(t *testing.T) {
	// Sync must never create nested parent directories as a side effect.
	// This test pins the "skip when the direct parent is missing" contract
	// introduced by issue #17 — the previous "creates parent dir" behaviour
	// is exactly what materialised ~/.zencoder/ on machines without zencoder.
	target := filepath.Join(t.TempDir(), "nested", "deep", "mcp.json")
	entry := serverEntry{Command: "cmd"}
	if err := mergeIntoTarget(target, "s", entry); err != nil {
		t.Fatalf("mergeIntoTarget returned error for missing parent: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("target file must not be created when parent does not exist")
	}
	if _, err := os.Stat(filepath.Dir(target)); err == nil {
		t.Error("nested parent directory must not be created")
	}
}

func TestFetchRemoteEntry_MCPServersDocument(t *testing.T) {
	payload := `{"mcpServers":{"wanted":{"command":"serve","args":["--port","8080"]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	entry, err := fetchRemoteEntry(context.Background(), srv.URL, "wanted")
	if err != nil {
		t.Fatalf("fetchRemoteEntry: %v", err)
	}
	if entry.Command != "serve" {
		t.Errorf("expected command=serve, got %q", entry.Command)
	}
}

func TestFetchRemoteEntry_FallbackToFirstEntry(t *testing.T) {
	payload := `{"mcpServers":{"other":{"command":"fallback"}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	entry, err := fetchRemoteEntry(context.Background(), srv.URL, "not-present")
	if err != nil {
		t.Fatalf("fetchRemoteEntry fallback: %v", err)
	}
	if entry.Command != "fallback" {
		t.Errorf("expected command=fallback, got %q", entry.Command)
	}
}

func TestFetchRemoteEntry_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_, err := fetchRemoteEntry(context.Background(), srv.URL, "any")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestFetchRemoteEntry_EmptyMCPServers(t *testing.T) {
	payload := `{"mcpServers":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	_, err := fetchRemoteEntry(context.Background(), srv.URL, "any")
	if err == nil {
		t.Fatal("expected error when mcpServers is empty")
	}
}

func TestManager_Sync_Inline(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	mcps := []config.MCPConfig{
		{
			Name:   "inline-server",
			Target: target,
			Inline: &config.MCPInlineConfig{
				Command: "node",
				Args:    []string{"server.js"},
			},
		},
	}
	m := NewManager(mcps)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("expected target file to be created by Sync")
	}
}

func TestManager_Sync_NoSourceOrInline(t *testing.T) {
	mcps := []config.MCPConfig{
		{Name: "bad", Target: filepath.Join(t.TempDir(), "mcp.json")},
	}
	m := NewManager(mcps)
	err := m.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error when no source or inline provided")
	}
}

func TestManager_Status_Missing(t *testing.T) {
	mcps := []config.MCPConfig{
		{Name: "srv", Target: "/no/such/file.json"},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Present {
		t.Error("expected Present=false for missing target")
	}
}

func TestManager_Status_Present(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers":{"my-srv":{"command":"cmd"}}}`), 0o644)
	mcps := []config.MCPConfig{
		{Name: "my-srv", Target: target},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Present {
		t.Error("expected Present=true when entry exists in target")
	}
}

func TestManager_Sync_WithSource(t *testing.T) {
	// syncOne with mc.Source set — covers the Source branch.
	payload := `{"mcpServers":{"my-srv":{"command":"node","args":["server.js"]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	target := filepath.Join(t.TempDir(), "mcp.json")
	mcps := []config.MCPConfig{
		{Name: "my-srv", Source: srv.URL, Target: target},
	}
	m := NewManager(mcps)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with source URL: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("expected target file to be created by Sync with source")
	}
}

func TestMergeIntoTarget_InvalidMCPServersValue(t *testing.T) {
	// mcpServers exists but has a value that cannot be unmarshalled into a map.
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers":123}`), 0o644)
	entry := serverEntry{Command: "cmd"}
	err := mergeIntoTarget(target, "s", entry)
	if err == nil {
		t.Fatal("expected error when mcpServers is not a JSON object")
	}
}

func TestMergeIntoTarget_InvalidExistingJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`not valid json`), 0o644)
	entry := serverEntry{Command: "cmd"}
	err := mergeIntoTarget(target, "s", entry)
	if err == nil {
		t.Fatal("expected error when existing file has invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// mergeIntoTarget / Manager.Sync — parent directory must already exist.
// sync never creates an agent-owned directory as a side effect.
// ---------------------------------------------------------------------------

func TestMergeIntoTarget_ParentMissing_Skips(t *testing.T) {
	// Target lives under a directory that does not yet exist — simulating
	// an uninstalled agent. mergeIntoTarget must NOT create the parent,
	// must NOT create the target, and must return nil.
	root := t.TempDir()
	parent := filepath.Join(root, ".zencoder")
	target := filepath.Join(parent, "mcp.json")

	entry := serverEntry{Command: "node"}
	if err := mergeIntoTarget(target, "s", entry); err != nil {
		t.Fatalf("mergeIntoTarget returned error for missing parent: %v", err)
	}

	if _, err := os.Stat(parent); err == nil {
		t.Error("mergeIntoTarget must not create the parent directory")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("mergeIntoTarget must not create the target file")
	}
}

func TestMergeIntoTarget_ParentNotADirectory_Errors(t *testing.T) {
	// The "parent" path exists but is a regular file, not a directory.
	// This is a real-world misconfiguration and should error explicitly
	// rather than silently skipping.
	root := t.TempDir()
	parent := filepath.Join(root, "not-a-dir")
	os.WriteFile(parent, []byte("plain file"), 0o644)
	target := filepath.Join(parent, "mcp.json")

	entry := serverEntry{Command: "node"}
	err := mergeIntoTarget(target, "s", entry)
	if err == nil {
		t.Fatal("expected error when target parent is not a directory")
	}
}

func TestManager_Sync_SkipsUninstalledAgentTarget(t *testing.T) {
	// End-to-end: a full Sync run with an MCP entry targeting a path inside
	// a directory that does not exist must return nil and leave the disk
	// untouched.
	root := t.TempDir()
	parent := filepath.Join(root, ".zencoder")
	target := filepath.Join(parent, "mcp.json")

	mcps := []config.MCPConfig{
		{
			Name:   "srv",
			Target: target,
			Inline: &config.MCPInlineConfig{Command: "node"},
		},
	}
	m := NewManager(mcps)
	if err := m.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := os.Stat(parent); err == nil {
		t.Errorf("Sync must not create uninstalled agent directory %s", parent)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("Sync must not create target file %s", target)
	}
}

func TestManager_Status_InvalidJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(target, []byte(`invalid json {{{`), 0o644)
	mcps := []config.MCPConfig{
		{Name: "srv", Target: target},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Err == nil {
		t.Error("expected error status for invalid JSON target")
	}
}

// ---------------------------------------------------------------------------
// serverEntryEqual
// ---------------------------------------------------------------------------

func TestServerEntryEqual_Identical(t *testing.T) {
	a := serverEntry{Command: "node", Args: []string{"server.js"}, Env: map[string]string{"PORT": "8080"}}
	b := serverEntry{Command: "node", Args: []string{"server.js"}, Env: map[string]string{"PORT": "8080"}}
	if !serverEntryEqual(a, b) {
		t.Error("expected equal entries to be reported as equal")
	}
}

func TestServerEntryEqual_DifferentCommand(t *testing.T) {
	a := serverEntry{Command: "node"}
	b := serverEntry{Command: "python"}
	if serverEntryEqual(a, b) {
		t.Error("expected different commands to be reported as not equal")
	}
}

func TestServerEntryEqual_DifferentArgs(t *testing.T) {
	a := serverEntry{Command: "cmd", Args: []string{"--a"}}
	b := serverEntry{Command: "cmd", Args: []string{"--b"}}
	if serverEntryEqual(a, b) {
		t.Error("expected different args to be reported as not equal")
	}
}

func TestServerEntryEqual_DifferentEnv(t *testing.T) {
	a := serverEntry{Command: "cmd", Env: map[string]string{"K": "v1"}}
	b := serverEntry{Command: "cmd", Env: map[string]string{"K": "v2"}}
	if serverEntryEqual(a, b) {
		t.Error("expected different env to be reported as not equal")
	}
}

func TestServerEntryEqual_NilVsEmpty(t *testing.T) {
	a := serverEntry{Command: "cmd", Args: nil}
	b := serverEntry{Command: "cmd", Args: []string{}}
	if !serverEntryEqual(a, b) {
		t.Error("expected nil and empty slice to be treated as equal")
	}
}

// ---------------------------------------------------------------------------
// Manager.Status — Dirty detection for inline MCP
// ---------------------------------------------------------------------------

func TestManager_Status_DirtyInline(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	// Store a different command than what is configured.
	os.WriteFile(target, []byte(`{"mcpServers":{"srv":{"command":"old-cmd"}}}`), 0o644)

	mcps := []config.MCPConfig{
		{
			Name:   "srv",
			Target: target,
			Inline: &config.MCPInlineConfig{Command: "new-cmd"},
		},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].Present {
		t.Error("expected Present=true")
	}
	if !statuses[0].Dirty {
		t.Error("expected Dirty=true when stored command differs from configured")
	}
}

func TestManager_Status_CleanInline(t *testing.T) {
	target := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers":{"srv":{"command":"node","args":["server.js"]}}}`), 0o644)

	mcps := []config.MCPConfig{
		{
			Name:   "srv",
			Target: target,
			Inline: &config.MCPInlineConfig{Command: "node", Args: []string{"server.js"}},
		},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Dirty {
		t.Error("expected Dirty=false when stored and configured entries are identical")
	}
}

func TestManager_Status_SourceNoInline_NoDirtyCheck(t *testing.T) {
	// For source-based MCPs (no Inline), Dirty should always be false at status time.
	target := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers":{"srv":{"command":"something"}}}`), 0o644)

	mcps := []config.MCPConfig{
		{Name: "srv", Target: target, Source: "https://example.com/mcp.json"},
	}
	m := NewManager(mcps)
	statuses := m.Status(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Dirty {
		t.Error("expected Dirty=false for source-based MCP (no inline check)")
	}
}

// ---------------------------------------------------------------------------
// ListServers
// ---------------------------------------------------------------------------

func TestListServers_FileNotExist(t *testing.T) {
	names, err := ListServers("/no/such/file.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if names != nil {
		t.Errorf("expected nil slice for missing file, got %v", names)
	}
}

func TestListServers_ValidFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "mcp.json")
	content := `{"mcpServers":{"server-b":{},"server-a":{},"server-c":{}}}`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err := ListServers(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 servers, got %d: %v", len(names), names)
	}
	// Must be sorted.
	if names[0] != "server-a" || names[1] != "server-b" || names[2] != "server-c" {
		t.Errorf("unexpected order: %v", names)
	}
}

func TestListServers_NoMCPServersKey(t *testing.T) {
	f := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(f, []byte(`{"other":"value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err := ListServers(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names != nil {
		t.Errorf("expected nil when no mcpServers key, got %v", names)
	}
}

func TestListServers_InvalidJSON(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(f, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ListServers(f)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// LoadServers
// ---------------------------------------------------------------------------

func TestLoadServers_FileNotExist(t *testing.T) {
	got, err := LoadServers("/no/such/file.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map for missing file, got %v", got)
	}
}

func TestLoadServers_ValidFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "mcp.json")
	content := `{
  "mcpServers": {
    "filesystem": {
      "command": "uvx",
      "args": ["mcp-server-filesystem", "/tmp"],
      "env": {"LOG_LEVEL": "debug"}
    },
    "git": {
      "command": "uvx",
      "args": ["mcp-server-git"]
    }
  }
}`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadServers(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	fs, ok := got["filesystem"]
	if !ok {
		t.Fatal("filesystem entry missing")
	}
	if fs.Command != "uvx" {
		t.Errorf("filesystem.command: want uvx, got %q", fs.Command)
	}
	if len(fs.Args) != 2 || fs.Args[0] != "mcp-server-filesystem" || fs.Args[1] != "/tmp" {
		t.Errorf("filesystem.args: unexpected %v", fs.Args)
	}
	if fs.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("filesystem.env: unexpected %v", fs.Env)
	}
	g, ok := got["git"]
	if !ok {
		t.Fatal("git entry missing")
	}
	if g.Command != "uvx" || len(g.Args) != 1 || g.Args[0] != "mcp-server-git" {
		t.Errorf("git entry: unexpected %+v", g)
	}
	if g.Env != nil && len(g.Env) != 0 {
		t.Errorf("git.env should be empty, got %v", g.Env)
	}
}

func TestLoadServers_NoMCPServersKey(t *testing.T) {
	f := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(f, []byte(`{"other":"value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadServers(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no mcpServers key, got %v", got)
	}
}

func TestLoadServers_InvalidJSON(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(f, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadServers(f); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
