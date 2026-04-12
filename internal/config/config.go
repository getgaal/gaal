package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"

	"gaal/internal/config/schema"
)

// ── Structures ────────────────────────────────────────────────────────────────

// Config is the top-level gaal configuration structure.
// It maps 1:1 with a single YAML file on disk; merging is handled by
// LoadChain which returns a ResolvedConfig.
type Config struct {
	Version      *int                  `yaml:"version,omitempty" json:"version,omitempty" jsonschema:"description=gaal Lite config schema version. Currently must be 1."`
	Repositories map[string]ConfigRepo `yaml:"repositories" json:"repositories,omitempty" jsonschema:"description=Map of workspace-relative paths to repository entries" validate:"dive"`
	Skills       []ConfigSkill         `yaml:"skills"       json:"skills,omitempty"       jsonschema:"description=Skill sources to install into agent skill directories"   validate:"dive"`
	MCPs         []ConfigMcp           `yaml:"mcps"         json:"mcps,omitempty"         jsonschema:"description=MCP server configuration entries to merge"             validate:"dive"`
	Telemetry    *bool                 `yaml:"telemetry,omitempty" json:"telemetry,omitempty" jsonschema:"description=Opt-in anonymous usage telemetry (true/false)"`
}

// ConfigRepo is a vcstool-compatible repository entry.
type ConfigRepo struct {
	Type    string `yaml:"type"    json:"type"             jsonschema:"description=VCS backend type,enum=git,enum=hg,enum=svn,enum=bzr,enum=tar,enum=zip" validate:"required,oneof=git hg svn bzr tar zip"`
	URL     string `yaml:"url"     json:"url"              jsonschema:"description=Repository URL or local path to clone/checkout"                             validate:"required"`
	Version string `yaml:"version" json:"version,omitempty" jsonschema:"description=Branch, tag, or commit hash; leave empty to use the default branch"`
}

// ConfigSkill defines a skill source to install.
type ConfigSkill struct {
	Source string   `yaml:"source"           json:"source"           jsonschema:"description=Skill source: GitHub shorthand (owner/repo), HTTPS URL, or local path" validate:"required"`
	Agents []string `yaml:"agents,omitempty" json:"agents,omitempty" jsonschema:"description=Target agent identifiers; use [\"*\"] to target all detected agents"`
	Global bool     `yaml:"global,omitempty" json:"global,omitempty" jsonschema:"description=When true the skill is installed globally under ~/.<agent>/skills/ instead of the project directory"`
	Select []string `yaml:"select,omitempty" json:"select,omitempty" jsonschema:"description=Specific skill names to include; empty list installs all skills from the source"`
}

// ConfigMcp defines an MCP server configuration entry.
type ConfigMcp struct {
	Name   string         `yaml:"name"             json:"name"              jsonschema:"description=Unique name identifying this MCP server entry"                                       validate:"required"`
	Source string         `yaml:"source,omitempty" json:"source,omitempty"  jsonschema:"description=URL to download a remote JSON server config (mutually exclusive with inline)"          validate:"required_without=Inline"`
	Target string         `yaml:"target"           json:"target"            jsonschema:"description=Absolute or ~-relative path to the JSON file to write or merge into"                   validate:"required"`
	Merge  *bool          `yaml:"merge,omitempty"  json:"merge,omitempty"   jsonschema:"description=Merge server entry into existing file rather than overwriting it (default: true when omitted)"`
	Inline *ConfigMcpItem `yaml:"inline,omitempty" json:"inline,omitempty"  jsonschema:"description=Inline server definition (mutually exclusive with source)"                          validate:"omitempty"`
}

// ConfigMcpItem is an inline MCP server specification.
type ConfigMcpItem struct {
	Command string            `yaml:"command"        json:"command"         jsonschema:"description=Executable to launch the MCP server process" validate:"required"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"  jsonschema:"description=Command-line arguments passed to the command"`
	Env     map[string]string `yaml:"env,omitempty"  json:"env,omitempty"   jsonschema:"description=Additional environment variables injected into the server process"`
}

// LevelConfigs holds each configuration level as loaded from disk, before
// merging. A nil pointer means the corresponding file was absent.
type LevelConfigs struct {
	Global    *Config
	User      *Config
	Workspace *Config
}

// ResolvedConfig is the result of LoadChain: the Config field carries the
// fully merged configuration (the source of truth at runtime) and Levels
// exposes each individual level as loaded from disk, before merging.
// ResolvedConfig embeds *Config so all field accesses (Repositories, Skills,
// MCPs, Telemetry…) work directly without extra dereferencing.
type ResolvedConfig struct {
	*Config
	Levels LevelConfigs
}

// ── Loading ───────────────────────────────────────────────────────────────────

// Merge rules (LoadChain):
//   - version / telemetry: the highest-priority level that explicitly sets the
//     field wins (nil = not set).
//   - repositories: map merge — higher-priority entry wins on key conflict.
//   - skills: upsert by Source — higher-priority level replaces any existing
//     entry with the same Source.
//   - mcps: upsert by Name — higher-priority level replaces any existing entry
//     with the same Name.

// Load reads and validates a single gaal configuration file.
// Duplicate skill sources and MCP names within the file are silently
// deduplicated, keeping the first occurrence.
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

	if err := cfg.validateVersion(path); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.expandPaths(filepath.Dir(path))
	cfg.deduplicate()
	return &cfg, nil
}

// LoadChain loads and merges all configuration levels in priority order:
// global -> user -> workspace. Missing files are silently skipped.
// The workspace path is the value of the --config flag (default: gaal.yaml).
// It returns a ResolvedConfig whose embedded Config is the runtime source of
// truth and whose Levels field exposes each raw per-level config.
func LoadChain(workspacePath string) (*ResolvedConfig, error) {
	slog.Debug("loading config chain", "workspace", workspacePath)

	var levels LevelConfigs
	type candidate struct {
		path  string
		store **Config
	}
	candidates := []candidate{
		{GlobalConfigFilePath(), &levels.Global},
		{userConfigFilePath(), &levels.User},
		{workspacePath, &levels.Workspace},
	}

	merged := &Config{}
	loaded := 0

	for _, c := range candidates {
		cfg, err := Load(c.path)
		if errors.Is(err, os.ErrNotExist) {
			slog.Debug("config file not found, skipping", "path", c.path)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("loading config %q: %w", c.path, err)
		}
		slog.Debug("config file loaded", "path", c.path)
		*c.store = cfg
		merged.mergeFrom(cfg)
		loaded++
	}

	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.path
	}

	if loaded == 0 {
		return nil, fmt.Errorf("no configuration file found (tried: %v)", paths)
	}

	if merged.Version == nil {
		slog.Warn("config is missing 'version: 1'; this will be required in a future release")
	}

	return &ResolvedConfig{Config: merged, Levels: levels}, nil
}

// GenerateSchema returns the JSON Schema (draft-07) for the Config type.
// The active schema.Generator (swappable via schema.Set) is used.
func GenerateSchema() ([]byte, error) {
	return schema.Generate(&Config{})
}

// ── Config methods ────────────────────────────────────────────────────────────

// validateVersion checks the schema version field. Missing is tolerated for
// backward compatibility (the caller is responsible for warning once after
// merging); any value other than 1 is a hard error.
func (c *Config) validateVersion(path string) error {
	if c.Version == nil {
		return nil
	}
	v := *c.Version
	if v <= 0 {
		return fmt.Errorf("version must be a positive integer (got %d in %s)", v, path)
	}
	if v != 1 {
		return fmt.Errorf(
			"%s declares version %d, but this build of gaal lite only understands version 1.\nUpgrade gaal lite, or check https://getgaal.com/schema for migration notes.",
			path, v,
		)
	}
	return nil
}

// JSONSchemaExtend customises the generated JSON Schema for Config.
// The schema is intentionally stricter than the runtime parser: version is
// required (not optional-with-default) and constrained to exactly 1 so IDE
// users get instant feedback.
func (Config) JSONSchemaExtend(schema *jsonschema.Schema) {
	if prop, ok := schema.Properties.Get("version"); ok {
		prop.Enum = []any{1}
	}
	schema.Required = append(schema.Required, "version")
}

// mergeFrom merges src into c. src represents a higher-priority config level.
// Rules:
//   - version / telemetry: src wins when explicitly set (non-nil).
//   - repositories: map merge — src wins on key conflict.
//   - skills: upsert by Source — src entry replaces any existing entry with
//     the same Source.
//   - mcps: upsert by Name — src entry replaces any existing entry with the
//     same Name.
func (c *Config) mergeFrom(src *Config) {
	slog.Debug("merging config", "repos", len(src.Repositories), "skills", len(src.Skills), "mcps", len(src.MCPs))

	if src.Version != nil {
		c.Version = src.Version
	}

	if src.Telemetry != nil {
		c.Telemetry = src.Telemetry
	}

	if len(src.Repositories) > 0 {
		if c.Repositories == nil {
			c.Repositories = make(map[string]ConfigRepo, len(src.Repositories))
		}
		for k, v := range src.Repositories {
			c.Repositories[k] = v
		}
	}

	for _, sk := range src.Skills {
		if i := indexOf(c.Skills, func(s ConfigSkill) bool { return s.Source == sk.Source }); i >= 0 {
			c.Skills[i] = sk // higher-priority src wins
		} else {
			c.Skills = append(c.Skills, sk)
		}
	}

	for _, mc := range src.MCPs {
		if i := indexOf(c.MCPs, func(m ConfigMcp) bool { return m.Name == mc.Name }); i >= 0 {
			c.MCPs[i] = mc // higher-priority src wins
		} else {
			c.MCPs = append(c.MCPs, mc)
		}
	}
}

// deduplicate removes duplicate entries within this Config, keeping the first
// occurrence. Skills are keyed by Source; MCPs are keyed by Name.
func (c *Config) deduplicate() {
	c.Skills = deduplicate(c.Skills, func(s ConfigSkill) string { return s.Source })
	c.MCPs = deduplicate(c.MCPs, func(m ConfigMcp) string { return m.Name })
}

func (c *Config) validate() error {
	slog.Debug("validating config", "repos", len(c.Repositories), "skills", len(c.Skills), "mcps", len(c.MCPs))
	return schema.Validate(c)
}

// ── ConfigSkill methods ───────────────────────────────────────────────────────

// UnmarshalYAML accepts the agents field in several hand-written shapes:
//   - scalar:       agents: "*"
//   - flat list:    agents: ["*", "claude"]
//   - nested list:  agents: [["*"]] — flattened one level
//
// The nested form is a common mistake when users mentally copy the canonical
// agents: ["*"] under a list bullet. We normalise all accepted shapes to
// []string so downstream code does not need to care.
func (s *ConfigSkill) UnmarshalYAML(node *yaml.Node) error {
	type rawSkill struct {
		Source string    `yaml:"source"`
		Agents yaml.Node `yaml:"agents,omitempty"`
		Global bool      `yaml:"global,omitempty"`
		Select []string  `yaml:"select,omitempty"`
	}
	var raw rawSkill
	if err := node.Decode(&raw); err != nil {
		return err
	}

	s.Source = raw.Source
	s.Global = raw.Global
	s.Select = raw.Select

	agents, err := decodeAgents(&raw.Agents)
	if err != nil {
		return fmt.Errorf("skill %q: agents: %w", raw.Source, err)
	}
	s.Agents = agents
	return nil
}

// decodeAgents normalises the agents node into []string. See
// ConfigSkill.UnmarshalYAML for accepted shapes.
func decodeAgents(n *yaml.Node) ([]string, error) {
	if n == nil || n.Kind == 0 {
		return nil, nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, item := range n.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, item.Value)
			case yaml.SequenceNode:
				for _, inner := range item.Content {
					if inner.Kind != yaml.ScalarNode {
						return nil, fmt.Errorf("line %d: nesting deeper than one level is not supported", inner.Line)
					}
					out = append(out, inner.Value)
				}
			default:
				return nil, fmt.Errorf("line %d: expected a string or list of strings", item.Line)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("line %d: expected a string or list of strings", n.Line)
	}
}

// ── ConfigMcp methods ─────────────────────────────────────────────────────────

// MergeEnabled reports whether this MCP entry should be merged (upserted) into
// the target file, as opposed to overwriting it. Defaults to true when Merge is nil.
func (mc ConfigMcp) MergeEnabled() bool {
	if mc.Merge == nil {
		return true
	}
	return *mc.Merge
}
