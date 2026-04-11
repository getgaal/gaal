package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"gaal/internal/config/schema"
)

// Config is the top-level gaal configuration.
type Config struct {
	Repositories map[string]RepoConfig `yaml:"repositories" json:"repositories,omitempty" jsonschema:"description=Map of workspace-relative paths to repository entries" validate:"dive"`
	Skills       []SkillConfig         `yaml:"skills"       json:"skills,omitempty"       jsonschema:"description=Skill sources to install into agent skill directories"   validate:"dive"`
	MCPs         []MCPConfig           `yaml:"mcps"         json:"mcps,omitempty"         jsonschema:"description=MCP server configuration entries to merge"             validate:"dive"`
}

// RepoConfig is a vcstool-compatible repository entry.
type RepoConfig struct {
	Type    string `yaml:"type"    json:"type"             jsonschema:"description=VCS backend type,enum=git,enum=hg,enum=svn,enum=bzr,enum=tar,enum=zip" validate:"required,oneof=git hg svn bzr tar zip"`
	URL     string `yaml:"url"     json:"url"              jsonschema:"description=Repository URL or local path to clone/checkout"                             validate:"required"`
	Version string `yaml:"version" json:"version,omitempty" jsonschema:"description=Branch, tag, or commit hash; leave empty to use the default branch"`
}

// SkillConfig defines a skill source to install.
type SkillConfig struct {
	Source string   `yaml:"source"           json:"source"           jsonschema:"description=Skill source: GitHub shorthand (owner/repo), HTTPS URL, or local path" validate:"required"`
	Agents []string `yaml:"agents,omitempty" json:"agents,omitempty" jsonschema:"description=Target agent identifiers; use [\"*\"] to target all detected agents"`
	Global bool     `yaml:"global,omitempty" json:"global,omitempty" jsonschema:"description=When true the skill is installed globally under ~/.<agent>/skills/ instead of the project directory"`
	Select []string `yaml:"select,omitempty" json:"select,omitempty" jsonschema:"description=Specific skill names to include; empty list installs all skills from the source"`
}

// MCPConfig defines an MCP server configuration entry.
type MCPConfig struct {
	Name   string           `yaml:"name"             json:"name"              jsonschema:"description=Unique name identifying this MCP server entry"                validate:"required"`
	Source string           `yaml:"source,omitempty" json:"source,omitempty"  jsonschema:"description=URL to download a remote JSON server config (mutually exclusive with inline)" validate:"required_without=Inline"`
	Target string           `yaml:"target"           json:"target"            jsonschema:"description=Absolute or ~-relative path to the JSON file to write or merge into" validate:"required"`
	Merge  bool             `yaml:"merge,omitempty"  json:"merge,omitempty"   jsonschema:"description=Merge server entry into existing file rather than overwriting it (default true)"`
	Inline *MCPInlineConfig `yaml:"inline,omitempty" json:"inline,omitempty"  jsonschema:"description=Inline server definition (mutually exclusive with source)"                   validate:"omitempty"`
}

// MCPInlineConfig is an inline MCP server specification.
type MCPInlineConfig struct {
	Command string            `yaml:"command"        json:"command"         jsonschema:"description=Executable to launch the MCP server process" validate:"required"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"  jsonschema:"description=Command-line arguments passed to the command"`
	Env     map[string]string `yaml:"env,omitempty"  json:"env,omitempty"   jsonschema:"description=Additional environment variables injected into the server process"`
}

// Configuration file locations by priority (lowest → highest):
//
//  1. Global   — system-wide, set by a package manager
//                  Linux/macOS : /etc/gaal/config.yaml
//                  Windows     : %PROGRAMDATA%\gaal\config.yaml
//  2. User     — per-user customisation
//                  Linux       : $XDG_CONFIG_HOME/gaal/config.yaml  (~/.config/gaal/config.yaml)
//                  macOS       : $XDG_CONFIG_HOME/gaal/config.yaml  (~/.config/gaal/config.yaml)
//                  Windows     : %AppData%\gaal\config.yaml
//  3. Workspace — project-specific, value of the --config flag (default: gaal.yaml in CWD)
//
// Higher-priority files override lower-priority ones:
//   - repositories: merged by path key, workspace wins on conflict.
//   - skills / mcps: accumulated across all levels (all entries are applied).

// globalConfigFilePath returns the system-wide config path for the current OS.
func globalConfigFilePath() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "gaal", "config.yaml")
	}
	// Linux and macOS both follow the /etc convention for system-wide config.
	return "/etc/gaal/config.yaml"
}

// userConfigDir returns the directory in which gaal stores per-user config.
// On macOS we intentionally diverge from os.UserConfigDir() (which would return
// ~/Library/Application Support) and prefer XDG_CONFIG_HOME when it is set,
// otherwise ~/.config to match the conventions of other CLI tools. Linux and
// Windows fall through to os.UserConfigDir().
func userConfigDir() (string, error) {
	if runtime.GOOS == "darwin" {
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return xdg, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config"), nil
	}
	return os.UserConfigDir()
}

// UserConfigFilePath is the exported accessor for userConfigFilePath. It is
// used by callers outside this package (e.g. the init wizard) that need to
// resolve the per-user config destination before a Config is loaded.
func UserConfigFilePath() string {
	return userConfigFilePath()
}

// userConfigFilePath returns the per-user config path for the current OS.
// It respects XDG_CONFIG_HOME on Linux and macOS when set, otherwise ~/.config
// on macOS (see userConfigDir), and %AppData% on Windows.
func userConfigFilePath() string {
	slog.Debug("resolving user config file path")
	dir, err := userConfigDir()
	if err != nil {
		// Fallback: XDG default.
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gaal", "config.yaml")
}

// Load reads and validates a single gaal configuration file.
func Load(path string) (*Config, error) {
	slog.Debug("loading config file", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.expandPaths(filepath.Dir(path))
	return &cfg, nil
}

// LoadChain loads and merges all configuration levels in priority order:
// global → user → workspace. Missing files are silently skipped.
// The workspace path is the value of the --config flag (default: gaal.yaml).
func LoadChain(workspacePath string) (*Config, error) {
	candidates := []string{globalConfigFilePath(), userConfigFilePath(), workspacePath}

	merged := &Config{}
	loaded := 0

	for _, p := range candidates {
		cfg, err := Load(p)
		if errors.Is(err, os.ErrNotExist) {
			slog.Debug("config file not found, skipping", "path", p)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("loading config %q: %w", p, err)
		}
		slog.Debug("config file loaded", "path", p)
		merged.mergeFrom(cfg)
		loaded++
	}

	if loaded == 0 {
		return nil, fmt.Errorf("no configuration file found (tried: %v)", candidates)
	}

	return merged, nil
}

// mergeFrom merges src into c. Higher-priority fields (from src) win:
//   - repositories: merged by path key, src overrides c on conflict.
//   - skills / mcps: appended (all entries from all levels are kept).
func (c *Config) mergeFrom(src *Config) {
	slog.Debug("merging config", "repos", len(src.Repositories), "skills", len(src.Skills), "mcps", len(src.MCPs))
	// Repositories: map merge — src (higher priority) wins on key conflict.
	if len(src.Repositories) > 0 {
		if c.Repositories == nil {
			c.Repositories = make(map[string]RepoConfig, len(src.Repositories))
		}
		for k, v := range src.Repositories {
			c.Repositories[k] = v
		}
	}

	// Skills and MCPs: accumulate across all levels.
	c.Skills = append(c.Skills, src.Skills...)
	c.MCPs = append(c.MCPs, src.MCPs...)
}

func (c *Config) validate() error {
	slog.Debug("validating config", "repos", len(c.Repositories), "skills", len(c.Skills), "mcps", len(c.MCPs))
	return schema.Validate(c)
}

// GenerateSchema returns the JSON Schema (draft-07) for the Config type.
// The active [schema.Generator] (swappable via [schema.Set]) is used.
func GenerateSchema() ([]byte, error) {
	return schema.Generate(&Config{})
}

// expandPaths expands ~ and relative paths, while leaving remote URLs and
// GitHub shorthands (owner/repo) untouched.
func (c *Config) expandPaths(baseDir string) {
	home, _ := os.UserHomeDir()

	expandPath := func(p string) string {
		// Accept both ~/ (POSIX) and ~\ (Windows) as home-relative prefixes.
		if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			return filepath.Join(home, p[2:])
		}
		// filepath.IsAbs("/posix/path") returns false on Windows;
		// handle POSIX-style absolute paths explicitly so cross-platform
		// config files (e.g. written on Linux, used on Windows) are preserved.
		if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
			return p
		}
		return filepath.Join(baseDir, p)
	}

	isRemote := func(s string) bool {
		return strings.HasPrefix(s, "http://") ||
			strings.HasPrefix(s, "https://") ||
			strings.HasPrefix(s, "git@") ||
			strings.HasPrefix(s, "ssh://")
	}

	// GitHub shorthand: exactly one forward-slash (owner/repo), no scheme, not a local path.
	isGitHubShorthand := func(s string) bool {
		if isRemote(s) ||
			strings.HasPrefix(s, "./") || strings.HasPrefix(s, `.\`) ||
			strings.HasPrefix(s, "../") || strings.HasPrefix(s, `..\`) ||
			strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) ||
			strings.HasPrefix(s, "/") || filepath.IsAbs(s) {
			return false
		}
		parts := strings.Split(s, "/")
		return len(parts) == 2
	}

	// Expand repository paths (the keys).
	expanded := make(map[string]RepoConfig, len(c.Repositories))
	for path, repo := range c.Repositories {
		expanded[expandPath(path)] = repo
	}
	c.Repositories = expanded

	// Skill sources: only expand local paths.
	for i := range c.Skills {
		src := c.Skills[i].Source
		if !isRemote(src) && !isGitHubShorthand(src) {
			c.Skills[i].Source = expandPath(src)
		}
	}

	// MCP targets are always local paths.
	for i := range c.MCPs {
		c.MCPs[i].Target = expandPath(c.MCPs[i].Target)
	}
}
