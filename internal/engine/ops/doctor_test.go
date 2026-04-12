package ops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaal/internal/config"
)

func TestDoctorCleanConfig(t *testing.T) {
	cfg := &config.Config{}
	ws := &config.Config{SourcePath: "gaal.yaml"} // Schema nil in workspace
	report := RunDoctor(cfg, DoctorOptions{Offline: true, Levels: config.LevelConfigs{Workspace: ws}})

	// Workspace config missing schema → warning → exit code 1.
	if report.ExitCode != 1 {
		t.Errorf("expected exit code 1 for config without schema, got %d", report.ExitCode)
	}
}

func TestDoctorSchemaMissing_Warning(t *testing.T) {
	cfg := &config.Config{}                       // merged schema is nil
	ws := &config.Config{SourcePath: "gaal.yaml"} // Schema nil in workspace
	report := RunDoctor(cfg, DoctorOptions{Offline: true, Levels: config.LevelConfigs{Workspace: ws}})

	found := false
	for _, f := range report.Findings {
		if f.Section == "config" && f.Severity == SeverityWarning && strings.Contains(f.Message, "schema") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about missing schema, findings: %+v", report.Findings)
	}
}

func TestDoctorSchemaOne_Info(t *testing.T) {
	v := 1
	cfg := &config.Config{Schema: &v}
	ws := &config.Config{Schema: &v, SourcePath: "gaal.yaml"}
	report := RunDoctor(cfg, DoctorOptions{Offline: true, Levels: config.LevelConfigs{Workspace: ws}})

	found := false
	for _, f := range report.Findings {
		if f.Section == "config" && f.Severity == SeverityInfo && strings.Contains(f.Message, "schema: 1") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected info about schema 1, findings: %+v", report.Findings)
	}
}

func TestDoctorCleanConfigWithSchema(t *testing.T) {
	v := 1
	cfg := &config.Config{Schema: &v}
	ws := &config.Config{Schema: &v, SourcePath: "gaal.yaml"}
	report := RunDoctor(cfg, DoctorOptions{Offline: true, Levels: config.LevelConfigs{Workspace: ws}})

	if report.ExitCode != 0 {
		t.Errorf("expected exit code 0 for clean config with schema, got %d", report.ExitCode)
	}
}

// TestDoctorSchema_MissingInOneLevel is the case the old implementation got
// wrong: the global level carries schema but the workspace level does not.
// Only the workspace file should trigger a warning; no global warning.
func TestDoctorSchema_MissingInOneLevel(t *testing.T) {
	v := 1
	globalCfg := &config.Config{Schema: &v, SourcePath: "/etc/gaal/config.yaml"}
	wsCfg := &config.Config{SourcePath: "gaal.yaml"} // workspace missing schema
	merged := &config.Config{Schema: &v}             // merged inherits from global

	levels := config.LevelConfigs{Global: globalCfg, Workspace: wsCfg}
	report := RunDoctor(merged, DoctorOptions{Offline: true, Levels: levels})

	var schemaFindings []Finding
	for _, f := range report.Findings {
		if f.Section == "config" && strings.Contains(f.Message, "schema") {
			schemaFindings = append(schemaFindings, f)
		}
	}
	if len(schemaFindings) != 1 {
		t.Fatalf("expected exactly 1 schema finding (workspace only), got %d: %+v", len(schemaFindings), schemaFindings)
	}
	if schemaFindings[0].Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %s", schemaFindings[0].Severity)
	}
	if !strings.Contains(schemaFindings[0].Message, "gaal.yaml") {
		t.Errorf("expected finding to reference workspace file, got: %s", schemaFindings[0].Message)
	}
}

func TestDoctorSkillSourceLocalMissing(t *testing.T) {
	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: "/nonexistent/path/that/does/not/exist", Agents: []string{"claude-code"}},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	if report.ExitCode != 2 {
		t.Errorf("expected exit code 2, got %d", report.ExitCode)
	}

	found := false
	for _, f := range report.Findings {
		if f.Severity == SeverityError && strings.Contains(f.Message, "/nonexistent/path") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error finding about missing local path")
	}
}

func TestDoctorSkillSourceLocalExists(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: dir, Agents: []string{"claude-code"}},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	for _, f := range report.Findings {
		if f.Severity == SeverityError && f.Section == "skills" {
			t.Errorf("unexpected error finding in skills section: %s", f.Message)
		}
	}
}

func TestDoctorSkillSourceRemoteReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: srv.URL, Agents: []string{"claude-code"}},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: false})

	for _, f := range report.Findings {
		if f.Severity == SeverityError && f.Section == "skills" {
			t.Errorf("unexpected error finding in skills section: %s", f.Message)
		}
	}
}

func TestDoctorSkillSourceRemoteUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: srv.URL, Agents: []string{"claude-code"}},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: false})

	found := false
	for _, f := range report.Findings {
		if f.Severity == SeverityError && f.Section == "skills" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error finding for unreachable remote source")
	}
}

func TestDoctorOfflineSkipsNetwork(t *testing.T) {
	cfg := &config.Config{
		Skills: []config.ConfigSkill{
			{Source: "owner/repo", Agents: []string{"claude-code"}},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	for _, f := range report.Findings {
		if f.Severity == SeverityError && f.Section == "skills" {
			t.Errorf("unexpected error finding in skills section (offline should skip network): %s", f.Message)
		}
	}
}

func TestDoctorMCPTargetValid(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`{"mcpServers": {}}`), 0o644)

	cfg := &config.Config{
		MCPs: []config.ConfigMcp{
			{Name: "test", Target: target},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	for _, f := range report.Findings {
		if f.Severity == SeverityError && f.Section == "mcps" {
			t.Errorf("unexpected error finding in mcps section: %s", f.Message)
		}
	}
}

func TestDoctorMCPTargetInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`not valid json`), 0o644)

	cfg := &config.Config{
		MCPs: []config.ConfigMcp{
			{Name: "test", Target: target},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	found := false
	for _, f := range report.Findings {
		if f.Severity == SeverityWarning && f.Section == "mcps" && strings.Contains(f.Message, "invalid JSON") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning finding about invalid JSON")
	}
}

func TestDoctorMCPTargetOutsideHome(t *testing.T) {
	// Use a target path under /tmp which is outside any reasonable $HOME.
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`{}`), 0o644)

	// Set HOME to a different directory so the target is "outside $HOME".
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		MCPs: []config.ConfigMcp{
			{Name: "test", Target: target},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	found := false
	for _, f := range report.Findings {
		if f.Severity == SeverityWarning && f.Section == "mcps" && strings.Contains(f.Message, "outside $HOME") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about target outside $HOME, findings: %+v", report.Findings)
	}
}

func TestDoctorExitCodeWarnings(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	os.WriteFile(target, []byte(`not json`), 0o644)

	cfg := &config.Config{
		MCPs: []config.ConfigMcp{
			{Name: "test", Target: target},
		},
	}
	report := RunDoctor(cfg, DoctorOptions{Offline: true})

	// Should have at least one warning and no errors in the mcps section.
	hasWarning := false
	hasError := false
	for _, f := range report.Findings {
		if f.Severity == SeverityWarning {
			hasWarning = true
		}
		if f.Severity == SeverityError {
			hasError = true
		}
	}
	if !hasWarning {
		t.Error("expected at least one warning finding")
	}
	if hasError {
		t.Error("unexpected error finding — wanted warnings only for exit code 1")
	}
	if report.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", report.ExitCode)
	}
}

func TestDoctorReportJSON(t *testing.T) {
	report := &DoctorReport{
		Findings: []Finding{
			{Section: "telemetry", Severity: SeverityInfo, Message: "enabled via GAAL_TELEMETRY=1"},
		},
		ExitCode: 0,
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded DoctorReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(decoded.Findings))
	}
	if decoded.Findings[0].Section != "telemetry" {
		t.Errorf("expected section=telemetry, got %q", decoded.Findings[0].Section)
	}
	if decoded.Findings[0].Severity != SeverityInfo {
		t.Errorf("expected severity=info, got %q", decoded.Findings[0].Severity)
	}
	if decoded.ExitCode != 0 {
		t.Errorf("expected exit_code=0, got %d", decoded.ExitCode)
	}
}

func TestResolveSkillURL(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"owner/repo", "https://github.com/owner/repo"},
		{"https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"git@github.com:owner/repo.git", "https://github.com/owner/repo"},
		{"ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"git@gitlab.com:team/project.git", "https://gitlab.com/team/project"},
	}
	for _, tt := range tests {
		got := resolveSkillURL(tt.source)
		if got != tt.expected {
			t.Errorf("resolveSkillURL(%q) = %q, want %q", tt.source, got, tt.expected)
		}
	}
}

func TestIsRemoteSource(t *testing.T) {
	tests := []struct {
		source   string
		expected bool
	}{
		{"owner/repo", true},
		{"https://github.com/owner/repo", true},
		{"git@github.com:owner/repo.git", true},
		{"ssh://git@github.com/owner/repo.git", true},
		{"/absolute/path", false},
		{"./relative/path", false},
		{"../parent/path", false},
		{"~/home/path", false},
	}
	for _, tt := range tests {
		got := isRemoteSource(tt.source)
		if got != tt.expected {
			t.Errorf("isRemoteSource(%q) = %v, want %v", tt.source, got, tt.expected)
		}
	}
}
