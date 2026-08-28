package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONCodec_RoundTripPreservesUnrelatedKeys(t *testing.T) {
	// Mimic a real ~/.claude.json that already carries session state, MRU
	// lists, and per-project entries alongside mcpServers.
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	original := `{
  "numStartups": 42,
  "projects": {"/home/user/repo": {"lastUsed": "2026-01-01"}},
  "oauthAccount": {"email": "user@example.com"}
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := jsonCodec{}
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

	keys, raw, err := readOrderedJSON(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, want := range []string{"numStartups", "projects", "oauthAccount", "mcpServers"} {
		found := false
		for _, k := range keys {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("key %q lost; keys = %v", want, keys)
		}
	}
	if string(raw["numStartups"]) != "42" {
		t.Errorf("numStartups lost: %s", raw["numStartups"])
	}

	got, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers (after write): %v", err)
	}
	if got["context7"].URL != "https://mcp.context7.com/mcp" {
		t.Errorf("url = %q, want https://mcp.context7.com/mcp", got["context7"].URL)
	}
}

func TestJSONCodec_MissingFileReturnsNil(t *testing.T) {
	servers, err := jsonCodec{}.ReadServers(filepath.Join(t.TempDir(), "no-such.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil servers map, got %v", servers)
	}
}

func TestJSONCodec_ReadServers_InvalidTopLevelJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`not valid json`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := (jsonCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error for invalid top-level JSON")
	}
}

func TestJSONCodec_ReadServers_McpServersWrongType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":123}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := (jsonCodec{}).ReadServers(path); err == nil {
		t.Fatal("expected error when mcpServers is not a JSON object")
	}
}

func TestJSONCodec_WriteServers_ExistingFileInvalidJSON(t *testing.T) {
	// WriteServers must surface readOrderedJSON's error rather than
	// silently clobbering a file it couldn't safely parse.
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`not valid json`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := (jsonCodec{}).WriteServers(path, map[string]serverEntry{"s": {Command: "cmd"}})
	if err == nil {
		t.Fatal("expected error when existing file has invalid JSON")
	}
}

func TestJSONCodec_UpsertReplacesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	seed := `{"mcpServers":{"context7":{"command":"old"},"other":{"command":"keep"}}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := jsonCodec{}
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	servers["context7"] = serverEntry{Command: "npx", Args: []string{"-y", "ctx"}}
	if err := c.WriteServers(path, servers); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}

	got, _ := c.ReadServers(path)
	if got["context7"].Command != "npx" {
		t.Errorf("upsert failed for context7: %+v", got["context7"])
	}
	if got["other"].Command != "keep" {
		t.Errorf("sibling entry lost: %+v", got["other"])
	}
}
