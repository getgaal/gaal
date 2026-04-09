package agent

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Info describes the file-system layout for a coding agent: where skills are
// installed and where the MCP server configuration file lives.
type Info struct {
	// ProjectSkillsDir is the skills directory relative to the project root.
	ProjectSkillsDir string
	// GlobalSkillsDir is the skills directory under the user home directory (~).
	GlobalSkillsDir string
	// MCPConfigFile is the path to the agent's MCP server configuration file,
	// relative to the user home directory (~). Empty when unknown or unsupported.
	MCPConfigFile string
}

// agentEntry is the YAML-decodable shape for a single agent.
type agentEntry struct {
	ProjectSkillsDir string `yaml:"project_skills_dir"`
	GlobalSkillsDir  string `yaml:"global_skills_dir"`
	MCPConfigFile    string `yaml:"mcp_config_file"`
}

// agentsFile is the top-level structure of agents.yaml.
type agentsFile struct {
	Agents map[string]agentEntry `yaml:"agents"`
}

//go:embed agents.yaml
var builtinAgentsFS embed.FS

// registry holds the merged set of built-in + user-defined agents.
// Populated once at startup by init().
var registry = map[string]Info{}

func init() {
	data, err := builtinAgentsFS.ReadFile("agents.yaml")
	if err != nil {
		// Unreachable at runtime (file is embedded), but guard against
		// broken builds.
		panic("agent: cannot read embedded agents.yaml: " + err.Error())
	}
	if err := loadInto(data, registry, false); err != nil {
		panic("agent: invalid embedded agents.yaml: " + err.Error())
	}

	// Optionally load user-defined agents from the OS config directory.
	// Missing file is silently ignored; parse errors are logged and skipped.
	if userPath, ok := userAgentsPath(); ok {
		slog.Debug("loading user agents file", "path", userPath)
		userData, err := os.ReadFile(userPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn("cannot read user agents file", "path", userPath, "err", err)
			}
		} else {
			if err := loadInto(userData, registry, true); err != nil {
				slog.Warn("invalid user agents file, skipping", "path", userPath, "err", err)
			}
		}
	}
}

// loadInto parses YAML data and merges entries into dst.
// When allowOverride is false, duplicate names cause an error.
func loadInto(data []byte, dst map[string]Info, allowOverride bool) error {
	var af agentsFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	for name, e := range af.Agents {
		if err := validateEntry(name, e); err != nil {
			return err
		}
		if _, exists := dst[name]; exists && !allowOverride {
			return fmt.Errorf("duplicate agent name %q", name)
		}
		dst[name] = Info{
			ProjectSkillsDir: e.ProjectSkillsDir,
			GlobalSkillsDir:  e.GlobalSkillsDir,
			MCPConfigFile:    e.MCPConfigFile,
		}
	}
	return nil
}

// validateEntry enforces security constraints on agent path fields:
//   - project_skills_dir must be relative and contain no ".." segments
//   - global_skills_dir must start with "~/" or "~\" (home-relative)
//   - mcp_config_file must be empty OR start with "~/" or "~\"
func validateEntry(name string, e agentEntry) error {
	slog.Debug("validating agent entry", "name", name)

	if filepath.IsAbs(e.ProjectSkillsDir) {
		return fmt.Errorf("agent %q: project_skills_dir must be relative, got %q", name, e.ProjectSkillsDir)
	}
	if containsDotDot(e.ProjectSkillsDir) {
		return fmt.Errorf("agent %q: project_skills_dir must not contain '..', got %q", name, e.ProjectSkillsDir)
	}
	if !strings.HasPrefix(e.GlobalSkillsDir, "~/") && !strings.HasPrefix(e.GlobalSkillsDir, `~\`) {
		return fmt.Errorf("agent %q: global_skills_dir must start with '~/', got %q", name, e.GlobalSkillsDir)
	}
	if e.MCPConfigFile != "" &&
		!strings.HasPrefix(e.MCPConfigFile, "~/") &&
		!strings.HasPrefix(e.MCPConfigFile, `~\`) {
		return fmt.Errorf("agent %q: mcp_config_file must be empty or start with '~/', got %q", name, e.MCPConfigFile)
	}
	return nil
}

// containsDotDot reports whether p contains a ".." path segment.
func containsDotDot(p string) bool {
	for _, seg := range strings.FieldsFunc(filepath.ToSlash(p), func(r rune) bool { return r == '/' }) {
		if seg == ".." {
			return true
		}
	}
	return false
}

// userAgentsPath returns the path to the optional user agents config file.
func userAgentsPath() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(dir, "gaal", "agents.yaml"), true
}

// Names returns all supported agent identifiers.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// Entry pairs an agent name with its registry Info.
type Entry struct {
	Name string
	Info Info
}

// List returns all registered agents sorted by name.
func List() []Entry {
	names := Names()
	sort.Strings(names)
	entries := make([]Entry, len(names))
	for i, n := range names {
		entries[i] = Entry{Name: n, Info: registry[n]}
	}
	return entries
}

// Lookup returns the Info for name and whether it was found.
func Lookup(name string) (Info, bool) {
	info, ok := registry[name]
	return info, ok
}

// SkillDir returns the target skills directory for the given agent.
// If global is true the user-home directory is returned (~ expanded).
func SkillDir(name string, global bool, home string) (string, bool) {
	info, ok := registry[name]
	if !ok {
		return "", false
	}
	if global {
		return ExpandHome(info.GlobalSkillsDir, home), true
	}
	return info.ProjectSkillsDir, true
}

// MCPConfigPath returns the absolute path to the agent's MCP configuration
// file (home expanded). Returns ("", false) when not known for this agent.
func MCPConfigPath(name, home string) (string, bool) {
	info, ok := registry[name]
	if !ok || info.MCPConfigFile == "" {
		return "", false
	}
	return ExpandHome(info.MCPConfigFile, home), true
}

// ExpandHome expands a leading ~/ or ~\ to the provided home directory.
func ExpandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}
