package ops

import (
	"log/slog"
	"sort"

	"gaal/internal/config"
)

// Scope is the gaal.yaml destination scope selected by the user during init.
// It drives the output file path, the filter applied to audit results, and
// the value of the Global flag on generated SkillConfig entries.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// CandidateKind distinguishes skill candidates from MCP candidates inside a
// flat list rendered by the init wizard.
type CandidateKind string

const (
	CandidateSkill CandidateKind = "skill"
	CandidateMCP   CandidateKind = "mcp"
)

// Candidate is one importable item surfaced by BuildImportCandidates. The
// wizard turns a user's selection into a Plan via BuildPlan.
type Candidate struct {
	Kind      CandidateKind
	AgentName string
	Label     string
	Detail    string

	// Skill fields (populated when Kind == CandidateSkill).
	SkillName       string
	SkillDesc       string
	SkillPath       string
	SkillSource     string
	SkillSourceKind SourceKind

	// MCP fields (populated when Kind == CandidateMCP).
	MCPName   string
	MCPTarget string
	MCPInline *config.ConfigMcpItem
}

// AgentSection groups candidates by their owning agent. The generic agent
// carries the list of delegators in GenericDelegators so the wizard can show
// an info line under its header.
type AgentSection struct {
	AgentName         string
	GenericDelegators []string
	Skills            []Candidate
	MCPs              []Candidate
}

// Candidates is the structured result of BuildImportCandidates, ready for
// rendering by the wizard.
type Candidates struct {
	Sections []AgentSection
}

// Plan is the resolved set of configuration entries to write to gaal.yaml.
type Plan struct {
	Skills []config.ConfigSkill
	MCPs   []config.ConfigMcp
}

// BuildPlan converts a slice of user-selected candidates into a Plan.
//
// Skills sharing the same (source, agent, scope) triple are grouped into a
// single SkillConfig with a sorted, deduplicated Select list. MCPs are never
// grouped. Output entries are sorted for deterministic YAML.
func BuildPlan(selected []Candidate, scope Scope) Plan {
	slog.Debug("building plan", "candidates", len(selected), "scope", scope)
	global := scope == ScopeGlobal

	type skillKey struct {
		source string
		agent  string
	}
	grouped := map[skillKey][]string{}
	keys := []skillKey{}
	var mcps []config.ConfigMcp

	for _, c := range selected {
		switch c.Kind {
		case CandidateSkill:
			k := skillKey{source: c.SkillSource, agent: c.AgentName}
			if _, ok := grouped[k]; !ok {
				keys = append(keys, k)
			}
			grouped[k] = append(grouped[k], c.SkillName)
		case CandidateMCP:
			entry := config.ConfigMcp{
				Name:   c.MCPName,
				Agents: []string{c.AgentName},
				Global: true,
			}
			if c.MCPInline != nil {
				inline := *c.MCPInline
				entry.Inline = &inline
			}
			mcps = append(mcps, entry)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].agent != keys[j].agent {
			return keys[i].agent < keys[j].agent
		}
		return keys[i].source < keys[j].source
	})

	skills := make([]config.ConfigSkill, 0, len(keys))
	for _, k := range keys {
		names := dedupSorted(grouped[k])
		skills = append(skills, config.ConfigSkill{
			Source: k.source,
			Agents: []string{k.agent},
			Global: global,
			Select: names,
		})
	}

	sort.Slice(mcps, func(i, j int) bool { return mcps[i].Name < mcps[j].Name })

	return Plan{Skills: skills, MCPs: mcps}
}

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	cp := make([]string, len(in))
	copy(cp, in)
	sort.Strings(cp)
	out := cp[:0]
	var last string
	for i, s := range cp {
		if i == 0 || s != last {
			out = append(out, s)
		}
		last = s
	}
	return out
}
