package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpencodeCodec_RoundTripPreservesUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	original := `{
  "$schema": "https://opencode.ai/config.json",
  "model": "ollama/kimi-k2.7-code:cloud",
  "provider": {
    "ollama": {
      "name": "Ollama Cloud"
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := opencodeCodec{}
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil servers map, got %v", servers)
	}

	servers = map[string]serverEntry{
		"context7": {Type: "http", URL: "https://mcp.context7.com/mcp"},
	}
	if err := c.WriteServers(path, servers); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse back: %v", err)
	}

	if doc["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema lost: %v", doc["$schema"])
	}
	if doc["model"] != "ollama/kimi-k2.7-code:cloud" {
		t.Errorf("model lost: %v", doc["model"])
	}
	provider, ok := doc["provider"].(map[string]any)
	if !ok || provider["ollama"] == nil {
		t.Errorf("provider lost: %v", doc["provider"])
	}

	// Entry must be under "mcp", not "mcpServers".
	mcp, ok := doc["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level \"mcp\" key, got keys: %v", doc)
	}
	if _, ok := doc["mcpServers"]; ok {
		t.Error("wrote \"mcpServers\" key; opencode reads \"mcp\", not \"mcpServers\"")
	}
	ctx7, ok := mcp["context7"].(map[string]any)
	if !ok {
		t.Fatalf("context7 missing under mcp: %v", mcp)
	}
	if ctx7["type"] != "remote" {
		t.Errorf("type = %v, want remote (URL-only entry)", ctx7["type"])
	}
	if ctx7["url"] != "https://mcp.context7.com/mcp" {
		t.Errorf("url = %v, want https://mcp.context7.com/mcp", ctx7["url"])
	}

	// Round-trip back through the codec.
	servers, err = c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers (after write): %v", err)
	}
	got, ok := servers["context7"]
	if !ok {
		t.Fatalf("context7 missing after round-trip; got %v", servers)
	}
	if got.URL != "https://mcp.context7.com/mcp" {
		t.Errorf("url = %q, want https://mcp.context7.com/mcp", got.URL)
	}
}

func TestOpencodeCodec_LocalEntryUsesCommandArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")

	c := opencodeCodec{}
	in := map[string]serverEntry{
		"filesystem": {
			Command: "uvx",
			Args:    []string{"mcp-server-filesystem", "/home/user/projects"},
			Env:     map[string]string{"DEBUG": "1"},
		},
	}
	if err := c.WriteServers(path, in); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc struct {
		MCP map[string]struct {
			Type        string            `json:"type"`
			Command     []string          `json:"command"`
			Environment map[string]string `json:"environment"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse back: %v", err)
	}
	fs, ok := doc.MCP["filesystem"]
	if !ok {
		t.Fatalf("filesystem entry missing: %v", doc.MCP)
	}
	if fs.Type != "local" {
		t.Errorf("type = %q, want local", fs.Type)
	}
	want := []string{"uvx", "mcp-server-filesystem", "/home/user/projects"}
	if len(fs.Command) != len(want) {
		t.Fatalf("command = %v, want %v", fs.Command, want)
	}
	for i, w := range want {
		if fs.Command[i] != w {
			t.Errorf("command[%d] = %q, want %q", i, fs.Command[i], w)
		}
	}
	if fs.Environment["DEBUG"] != "1" {
		t.Errorf("environment not preserved: %+v", fs.Environment)
	}

	// And round-tripping back through the codec must split command[0] into
	// Command and the rest into Args again.
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	got := servers["filesystem"]
	if got.Command != "uvx" {
		t.Errorf("Command = %q, want uvx", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "mcp-server-filesystem" || got.Args[1] != "/home/user/projects" {
		t.Errorf("Args = %v, want [mcp-server-filesystem /home/user/projects]", got.Args)
	}
}

func TestOpencodeCodec_MissingFileReturnsNil(t *testing.T) {
	servers, err := opencodeCodec{}.ReadServers(filepath.Join(t.TempDir(), "no-such", "opencode.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil servers map, got %v", servers)
	}
}

func TestOpencodeCodec_UpsertReplacesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	seed := `{
  "mcp": {
    "context7": {"type": "remote", "url": "https://old.example.com/mcp"}
  }
}
`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := opencodeCodec{}
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	servers["context7"] = serverEntry{URL: "https://mcp.context7.com/mcp"}
	if err := c.WriteServers(path, servers); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}

	out, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers (after write): %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 entry, got %d: %v", len(out), out)
	}
	if out["context7"].URL != "https://mcp.context7.com/mcp" {
		t.Errorf("url = %q, want https://mcp.context7.com/mcp", out["context7"].URL)
	}
}

func TestMergeIntoTarget_OpencodeDispatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(target, []byte(`{"model": "ollama/kimi-k2.7-code:cloud"}`+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	entry := serverEntry{Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-git"}}
	if err := mergeIntoTarget(target, "git", entry); err != nil {
		t.Fatalf("mergeIntoTarget: %v", err)
	}

	data, _ := os.ReadFile(target)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("target is not valid JSON: %v\n%s", err, data)
	}
	if _, ok := doc["mcpServers"]; ok {
		t.Error("mergeIntoTarget wrote \"mcpServers\" into opencode.json; opencode reads \"mcp\"")
	}
	mcp, ok := doc["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level \"mcp\" key, got: %v", doc)
	}
	if mcp["git"] == nil {
		t.Errorf("git entry not written: %v", mcp)
	}
}

// ── error paths ──────────────────────────────────────────────────────────

func TestOpencodeCodec_ReadServers_InvalidTopLevelJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`not valid json`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := (opencodeCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error for invalid top-level JSON")
	}
}

func TestOpencodeCodec_ReadServers_McpKeyWrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	// "mcp" exists but is a number, not an object — must fail to unmarshal
	// into map[string]json.RawMessage, not silently treat it as empty.
	if err := os.WriteFile(path, []byte(`{"mcp":123}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := (opencodeCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error when mcp is not a JSON object")
	}
}

func TestOpencodeCodec_ReadServers_EntryWrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	// mcp.context7 exists but is a string, not an opencodeEntry object.
	if err := os.WriteFile(path, []byte(`{"mcp":{"context7":"nope"}}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := (opencodeCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error when an mcp entry is not a JSON object")
	}
}

func TestOpencodeCodec_ReadServers_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{}}`), 0o000); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) // TempDir cleanup needs read perms back
	if _, err := (opencodeCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestOpencodeEntryFrom_NeitherURLNorCommand_PreservesRawType(t *testing.T) {
	// An entry with no URL and no Command can't be classified as
	// local/remote by opencodeEntryFrom's normal heuristic; it must fall
	// back to preserving whatever Type was already set rather than
	// silently emitting an empty type.
	got := opencodeEntryFrom(serverEntry{Type: "custom"})
	if got.Type != "custom" {
		t.Errorf("Type = %q, want %q (preserved from serverEntry.Type)", got.Type, "custom")
	}
	if got.URL != "" || len(got.Command) != 0 {
		t.Errorf("expected no URL/Command, got URL=%q Command=%v", got.URL, got.Command)
	}
}

func TestOpencodeCodec_WriteServers_ExistingFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(`not valid json`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := (opencodeCodec{}).WriteServers(path, map[string]serverEntry{"s": {Command: "cmd"}})
	if err == nil {
		t.Fatal("expected error when existing file has invalid JSON")
	}
}
