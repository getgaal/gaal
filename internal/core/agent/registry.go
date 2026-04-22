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

	"gaal/internal/config/platform"

	"gopkg.in/yaml.v3"
)

// Info describes the file-system layout for a coding agent.
//
// Fields ending in *Dir are managed by "gaal sync" (install targets).
// Fields ending in *Search are scanned by "gaal audit" (discovery only).
type Info struct {
	// ProjectSkillsDir is the skills directory relative to the project root.
	// Managed by gaal sync when global: true.
	ProjectSkillsDir string
	// GlobalSkillsDir is the skills directory under the user home directory (~).
	// Managed by gaal sync when global: true.
	GlobalSkillsDir string
	// GlobalMCPConfigFile is the user-global MCP server configuration file
	// (~/ prefix). This is the agent's main MCP config (e.g. claude_desktop_config.json).
	// Managed by gaal sync when global: true.
	GlobalMCPConfigFile string
	// ProjectMCPConfigFile is the workspace-scoped MCP server configuration file
	// (~/ prefix). Empty when the agent has no workspace-level MCP config.
	// Managed by gaal sync when global: false.
	ProjectMCPConfigFile string

	// ProjectSkillsSearch is the list of project-relative directories to scan
	// for SKILL.md files during "gaal audit". Scanned at 1 level deep.
	// When empty, ProjectSkillsDir is used as the sole search path.
	ProjectSkillsSearch []string
	// GlobalSkillsSearch is the list of home-relative directories (~/ prefix)
	// to scan for SKILL.md files during "gaal audit". Scanned at 1 level deep.
	// When empty, GlobalSkillsDir is used as the sole search path.
	GlobalSkillsSearch []string
	// PmSkillsSearch is the list of home-relative directories (~/ prefix)
	// installed by the agent's package manager. Scanned recursively for
	// sub-trees containing a "skills/" folder with SKILL.md files.
	PmSkillsSearch []string

	// SupportsGenericProject reports whether the agent natively reads
	// project-level skills from the shared .agents/skills convention
	// owned by the "generic" built-in agent. When true, the agent's own
	// ProjectSkillsDir may be empty; sync and audit transparently
	// delegate to generic's project path.
	SupportsGenericProject bool
	// SupportsGenericGlobal reports whether the agent natively reads
	// global skills from the shared ~/.agents/skills convention owned by
	// the "generic" built-in agent. When true, the agent's own
	// GlobalSkillsDir may be empty; sync and audit transparently
	// delegate to generic's global path.
	SupportsGenericGlobal bool
}

// agentEntry is the YAML-decodable shape for a single agent.
type agentEntry struct {
	ProjectSkillsDir       string   `yaml:"project_skills_dir"`
	GlobalSkillsDir        string   `yaml:"global_skills_dir"`
	GlobalMCPConfigFile    string   `yaml:"global_mcp_config_file"`
	ProjectMCPConfigFile   string   `yaml:"project_mcp_config_file"`
	ProjectSkillsSearch    []string `yaml:"project_skills_search"`
	GlobalSkillsSearch     []string `yaml:"global_skills_search"`
	PmSkillsSearch         []string `yaml:"pm_skills_search"`
	SupportsGenericProject bool     `yaml:"supports_generic_project"`
	SupportsGenericGlobal  bool     `yaml:"supports_generic_global"`
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
			ProjectSkillsDir:       e.ProjectSkillsDir,
			GlobalSkillsDir:        e.GlobalSkillsDir,
			GlobalMCPConfigFile:    e.GlobalMCPConfigFile,
			ProjectMCPConfigFile:   e.ProjectMCPConfigFile,
			ProjectSkillsSearch:    e.ProjectSkillsSearch,
			GlobalSkillsSearch:     e.GlobalSkillsSearch,
			PmSkillsSearch:         e.PmSkillsSearch,
			SupportsGenericProject: e.SupportsGenericProject,
			SupportsGenericGlobal:  e.SupportsGenericGlobal,
		}
	}
	return nil
}

// validateEntry enforces security constraints on agent path fields:
//   - project_skills_dir must be relative and contain no ".." segments
//   - project_skills_search entries must be relative and contain no ".." segments
//   - global_skills_dir must start with "~/" or "~\" (home-relative)
//   - global_mcp_config_file must be empty OR start with "~/" or "~\"
//   - project_mcp_config_file must be empty OR start with "~/" or "~\"
//   - global_skills_search and pm_skills_search entries must start with "~/" or "~\"
func validateEntry(name string, e agentEntry) error {
	slog.Debug("validating agent entry", "name", name)

	// project_skills_dir: may be empty when the agent delegates project
	// skills to the "generic" convention; otherwise must be relative.
	if e.ProjectSkillsDir == "" {
		if !e.SupportsGenericProject {
			return fmt.Errorf("agent %q: project_skills_dir must be set unless supports_generic_project is true", name)
		}
	} else {
		if isAbsPath(e.ProjectSkillsDir) {
			return fmt.Errorf("agent %q: project_skills_dir must be relative, got %q", name, e.ProjectSkillsDir)
		}
		if containsDotDot(e.ProjectSkillsDir) {
			return fmt.Errorf("agent %q: project_skills_dir must not contain '..', got %q", name, e.ProjectSkillsDir)
		}
	}

	// global_skills_dir: may be empty when the agent delegates global
	// skills to the "generic" convention; otherwise must be ~/-prefixed.
	if e.GlobalSkillsDir == "" {
		if !e.SupportsGenericGlobal {
			return fmt.Errorf("agent %q: global_skills_dir must be set unless supports_generic_global is true", name)
		}
	} else if !strings.HasPrefix(e.GlobalSkillsDir, "~/") && !strings.HasPrefix(e.GlobalSkillsDir, `~\`) {
		return fmt.Errorf("agent %q: global_skills_dir must start with '~/', got %q", name, e.GlobalSkillsDir)
	}
	if e.GlobalMCPConfigFile != "" &&
		!strings.HasPrefix(e.GlobalMCPConfigFile, "~/") &&
		!strings.HasPrefix(e.GlobalMCPConfigFile, `~\`) {
		return fmt.Errorf("agent %q: global_mcp_config_file must be empty or start with '~/', got %q", name, e.GlobalMCPConfigFile)
	}
	if e.ProjectMCPConfigFile != "" &&
		!strings.HasPrefix(e.ProjectMCPConfigFile, "~/") &&
		!strings.HasPrefix(e.ProjectMCPConfigFile, `~\\`) {
		return fmt.Errorf("agent %q: project_mcp_config_file must be empty or start with '~/', got %q", name, e.ProjectMCPConfigFile)
	}
	for _, d := range e.ProjectSkillsSearch {
		if isAbsPath(d) {
			return fmt.Errorf("agent %q: project_skills_search entry must be relative, got %q", name, d)
		}
		if containsDotDot(d) {
			return fmt.Errorf("agent %q: project_skills_search entry must not contain '..', got %q", name, d)
		}
	}
	for _, d := range e.GlobalSkillsSearch {
		if !strings.HasPrefix(d, "~/") && !strings.HasPrefix(d, `~\`) {
			return fmt.Errorf("agent %q: global_skills_search entry must start with '~/', got %q", name, d)
		}
	}
	for _, d := range e.PmSkillsSearch {
		if !strings.HasPrefix(d, "~/") && !strings.HasPrefix(d, `~\`) {
			return fmt.Errorf("agent %q: pm_skills_search entry must start with '~/', got %q", name, d)
		}
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

// isAbsPath reports whether p is absolute in a cross-platform sense.
// filepath.IsAbs alone is not sufficient on Windows: it returns false for
// Unix-style paths like "/foo/bar" (no drive letter), which must also be
// rejected as non-relative.
func isAbsPath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`)
}

// userAgentsPath returns the path to the optional user agents config file.
// Platform-specific XDG/darwin logic is centralised in platform.UserConfigDir.
func userAgentsPath() (string, bool) {
	slog.Debug("resolving user agents path")
	dir, err := platform.UserConfigDir()
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
//
// Agents that support the generic convention for the requested scope
// delegate to the "generic" built-in, so sync lands skills in the
// shared .agents/skills tree instead of an agent-specific fork.
func SkillDir(name string, global bool, home string) (string, bool) {
	info, ok := registry[name]
	if !ok {
		return "", false
	}
	if global {
		if info.SupportsGenericGlobal {
			gen, ok := registry["generic"]
			if !ok {
				return "", false
			}
			return ExpandHome(gen.GlobalSkillsDir, home), true
		}
		return ExpandHome(info.GlobalSkillsDir, home), true
	}
	if info.SupportsGenericProject {
		gen, ok := registry["generic"]
		if !ok {
			return "", false
		}
		return gen.ProjectSkillsDir, true
	}
	return info.ProjectSkillsDir, true
}

// GlobalMCPConfigPath returns the absolute path to the agent's user-global MCP
// configuration file (home expanded). Returns ("", false) when not supported.
func GlobalMCPConfigPath(name, home string) (string, bool) {
	slog.Debug("resolving global mcp config path", "agent", name)
	info, ok := registry[name]
	if !ok || info.GlobalMCPConfigFile == "" {
		return "", false
	}
	return ExpandHome(info.GlobalMCPConfigFile, home), true
}

// ProjectMCPConfigPath returns the absolute path to the agent's workspace-scoped
// MCP configuration file (home expanded). Returns ("", false) when not supported.
func ProjectMCPConfigPath(name, home string) (string, bool) {
	slog.Debug("resolving project mcp config path", "agent", name)
	info, ok := registry[name]
	if !ok || info.ProjectMCPConfigFile == "" {
		return "", false
	}
	return ExpandHome(info.ProjectMCPConfigFile, home), true
}

// ExpandedProjectSkillsSearch returns the list of project-relative dirs to scan
// for skills during audit. Falls back to ProjectSkillsDir when the list is empty.
func ExpandedProjectSkillsSearch(name string) []string {
	slog.Debug("resolving project skills search dirs", "agent", name)
	info, ok := registry[name]
	if !ok {
		return nil
	}
	if len(info.ProjectSkillsSearch) > 0 {
		return info.ProjectSkillsSearch
	}
	if info.ProjectSkillsDir != "" {
		return []string{info.ProjectSkillsDir}
	}
	return nil
}

// ExpandedGlobalSkillsSearch returns the list of absolute home-expanded dirs to
// scan for skills during audit. Falls back to GlobalSkillsDir when the list is empty.
func ExpandedGlobalSkillsSearch(name, home string) []string {
	slog.Debug("resolving global skills search dirs", "agent", name)
	info, ok := registry[name]
	if !ok {
		return nil
	}
	src := info.GlobalSkillsSearch
	if len(src) == 0 && info.GlobalSkillsDir != "" {
		src = []string{info.GlobalSkillsDir}
	}
	out := make([]string, 0, len(src))
	for _, d := range src {
		out = append(out, ExpandHome(d, home))
	}
	return out
}

// ExpandedPmSkillsSearch returns the list of absolute home-expanded package-manager
// dirs to scan recursively for skills during audit.
func ExpandedPmSkillsSearch(name, home string) []string {
	slog.Debug("resolving pm skills search dirs", "agent", name)
	info, ok := registry[name]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(info.PmSkillsSearch))
	for _, d := range info.PmSkillsSearch {
		out = append(out, ExpandHome(d, home))
	}
	return out
}

// ExpandHome expands a leading ~/ or ~\ to the provided home directory.
func ExpandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}
