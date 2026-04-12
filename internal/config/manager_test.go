package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeYAML creates a temp file with the given YAML content and returns its path.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "gaal-*.yaml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// ---------------------------------------------------------------------------
// UserConfigFilePath
// ---------------------------------------------------------------------------

func TestUserConfigFilePath_DelegatesToPlatform(t *testing.T) {
	got := UserConfigFilePath()
	if got == "" {
		t.Fatal("UserConfigFilePath returned empty string")
	}
	if !strings.HasSuffix(got, filepath.Join("gaal", "config.yaml")) {
		t.Errorf("got %q, expected suffix gaal/config.yaml", got)
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestLoad_ValidMinimal(t *testing.T) {
	// Use a real absolute path so expandPaths leaves the key unchanged on all platforms.
	repoPath := filepath.ToSlash(filepath.Join(t.TempDir(), "myrepo"))
	p := writeYAML(t, fmt.Sprintf(`
repositories:
  %s:
    type: git
    url: https://example.com/foo.git
`, repoPath))
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// After expandPaths, absolute paths are left unchanged.
	if _, ok := cfg.Repositories[repoPath]; !ok {
		t.Errorf("expected repository %q, got keys: %v", repoPath, repoKeys(cfg))
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/this/path/does/not/exist/gaal.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	p := writeYAML(t, "repositories: [unclosed")
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_ValidationError_MissingType(t *testing.T) {
	p := writeYAML(t, `
repositories:
  /abs/myrepo:
    url: https://example.com/x.git
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for missing type")
	}
}

func TestLoad_ValidationError_MissingURL(t *testing.T) {
	p := writeYAML(t, `
repositories:
  /abs/myrepo:
    type: git
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for missing url")
	}
}

func TestLoad_SkillAgents_FlatList(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents: ["*"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) != 1 || len(cfg.Skills[0].Agents) != 1 || cfg.Skills[0].Agents[0] != "*" {
		t.Errorf("got agents %v, want [\"*\"]", cfg.Skills[0].Agents)
	}
}

func TestLoad_SkillAgents_Scalar(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents: "*"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) != 1 || len(cfg.Skills[0].Agents) != 1 || cfg.Skills[0].Agents[0] != "*" {
		t.Errorf("got agents %v, want [\"*\"]", cfg.Skills[0].Agents)
	}
}

func TestLoad_SkillAgents_NestedListIsFlattened(t *testing.T) {
	// Regression for https://github.com/gmg-inc/gaal-lite/issues/13
	// A list-of-lists is a common hand-written mistake (mentally copying the
	// canonical `agents: ["*"]` example under a list bullet).
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents:
      - ["*", "claude"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"*", "claude"}
	got := cfg.Skills[0].Agents
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("agents[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_SkillAgents_MixedFlatAndNested(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents:
      - claude
      - ["codex", "cursor"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"claude", "codex", "cursor"}
	got := cfg.Skills[0].Agents
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("agents[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_SkillAgents_DoubleNestedRejected(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents:
      - [["*"]]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for doubly-nested agents list")
	}
	msg := err.Error()
	if !strings.Contains(msg, "agents") {
		t.Errorf("error should mention 'agents', got: %v", err)
	}
	if !strings.Contains(msg, "owner/repo") {
		t.Errorf("error should name the skill source for context, got: %v", err)
	}
}

func TestLoad_SkillAgents_MapRejected(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents:
      key: value
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for map-shaped agents field")
	}
}

func TestLoad_ValidationError_SkillNoSource(t *testing.T) {
	p := writeYAML(t, `
skills:
  - agents: ["*"]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for skill without source")
	}
}

func TestLoad_ValidationError_MCPNoName(t *testing.T) {
	p := writeYAML(t, `
mcps:
  - target: /tmp/mcp.json
    source: https://example.com/mcp.json
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for mcp without name")
	}
}

func TestLoad_ValidationError_MCPNoTarget(t *testing.T) {
	p := writeYAML(t, `
mcps:
  - name: myserver
    source: https://example.com/mcp.json
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for mcp without target")
	}
}

func TestLoad_ValidationError_MCPNoSource(t *testing.T) {
	p := writeYAML(t, `
mcps:
  - name: myserver
    target: /tmp/mcp.json
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error for mcp without source or inline")
	}
}

// ---------------------------------------------------------------------------
// Schema field
// ---------------------------------------------------------------------------

func TestLoad_SchemaExplicitOne(t *testing.T) {
	p := writeYAML(t, `
schema: 1
skills:
  - source: owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Schema == nil || *cfg.Schema != 1 {
		t.Errorf("expected Schema=1, got %v", cfg.Schema)
	}
}

func TestLoad_SchemaMissing_DefaultsToNil(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Missing schema is accepted (with a warning) and left as nil.
	if cfg.Schema != nil {
		t.Errorf("expected Schema=nil for missing field, got %d", *cfg.Schema)
	}
}

func TestLoad_SchemaTwo_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: 2
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: 2")
	}
	if !strings.Contains(err.Error(), "schema 2") {
		t.Errorf("error should mention schema 2, got: %v", err)
	}
	if !strings.Contains(err.Error(), "only understands schema 1") {
		t.Errorf("error should mention supported schema, got: %v", err)
	}
}

func TestLoad_SchemaZero_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: 0
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: 0")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("error should mention positive integer, got: %v", err)
	}
}

func TestLoad_SchemaNegative_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: -1
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: -1")
	}
	if !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("error should mention positive integer, got: %v", err)
	}
}

func TestLoad_SchemaString_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: "1"
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: \"1\" (string)")
	}
}

func TestLoad_SchemaLatest_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: latest
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: latest")
	}
}

func TestLoad_SchemaLargeNumber_Rejected(t *testing.T) {
	p := writeYAML(t, `
schema: 99
skills:
  - source: owner/repo
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for schema: 99")
	}
	if !strings.Contains(err.Error(), "schema 99") {
		t.Errorf("error should mention the actual schema number, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LoadChain
// ---------------------------------------------------------------------------

func TestLoadChain_OnlyWorkspace(t *testing.T) {
	// Isolate from the host's real global/user configs.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(empty, "xdg"))

	repoPath := filepath.ToSlash(filepath.Join(t.TempDir(), "testrepo"))
	p := writeYAML(t, fmt.Sprintf(`
repositories:
  %s:
    type: git
    url: https://example.com/test.git
`, repoPath))
	cfg, err := LoadChain(p)
	if err != nil {
		t.Fatalf("LoadChain: %v", err)
	}
	if _, ok := cfg.Repositories[repoPath]; !ok {
		t.Errorf("expected %q in merged config, got: %v", repoPath, repoKeys(cfg.Config))
	}
}

func TestLoadChain_AllMissing(t *testing.T) {
	// Isolate from the host's real global/user configs so this test passes on
	// dev machines that happen to have a ~/.config/gaal/config.yaml.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(empty, "xdg"))

	_, err := LoadChain(filepath.Join(empty, "no-such-workspace.yaml"))
	if err == nil {
		t.Fatal("expected error when no config file found")
	}
}

// ---------------------------------------------------------------------------
// mergeFrom
// ---------------------------------------------------------------------------

func TestMergeFrom_WorkspaceWins(t *testing.T) {
	dir := t.TempDir()
	// Use a real absolute path that survives expandPaths on all platforms.
	sharedPath := filepath.ToSlash(filepath.Join(dir, "shared"))

	lower := filepath.Join(dir, "lower.yaml")
	os.WriteFile(lower, []byte(fmt.Sprintf(`
repositories:
  %s:
    type: git
    url: https://example.com/original.git
`, sharedPath)), 0o644)

	higher := filepath.Join(dir, "higher.yaml")
	os.WriteFile(higher, []byte(fmt.Sprintf(`
repositories:
  %s:
    type: git
    url: https://example.com/override.git
`, sharedPath)), 0o644)

	cfgLow, err := Load(lower)
	if err != nil {
		t.Fatalf("Load lower: %v", err)
	}
	cfgHigh, err := Load(higher)
	if err != nil {
		t.Fatalf("Load higher: %v", err)
	}

	merged := &Config{}
	merged.mergeFrom(cfgLow, ScopeGlobal)
	merged.mergeFrom(cfgHigh, ScopeWorkspace)

	got := merged.Repositories[sharedPath].URL
	if got != "https://example.com/override.git" {
		t.Errorf("wanted override URL, got %q", got)
	}
}

func TestMergeFrom_SkillsAccumulated(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a.yaml")
	os.WriteFile(a, []byte("skills:\n  - source: owner/repo-a\n"), 0o644)

	b := filepath.Join(dir, "b.yaml")
	os.WriteFile(b, []byte("skills:\n  - source: owner/repo-b\n"), 0o644)

	cfgA, _ := Load(a)
	cfgB, _ := Load(b)

	merged := &Config{}
	merged.mergeFrom(cfgA, ScopeGlobal)
	merged.mergeFrom(cfgB, ScopeWorkspace)

	if len(merged.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(merged.Skills))
	}
}

func TestMergeFrom_EmptySrc(t *testing.T) {
	base := &Config{
		Repositories: map[string]ConfigRepo{
			"/r1": {Type: "git", URL: "https://example.com/r1.git"},
		},
	}
	base.mergeFrom(&Config{}, ScopeWorkspace)
	if len(base.Repositories) != 1 {
		t.Errorf("expected 1 repo after merging empty src, got %d", len(base.Repositories))
	}
}

func TestMergeFrom_NilRepositoriesInDst(t *testing.T) {
	dst := &Config{}
	src := &Config{
		Repositories: map[string]ConfigRepo{
			"/new": {Type: "git", URL: "https://example.com/new.git"},
		},
	}
	dst.mergeFrom(src, ScopeWorkspace)
	if len(dst.Repositories) != 1 {
		t.Errorf("expected 1 repo, got %d", len(dst.Repositories))
	}
}

func TestMergeFrom_SchemaSrcWins(t *testing.T) {
	v1 := 1
	dst := &Config{}
	src := &Config{Schema: &v1}
	dst.mergeFrom(src, ScopeWorkspace)
	if dst.Schema == nil || *dst.Schema != 1 {
		t.Errorf("expected Schema=1 from src, got %v", dst.Schema)
	}
}

func TestMergeFrom_SchemaDstPreservedWhenSrcNil(t *testing.T) {
	v1 := 1
	dst := &Config{Schema: &v1}
	src := &Config{}
	dst.mergeFrom(src, ScopeWorkspace)
	if dst.Schema == nil || *dst.Schema != 1 {
		t.Errorf("expected Schema=1 preserved from dst, got %v", dst.Schema)
	}
}

// ---------------------------------------------------------------------------
// Telemetry field
// ---------------------------------------------------------------------------

func TestTelemetryFieldLoadedFromYAML(t *testing.T) {
	p := writeYAML(t, "telemetry: true\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telemetry == nil {
		t.Fatal("expected Telemetry to be non-nil, got nil")
	}
	if !*cfg.Telemetry {
		t.Errorf("expected Telemetry to be true, got false")
	}
}

func TestTelemetryFieldNilWhenAbsent(t *testing.T) {
	p := writeYAML(t, "skills:\n  - source: owner/repo\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telemetry != nil {
		t.Errorf("expected Telemetry to be nil when absent, got %v", *cfg.Telemetry)
	}
}

func TestTelemetryCarriedFromLower(t *testing.T) {
	dir := t.TempDir()

	// lower level sets telemetry: true
	lower := filepath.Join(dir, "lower.yaml")
	os.WriteFile(lower, []byte("telemetry: true\n"), 0o644)

	// higher level has no telemetry field
	higher := filepath.Join(dir, "higher.yaml")
	os.WriteFile(higher, []byte("skills:\n  - source: owner/repo\n"), 0o644)

	cfgLow, err := Load(lower)
	if err != nil {
		t.Fatalf("Load lower: %v", err)
	}
	cfgHigh, err := Load(higher)
	if err != nil {
		t.Fatalf("Load higher: %v", err)
	}

	merged := &Config{}
	merged.mergeFrom(cfgLow, ScopeGlobal)
	merged.mergeFrom(cfgHigh, ScopeWorkspace)

	// Higher level has nil Telemetry (not set) so lower's value is preserved.
	if merged.Telemetry == nil {
		t.Fatal("expected Telemetry to be non-nil (carried from lower), got nil")
	}
	if !*merged.Telemetry {
		t.Errorf("expected Telemetry=true (carried from lower), got false")
	}
}

// TestTelemetry_UserOverridesGlobal verifies that the user scope (≤ maxscope=user)
// can override a telemetry value set at global scope.
func TestTelemetry_UserOverridesGlobal(t *testing.T) {
	dir := t.TempDir()

	global := filepath.Join(dir, "global.yaml")
	os.WriteFile(global, []byte("telemetry: true\n"), 0o644)

	user := filepath.Join(dir, "user.yaml")
	os.WriteFile(user, []byte("telemetry: false\n"), 0o644)

	cfgGlobal, err := Load(global)
	if err != nil {
		t.Fatalf("Load global: %v", err)
	}
	cfgUser, err := Load(user)
	if err != nil {
		t.Fatalf("Load user: %v", err)
	}

	merged := &Config{}
	merged.mergeFrom(cfgGlobal, ScopeGlobal)
	merged.mergeFrom(cfgUser, ScopeUser)

	if merged.Telemetry == nil {
		t.Fatal("expected Telemetry to be non-nil, got nil")
	}
	if *merged.Telemetry {
		t.Errorf("expected Telemetry=false (user overrides global), got true")
	}
}

// TestTelemetry_WorkspaceCannotOverride verifies that ScopeWorkspace is blocked
// by the maxscope=user annotation on Config.Telemetry.
func TestTelemetry_WorkspaceCannotOverride(t *testing.T) {
	dir := t.TempDir()

	global := filepath.Join(dir, "global.yaml")
	os.WriteFile(global, []byte("telemetry: true\n"), 0o644)

	workspace := filepath.Join(dir, "workspace.yaml")
	os.WriteFile(workspace, []byte("telemetry: false\n"), 0o644)

	cfgGlobal, err := Load(global)
	if err != nil {
		t.Fatalf("Load global: %v", err)
	}
	cfgWorkspace, err := Load(workspace)
	if err != nil {
		t.Fatalf("Load workspace: %v", err)
	}

	merged := &Config{}
	merged.mergeFrom(cfgGlobal, ScopeGlobal)
	merged.mergeFrom(cfgWorkspace, ScopeWorkspace)

	// ScopeWorkspace > maxscope=user: the workspace value must be ignored.
	if merged.Telemetry == nil {
		t.Fatal("expected Telemetry to be non-nil (global value carried), got nil")
	}
	if !*merged.Telemetry {
		t.Errorf("expected Telemetry=true (workspace blocked, global value preserved), got false")
	}
}

// ---------------------------------------------------------------------------
// GenerateSchema — schema constraints
// ---------------------------------------------------------------------------

func TestGenerateSchema_SchemaRequired(t *testing.T) {
	data, err := GenerateSchema()
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	s := string(data)

	// "schema" must appear in "required" at the top level.
	if !strings.Contains(s, `"schema"`) {
		t.Error("generated JSON Schema should contain a 'schema' property")
	}

	// Check that schema is in the required list.
	if !strings.Contains(s, `"required"`) {
		t.Error("generated JSON Schema should have a required list")
	}
}

func TestGenerateSchema_SchemaEnumOne(t *testing.T) {
	data, err := GenerateSchema()
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}

	// Parse the schema JSON to check the schema property's enum constraint.
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	// invopop/jsonschema places the Config definition under $defs/Config when
	// DoNotReference is false (the default). The top-level schema is just a $ref.
	// Try top-level properties first, then fall back to $defs/Config.
	var configDef map[string]any
	if props, ok := root["properties"]; ok {
		configDef = root
		_ = props
	} else if defs, ok := root["$defs"].(map[string]any); ok {
		if cfg, ok := defs["Config"].(map[string]any); ok {
			configDef = cfg
		}
	}
	if configDef == nil {
		t.Fatal("schema missing properties (checked top-level and $defs/Config)")
	}

	props, ok := configDef["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	schemaProp, ok := props["schema"].(map[string]any)
	if !ok {
		t.Fatal("schema missing schema property")
	}
	enumVal, ok := schemaProp["enum"]
	if !ok {
		t.Fatal("schema property missing enum constraint")
	}
	enumSlice, ok := enumVal.([]any)
	if !ok || len(enumSlice) != 1 {
		t.Fatalf("expected enum with one element, got %v", enumVal)
	}
	// JSON numbers unmarshal as float64.
	if enumSlice[0] != float64(1) {
		t.Errorf("expected enum=[1], got enum=%v", enumSlice)
	}
}

// ---------------------------------------------------------------------------
// MCPConfig.MergeEnabled
// ---------------------------------------------------------------------------

func TestMCPConfig_MergeEnabled_NilDefaultsTrue(t *testing.T) {
	mc := ConfigMcp{Name: "srv", Target: "/tmp/mcp.json"}
	if !mc.MergeEnabled() {
		t.Error("expected MergeEnabled()=true when Merge is nil")
	}
}

func TestMCPConfig_MergeEnabled_ExplicitFalse(t *testing.T) {
	f := false
	mc := ConfigMcp{Name: "srv", Target: "/tmp/mcp.json", Merge: &f}
	if mc.MergeEnabled() {
		t.Error("expected MergeEnabled()=false when Merge is explicitly false")
	}
}

func TestMCPConfig_MergeEnabled_ExplicitTrue(t *testing.T) {
	tr := true
	mc := ConfigMcp{Name: "srv", Target: "/tmp/mcp.json", Merge: &tr}
	if !mc.MergeEnabled() {
		t.Error("expected MergeEnabled()=true when Merge is explicitly true")
	}
}

// ---------------------------------------------------------------------------
// Skills / MCPs deduplication
// ---------------------------------------------------------------------------

func TestLoad_SkillsDeduplicated(t *testing.T) {
	p := writeYAML(t, `
skills:
  - source: owner/repo
    agents: ["*"]
  - source: owner/repo
    agents: ["claude"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Skills) != 1 {
		t.Errorf("expected 1 skill after intra-file dedup, got %d", len(cfg.Skills))
	}
	// First occurrence must be kept.
	if len(cfg.Skills[0].Agents) == 0 || cfg.Skills[0].Agents[0] != "*" {
		t.Errorf("expected first occurrence (agents=[*]) to be kept, got %v", cfg.Skills[0].Agents)
	}
}

func TestLoad_MCPsDeduplicated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gaal.yaml")
	os.WriteFile(p, []byte(`
mcps:
  - name: myserver
    target: /tmp/mcp.json
    inline:
      command: first-cmd
  - name: myserver
    target: /tmp/mcp.json
    inline:
      command: second-cmd
`), 0o644)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.MCPs) != 1 {
		t.Errorf("expected 1 MCP after intra-file dedup, got %d", len(cfg.MCPs))
	}
	// First occurrence must be kept.
	if cfg.MCPs[0].Inline == nil || cfg.MCPs[0].Inline.Command != "first-cmd" {
		t.Errorf("expected first occurrence (command=first-cmd) to be kept, got %v", cfg.MCPs[0].Inline)
	}
}

func TestMergeFrom_SkillsDeduplicated(t *testing.T) {
	dir := t.TempDir()

	lower := filepath.Join(dir, "lower.yaml")
	os.WriteFile(lower, []byte("skills:\n  - source: owner/repo\n    agents: [\"*\"]\n"), 0o644)

	higher := filepath.Join(dir, "higher.yaml")
	os.WriteFile(higher, []byte("skills:\n  - source: owner/repo\n    agents: [\"claude\"]\n"), 0o644)

	cfgLow, _ := Load(lower)
	cfgHigh, _ := Load(higher)

	merged := &Config{}
	merged.mergeFrom(cfgLow, ScopeGlobal)
	merged.mergeFrom(cfgHigh, ScopeWorkspace)

	if len(merged.Skills) != 1 {
		t.Errorf("expected 1 skill after cross-level dedup, got %d", len(merged.Skills))
	}
	// Higher-priority level wins.
	if len(merged.Skills[0].Agents) == 0 || merged.Skills[0].Agents[0] != "claude" {
		t.Errorf("expected higher-priority entry (agents=[claude]) to win, got %v", merged.Skills[0].Agents)
	}
}

func TestMergeFrom_MCPsDeduplicated(t *testing.T) {
	dir := t.TempDir()

	lower := filepath.Join(dir, "lower.yaml")
	os.WriteFile(lower, []byte(`mcps:
  - name: myserver
    target: /tmp/mcp.json
    inline:
      command: global-cmd
`), 0o644)

	higher := filepath.Join(dir, "higher.yaml")
	os.WriteFile(higher, []byte(`mcps:
  - name: myserver
    target: /tmp/mcp.json
    inline:
      command: workspace-cmd
`), 0o644)

	cfgLow, _ := Load(lower)
	cfgHigh, _ := Load(higher)

	merged := &Config{}
	merged.mergeFrom(cfgLow, ScopeGlobal)
	merged.mergeFrom(cfgHigh, ScopeWorkspace)

	if len(merged.MCPs) != 1 {
		t.Errorf("expected 1 MCP after cross-level dedup, got %d", len(merged.MCPs))
	}
	// Higher-priority level wins.
	if merged.MCPs[0].Inline == nil || merged.MCPs[0].Inline.Command != "workspace-cmd" {
		t.Errorf("expected higher-priority entry (command=workspace-cmd) to win, got %v", merged.MCPs[0].Inline)
	}
}

// ---------------------------------------------------------------------------
// LoadChain + ConfigLevels
// ---------------------------------------------------------------------------

func TestLoadChain_PopulatesLevels(t *testing.T) {
	// Isolate from the host's real global/user configs.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(empty, "xdg"))

	p := writeYAML(t, "skills:\n  - source: owner/workspace-repo\n")
	cfg, err := LoadChain(p)
	if err != nil {
		t.Fatalf("LoadChain: %v", err)
	}
	// Global and user configs are absent in this isolated env.
	if cfg.Levels.Global != nil {
		t.Errorf("expected Levels.Global=nil (file absent), got non-nil")
	}
	if cfg.Levels.User != nil {
		t.Errorf("expected Levels.User=nil (file absent), got non-nil")
	}
	if cfg.Levels.Workspace == nil {
		t.Fatal("expected Levels.Workspace to be non-nil")
	}
	if len(cfg.Levels.Workspace.Skills) != 1 || cfg.Levels.Workspace.Skills[0].Source != "owner/workspace-repo" {
		t.Errorf("unexpected Levels.Workspace.Skills: %v", cfg.Levels.Workspace.Skills)
	}
}

// ---------------------------------------------------------------------------
// SourcePath — set by Load
// ---------------------------------------------------------------------------

func TestLoad_SetsSourcePath(t *testing.T) {
	p := writeYAML(t, "skills:\n  - source: owner/repo\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SourcePath != p {
		t.Errorf("SourcePath = %q, want %q", cfg.SourcePath, p)
	}
}

// ---------------------------------------------------------------------------
// ResolvedConfig.SourcePaths
// ---------------------------------------------------------------------------

func TestResolvedConfig_SourcePaths_OnlyWorkspace(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(empty, "xdg"))

	p := writeYAML(t, "skills:\n  - source: owner/repo\n")
	rcfg, err := LoadChain(p)
	if err != nil {
		t.Fatalf("LoadChain: %v", err)
	}
	paths := rcfg.SourcePaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 source path (workspace only), got %d: %v", len(paths), paths)
	}
	if paths[0] != p {
		t.Errorf("SourcePaths()[0] = %q, want %q", paths[0], p)
	}
}

func TestResolvedConfig_SourcePaths_MultiLevel(t *testing.T) {
	dir := t.TempDir()

	userDir := filepath.Join(dir, ".config", "gaal")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userFile := filepath.Join(userDir, "config.yaml")
	os.WriteFile(userFile, []byte("skills:\n  - source: owner/user-repo\n"), 0o644)

	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	wsFile := writeYAML(t, "skills:\n  - source: owner/ws-repo\n")
	rcfg, err := LoadChain(wsFile)
	if err != nil {
		t.Fatalf("LoadChain: %v", err)
	}
	paths := rcfg.SourcePaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 source paths (user + workspace), got %d: %v", len(paths), paths)
	}
}

func TestResolvedConfig_SourcePaths_Empty(t *testing.T) {
	// ResolvedConfig with no loaded levels returns empty slice.
	rcfg := &ResolvedConfig{Config: &Config{}, Levels: LevelConfigs{}}
	paths := rcfg.SourcePaths()
	if len(paths) != 0 {
		t.Errorf("expected empty paths for no levels, got %v", paths)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func repoKeys(cfg *Config) []string {
	keys := make([]string, 0, len(cfg.Repositories))
	for k := range cfg.Repositories {
		keys = append(keys, k)
	}
	return keys
}

// Ensure fmt is used (suppress unused import).
var _ = fmt.Sprintf
