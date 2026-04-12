package config

import "fmt"

// ConfigScope identifies the scope at which a configuration level operates
// within the merge chain. Scopes are ordered from lowest to highest priority:
// ScopeGlobal < ScopeUser < ScopeWorkspace.
//
// A field annotated with gaal:"maxscope=<scope>" will only be overridden by
// config levels whose scope is ≤ the declared maximum.
type ConfigScope int

const (
	// ScopeGlobal is the system-wide configuration scope (e.g. /etc/gaal/).
	ScopeGlobal ConfigScope = iota // 0
	// ScopeUser is the per-user configuration scope (e.g. ~/.config/gaal/).
	ScopeUser // 1
	// ScopeWorkspace is the project-local configuration scope (gaal.yaml in CWD).
	ScopeWorkspace // 2
)

// String returns the canonical string representation of a ConfigScope.
func (s ConfigScope) String() string {
	switch s {
	case ScopeGlobal:
		return "global"
	case ScopeUser:
		return "user"
	case ScopeWorkspace:
		return "workspace"
	default:
		return fmt.Sprintf("ConfigScope(%d)", int(s))
	}
}

// ParseConfigScope parses a string into a ConfigScope.
// Accepted values (case-sensitive): "global", "user", "workspace".
func ParseConfigScope(s string) (ConfigScope, error) {
	switch s {
	case "global":
		return ScopeGlobal, nil
	case "user":
		return ScopeUser, nil
	case "workspace":
		return ScopeWorkspace, nil
	default:
		return 0, fmt.Errorf("unknown config scope %q: must be one of global, user, workspace", s)
	}
}
