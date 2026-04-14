package render

// StatusCode is the machine-readable state of a resource.
type StatusCode string

const (
	StatusOK        StatusCode = "ok"
	StatusDirty     StatusCode = "dirty"
	StatusNotCloned StatusCode = "not_cloned"
	StatusPartial   StatusCode = "partial"
	StatusPresent   StatusCode = "present"
	StatusAbsent    StatusCode = "absent"
	StatusUnmanaged StatusCode = "unmanaged"
	StatusError     StatusCode = "error"
)

// RepoEntry holds the status of a single repository.
type RepoEntry struct {
	Path    string     `json:"path"`
	Type    string     `json:"type"`
	Status  StatusCode `json:"status"`
	Dirty   bool       `json:"dirty,omitempty"`
	Current string     `json:"current,omitempty"`
	Want    string     `json:"want,omitempty"`
	URL     string     `json:"url,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// SkillEntry holds the status of a single skill configuration.
type SkillEntry struct {
	Source    string     `json:"source"`
	Agent     string     `json:"agent"`
	Global    bool       `json:"global"`
	Status    StatusCode `json:"status"`
	Installed []string   `json:"installed"`
	Missing   []string   `json:"missing"`
	Modified  []string   `json:"modified,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// AgentEntry holds the registry information for a supported agent.
type AgentEntry struct {
	Name                    string `json:"name"`
	Installed               bool   `json:"installed"`
	Source                  string `json:"source"`
	ProjectSkillsDir        string `json:"project_skills_dir"`
	GlobalSkillsDir         string `json:"global_skills_dir"`
	ProjectMCPConfigFile    string `json:"project_mcp_config_file,omitempty"`
	ProjectSkillsViaGeneric bool   `json:"project_skills_via_generic,omitempty"`
	GlobalSkillsViaGeneric  bool   `json:"global_skills_via_generic,omitempty"`
}

// AgentPath describes a single search path for an agent's skills or config.
type AgentPath struct {
	Label      string `json:"label"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	SkillCount int    `json:"skill_count"`
}

// AgentDetail holds the full detail view for a single agent.
type AgentDetail struct {
	Name       string      `json:"name"`
	Installed  bool        `json:"installed"`
	Source     string      `json:"source"`
	Paths      []AgentPath `json:"paths"`
	MCPSupport bool        `json:"mcp_support"`
	MCPConfig  string      `json:"mcp_config,omitempty"`
	MCPExists  bool        `json:"mcp_exists,omitempty"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// MCPEntry holds the status of a single MCP server entry.
type MCPEntry struct {
	Name   string     `json:"name"`
	Status StatusCode `json:"status"`
	Dirty  bool       `json:"dirty,omitempty"`
	Target string     `json:"target"`
	Error  string     `json:"error,omitempty"`
}

// StatusReport aggregates the status of all managed resources.
type StatusReport struct {
	Repositories []RepoEntry  `json:"repositories"`
	Skills       []SkillEntry `json:"skills"`
	MCPs         []MCPEntry   `json:"mcps"`
	Agents       []AgentEntry `json:"agents"`
}

// PlanAction describes what sync would do for a given resource.
type PlanAction string

const (
	PlanNoOp   PlanAction = "no_change"
	PlanClone  PlanAction = "clone"
	PlanUpdate PlanAction = "update"
	PlanCreate PlanAction = "create"
	PlanError  PlanAction = "error"
)

// PlanRepoEntry describes the planned action for a single repository.
type PlanRepoEntry struct {
	Path    string     `json:"path"`
	Type    string     `json:"type"`
	Action  PlanAction `json:"action"`
	URL     string     `json:"url,omitempty"`
	Current string     `json:"current,omitempty"`
	Want    string     `json:"want,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// PlanSkillEntry describes the planned action for a single skill config.
type PlanSkillEntry struct {
	Source  string     `json:"source"`
	Agent   string     `json:"agent"`
	Action  PlanAction `json:"action"`
	Install []string   `json:"install,omitempty"`
	Update  []string   `json:"update,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// PlanMCPEntry describes the planned action for a single MCP entry.
type PlanMCPEntry struct {
	Name   string     `json:"name"`
	Target string     `json:"target"`
	Action PlanAction `json:"action"`
	Error  string     `json:"error,omitempty"`
}

// PlanReport aggregates the planned actions for all managed resources.
type PlanReport struct {
	Repositories []PlanRepoEntry  `json:"repositories"`
	Skills       []PlanSkillEntry `json:"skills"`
	MCPs         []PlanMCPEntry   `json:"mcps"`
	HasChanges   bool             `json:"has_changes"`
	HasErrors    bool             `json:"has_errors"`
}

// AuditSkillEntry holds the metadata of a single skill discovered during audit.
type AuditSkillEntry struct {
	Name string `json:"name"`
	Desc string `json:"desc,omitempty"`
	// Agent is the name of the agent that owns this skill directory.
	Agent string `json:"agent"`
	// Source is one of "project", "global", or "package-manager".
	Source string `json:"source"`
	Path   string `json:"path"`
}

// AuditMCPEntry holds the MCP servers found for a single agent.
type AuditMCPEntry struct {
	Agent      string   `json:"agent"`
	ConfigFile string   `json:"config_file"`
	Servers    []string `json:"servers"`
}

// AuditReport aggregates all skills and MCP servers discovered on the machine.
type AuditReport struct {
	Home   string            `json:"-"`
	Skills []AuditSkillEntry `json:"skills"`
	MCPs   []AuditMCPEntry   `json:"mcps"`
}
