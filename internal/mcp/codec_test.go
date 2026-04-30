package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// codecFor dispatch ----------------------------------------------------------

func TestCodecFor_DispatchesByExtension(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/x/claude_desktop_config.json", "json"},
		{"/x/config.toml", "toml"},
		{"/x/CONFIG.TOML", "toml"}, // case-insensitive
		{"/x/no-extension", "json"},
	}
	for _, tc := range cases {
		got := "json"
		if _, ok := codecFor(tc.path).(tomlCodec); ok {
			got = "toml"
		}
		if got != tc.want {
			t.Errorf("codecFor(%q) = %s, want %s", tc.path, got, tc.want)
		}
	}
}

// TOML codec -----------------------------------------------------------------

func TestTOMLCodec_RoundTripPreservesUnrelatedKeys(t *testing.T) {
	// Mimic a real ~/.codex/config.toml that already carries user settings
	// gaal must not stomp.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := `model = "gpt-5.4"
sandbox = "macos"
approval_policy = "on-request"

[analytics]
enabled = true

[features]
child_agents_md = true
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := tomlCodec{}
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil servers map, got %v", servers)
	}

	servers = map[string]serverEntry{
		"context7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@latest"}},
	}
	if err := c.WriteServers(path, servers); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse back: %v", err)
	}

	// Unrelated keys must survive.
	if doc["model"] != "gpt-5.4" {
		t.Errorf("model lost: %v", doc["model"])
	}
	if doc["sandbox"] != "macos" {
		t.Errorf("sandbox lost: %v", doc["sandbox"])
	}
	if doc["approval_policy"] != "on-request" {
		t.Errorf("approval_policy lost: %v", doc["approval_policy"])
	}
	analytics, ok := doc["analytics"].(map[string]any)
	if !ok || analytics["enabled"] != true {
		t.Errorf("[analytics] table lost: %v", doc["analytics"])
	}
	features, ok := doc["features"].(map[string]any)
	if !ok || features["child_agents_md"] != true {
		t.Errorf("[features] table lost: %v", doc["features"])
	}

	// Server entry must be the codex-expected shape.
	servers, err = c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers (after write): %v", err)
	}
	got, ok := servers["context7"]
	if !ok {
		t.Fatalf("context7 missing after round-trip; got %v", servers)
	}
	if got.Command != "npx" {
		t.Errorf("command = %q, want npx", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "-y" || got.Args[1] != "@upstash/context7-mcp@latest" {
		t.Errorf("args = %v, want [-y @upstash/context7-mcp@latest]", got.Args)
	}

	// And the rendered file must contain the [mcp_servers.context7] header
	// so codex's parser actually sees it.
	if !strings.Contains(string(data), "mcp_servers") {
		t.Errorf("rendered TOML missing mcp_servers table:\n%s", data)
	}
}

func TestTOMLCodec_UpsertReplacesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	seed := `[mcp_servers.context7]
command = "old"
args = ["x"]

[mcp_servers.other]
command = "keep"
`
	os.WriteFile(path, []byte(seed), 0o644)

	c := tomlCodec{}
	servers, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	if servers["context7"].Command != "old" {
		t.Fatalf("seed not parsed: %+v", servers)
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

func TestTOMLCodec_MissingFileReturnsNil(t *testing.T) {
	servers, err := tomlCodec{}.ReadServers(filepath.Join(t.TempDir(), "no-such.toml"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil servers map, got %v", servers)
	}
}

func TestTOMLCodec_PreservesEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	c := tomlCodec{}
	in := map[string]serverEntry{
		"with-env": {
			Command: "node",
			Args:    []string{"server.js"},
			Env:     map[string]string{"API_KEY": "secret", "DEBUG": "1"},
		},
	}
	if err := c.WriteServers(path, in); err != nil {
		t.Fatalf("WriteServers: %v", err)
	}
	out, err := c.ReadServers(path)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	got := out["with-env"]
	if got.Env["API_KEY"] != "secret" || got.Env["DEBUG"] != "1" {
		t.Errorf("env not preserved: %+v", got.Env)
	}
}

// End-to-end via mergeIntoTarget --------------------------------------------

func TestMergeIntoTarget_TOMLDispatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, []byte("model = \"gpt-5.4\"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	entry := serverEntry{Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@latest"}}
	if err := mergeIntoTarget(target, "context7", entry); err != nil {
		t.Fatalf("mergeIntoTarget: %v", err)
	}

	// File must still parse as TOML — not JSON.
	data, _ := os.ReadFile(target)
	if json.Valid(data) && !strings.Contains(string(data), "mcp_servers") {
		t.Errorf("target was rewritten as JSON; got:\n%s", data)
	}

	servers, err := tomlCodec{}.ReadServers(target)
	if err != nil {
		t.Fatalf("ReadServers: %v", err)
	}
	if servers["context7"].Command != "npx" {
		t.Errorf("context7 not written: %+v", servers)
	}
}
