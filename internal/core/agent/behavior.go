package agent

import (
	"fmt"
	"log/slog"
	"slices"
)

// Scope identifies the kind of resource a [Behavior.Validate] call is
// about. The skill manager passes ScopeSkill{Global,Project}; the MCP
// manager passes ScopeMCP{Global,Project}. Validation only emits the
// warnings that apply to the requested scope.
type Scope string

const (
	ScopeSkillGlobal  Scope = "skill-global"
	ScopeSkillProject Scope = "skill-project"
	ScopeMCPGlobal    Scope = "mcp-global"
	ScopeMCPProject   Scope = "mcp-project"
)

// WarningCode identifies the kind of behavioural mismatch reported by
// [Behavior.Validate]. Callers should dedupe by (Code, Agent) so the same
// fact is not repeated once per matching scope or once per wildcard
// expansion.
type WarningCode string

const (
	// WarnSkillsUnsupported — the agent has no on-disk SKILL.md
	// mechanism; any sync into a skills directory is a no-op
	// (e.g. claude-desktop).
	WarnSkillsUnsupported WarningCode = "skills_unsupported"
	// WarnMCPGlobalUnsupported — the agent has no user-global MCP
	// config file; an entry with global: true silently lands nowhere.
	WarnMCPGlobalUnsupported WarningCode = "mcp_global_unsupported"
	// WarnMCPProjectUnsupported — the agent has no workspace-scoped
	// MCP config file; an entry with global: false silently lands
	// nowhere.
	WarnMCPProjectUnsupported WarningCode = "mcp_project_unsupported"
	// WarnUnsupportedPlatform — the agent is not officially supported
	// on the current OS (e.g. claude-desktop on linux).
	WarnUnsupportedPlatform WarningCode = "unsupported_platform"
)

// Warning is the data shape emitted by [Behavior.Validate]. Msg is the
// user-facing sentence; Hint suggests a remediation.
type Warning struct {
	Code  WarningCode
	Agent string
	Scope Scope
	Msg   string
	Hint  string
}

// Behavior describes what a registered agent can and cannot do. It is
// derived from the agent's [Info] entry plus the explicit behaviour keys
// in agents.yaml.
//
// The struct (rather than interface) shape is deliberate: the registry is
// YAML-driven and users can add custom agents in
// ~/.config/gaal/agents.yaml. An interface-per-agent design would force
// custom agents to ship Go code, breaking that contract.
type Behavior struct {
	Name               string
	SupportsSkills     bool
	SupportsMCPGlobal  bool
	SupportsMCPProject bool
	// SupportedPlatforms restricts the agent to one or more values of
	// runtime.GOOS (darwin, linux, windows). Empty = no restriction.
	SupportedPlatforms []string
}

// Validate reports the behavioural mismatches that apply when an
// operation of the given Scope targets this agent on the given OS.
//
// goos is taken as a parameter (rather than reading runtime.GOOS
// internally) so callers can test platform-restricted branches without
// OS-specific build tags.
//
// An empty slice means "no issue".
func (b Behavior) Validate(scope Scope, goos string) []Warning {
	slog.Debug("validating agent behaviour", "agent", b.Name, "scope", scope, "goos", goos)
	var out []Warning
	if !b.platformSupported(goos) {
		out = append(out, Warning{
			Code:  WarnUnsupportedPlatform,
			Agent: b.Name,
			Scope: scope,
			Msg:   fmt.Sprintf("agent %q is not officially supported on %s", b.Name, goos),
			Hint:  "remove this agent from agents:, or list agents explicitly without it",
		})
	}
	switch scope {
	case ScopeSkillGlobal, ScopeSkillProject:
		if !b.SupportsSkills {
			out = append(out, Warning{
				Code:  WarnSkillsUnsupported,
				Agent: b.Name,
				Scope: scope,
				Msg:   fmt.Sprintf("agent %q has no on-disk SKILL.md mechanism — skill installs are a no-op", b.Name),
				Hint:  "remove this agent from agents:, or list agents explicitly without it",
			})
		}
	case ScopeMCPGlobal:
		if !b.SupportsMCPGlobal {
			out = append(out, Warning{
				Code:  WarnMCPGlobalUnsupported,
				Agent: b.Name,
				Scope: scope,
				Msg:   fmt.Sprintf("agent %q has no user-global MCP config", b.Name),
				Hint:  "set global: false (project scope) or remove this agent from agents:",
			})
		}
	case ScopeMCPProject:
		if !b.SupportsMCPProject {
			out = append(out, Warning{
				Code:  WarnMCPProjectUnsupported,
				Agent: b.Name,
				Scope: scope,
				Msg:   fmt.Sprintf("agent %q has no workspace-scoped MCP config", b.Name),
				Hint:  "set global: true (user scope) or remove this agent from agents:",
			})
		}
	}
	return out
}

func (b Behavior) platformSupported(goos string) bool {
	if len(b.SupportedPlatforms) == 0 {
		return true
	}
	return slices.Contains(b.SupportedPlatforms, goos)
}

// BehaviorFor returns the [Behavior] for the named agent. Returns false
// when the agent is not registered.
func BehaviorFor(name string) (Behavior, bool) {
	slog.Debug("resolving agent behaviour", "agent", name)
	info, ok := registry[name]
	if !ok {
		return Behavior{}, false
	}
	return behaviorFromInfo(name, info), true
}

// behaviorFromInfo derives a [Behavior] from an [Info] entry.
//
// MCP support is structural: an agent supports MCP for a given scope
// iff the corresponding *MCPConfigFile field on Info is non-empty. Skill
// support and platform restrictions are explicit opt-outs declared in
// agents.yaml — defaults are "skills supported" and "all platforms".
func behaviorFromInfo(name string, info Info) Behavior {
	return Behavior{
		Name:               name,
		SupportsSkills:     info.SupportsSkills,
		SupportsMCPGlobal:  info.GlobalMCPConfigFile != "",
		SupportsMCPProject: info.ProjectMCPConfigFile != "",
		SupportedPlatforms: info.SupportedPlatforms,
	}
}
