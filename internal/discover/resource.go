package discover

// ResourceType identifies the kind of managed resource.
type ResourceType string

const (
	ResourceSkill ResourceType = "skill"
	ResourceRepo  ResourceType = "repo"
	ResourceMCP   ResourceType = "mcp"
)

// Scope describes where the resource lives relative to the user's environment.
type Scope string

const (
	ScopeGlobal    Scope = "global"    // home-relative, shared across all projects
	ScopeWorkspace Scope = "workspace" // project-relative
)

// DriftState describes the divergence between the installed resource and its
// last-recorded snapshot.
type DriftState string

const (
	DriftOK        DriftState = "ok"        // matches snapshot exactly
	DriftModified  DriftState = "modified"  // content changed since last sync
	DriftMissing   DriftState = "missing"   // expected by snapshot but absent on disk
	DriftUnmanaged DriftState = "unmanaged" // present on disk, not in any snapshot
	DriftUnknown   DriftState = "unknown"   // no snapshot available
)

// Resource is a generic, config-independent representation of a discovered
// resource (skill directory, VCS repository, or MCP config file) found on
// the local filesystem.
type Resource struct {
	Type    ResourceType
	Scope   Scope
	Path    string // absolute path on disk
	Name    string // human-readable identifier
	Drift   DriftState
	VCSType string            // non-empty for VCS-backed resources ("git", "hg", …)
	Managed bool              // true when reconciled with the current gaal config
	Meta    map[string]string // free-form metadata: agent, url, desc, …
}
