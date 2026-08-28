package mcp

import (
	"testing"
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

func TestCodecFor_DispatchesOpencodeByFilename(t *testing.T) {
	// opencode.json is only distinguishable from every other agent's
	// mcpServers-keyed JSON by its file name, not its extension.
	cases := []struct {
		path string
		want string
	}{
		{"/x/opencode.json", "opencode"},
		{"/x/OPENCODE.JSON", "opencode"}, // case-insensitive
		{"/x/opencode.jsonc", "opencode"},
		{"/x/.config/opencode/opencode.json", "opencode"},
		{"/x/config.json", "json"}, // the old (wrong) path must NOT dispatch as opencode
		{"/x/.mcp.json", "json"},
		{"/x/config.toml", "toml"},
	}
	for _, tc := range cases {
		codec := codecFor(tc.path)
		got := "json"
		switch codec.(type) {
		case tomlCodec:
			got = "toml"
		case opencodeCodec:
			got = "opencode"
		}
		if got != tc.want {
			t.Errorf("codecFor(%q) = %s, want %s", tc.path, got, tc.want)
		}
	}
}
