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
)

// Config is the top-level gaal configuration.
type Config struct {
	Repositories map[string]RepoConfig `yaml:"repositories"`
	Skills       []SkillConfig         `yaml:"skills"`
	MCPs         []MCPConfig           `yaml:"mcps"`
}

// RepoConfig is a vcstool-compatible repository entry.
type RepoConfig struct {
	Type    string `yaml:"type"` // git, hg, svn, bzr, tar, zip
	URL     string `yaml:"url"`
	Version string `yaml:"version"` // branch, tag, commit; empty = default branch
}

// SkillConfig defines a skill source to install.
type SkillConfig struct {
	Source string   `yaml:"source"`           // owner/repo, URL, or local path
	Agents []string `yaml:"agents,omitempty"` // specific agents; ["*"] = all detected
	Global bool     `yaml:"global,omitempty"` // true = install to ~/.<agent>/skills/
	Select []string `yaml:"select,omitempty"` // specific skill names; empty = all
}

// MCPConfig defines an MCP server configuration entry.
type MCPConfig struct {
	Name   string           `yaml:"name"`
	Source string           `yaml:"source,omitempty"` // URL to download JSON config
	Target string           `yaml:"target"`           // JSON file to merge into
	Merge  bool             `yaml:"merge,omitempty"`  // merge into existing (default true)
	Inline *MCPInlineConfig `yaml:"inline,omitempty"` // inline server definition
}

// MCPInlineConfig is an inline MCP server specification.
type MCPInlineConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// Configuration file locations by priority (lowest → highest):
//
//  1. Global   — system-wide, set by a package manager
//                  Linux/macOS : /etc/gaal/config.yaml
//                  Windows     : %PROGRAMDATA%\gaal\config.yaml
//  2. User     — per-user customisation
//                  Linux       : $XDG_CONFIG_HOME/gaal/config.yaml  (~/.config/gaal/config.yaml)
//                  macOS       : ~/Library/Application Support/gaal/config.yaml
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

// userConfigFilePath returns the per-user config path for the current OS.
// It delegates to os.UserConfigDir() which respects XDG_CONFIG_HOME on Linux,
// ~/Library/Application Support on macOS, and %AppData% on Windows.
func userConfigFilePath() string {
	dir, err := os.UserConfigDir()
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
	for path, repo := range c.Repositories {
		if repo.Type == "" {
			return fmt.Errorf("repository %q: type is required", path)
		}
		if repo.URL == "" {
			return fmt.Errorf("repository %q: url is required", path)
		}
	}

	for i, s := range c.Skills {
		if s.Source == "" {
			return fmt.Errorf("skill[%d]: source is required", i)
		}
	}

	for i, m := range c.MCPs {
		if m.Name == "" {
			return fmt.Errorf("mcp[%d]: name is required", i)
		}
		if m.Target == "" {
			return fmt.Errorf("mcp %q: target is required", m.Name)
		}
		if m.Source == "" && m.Inline == nil {
			return fmt.Errorf("mcp %q: source or inline is required", m.Name)
		}
	}

	return nil
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
		if filepath.IsAbs(p) {
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
			strings.HasPrefix(s, "./") || strings.HasPrefix(s, `./`) ||
			strings.HasPrefix(s, "../") || strings.HasPrefix(s, `.\`) ||
			strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) ||
			filepath.IsAbs(s) {
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
