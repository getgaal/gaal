//go:build e2e

package e2e

import (
	"fmt"
	"strings"
)

// configBuilder assembles small gaal.yaml documents in a readable, test-
// local way. The fields cover the subset of the schema the e2e suite
// exercises — repositories, skills, mcps. Callers add entries via the
// chainable Add* methods and finalise with String().
type configBuilder struct {
	repos  []repoBlock
	skills []skillBlock
	mcps   []mcpBlock
}

type repoBlock struct {
	path    string // workspace-relative checkout path (the YAML map key)
	typ     string // "git", "hg", "tar", etc.
	url     string // clone source — local path or URL
	version string // optional tag / branch / commit
}

type skillBlock struct {
	source  string
	agents  []string
	global  bool
	selects []string
}

type mcpBlock struct {
	name    string
	agents  []string
	global  bool
	command string
	args    []string
	env     map[string]string
}

func newConfig() *configBuilder { return &configBuilder{} }

// AddRepository registers a repository entry. path is the workspace-relative
// checkout location (the YAML map key); typ is "git", "hg", "tar", etc.;
// url is the clone source — a local filesystem path is acceptable for
// hermetic test backends.
func (b *configBuilder) AddRepository(path, typ, url, version string) *configBuilder {
	b.repos = append(b.repos, repoBlock{path: path, typ: typ, url: url, version: version})
	return b
}

func (b *configBuilder) AddSkill(source string, agents []string, global bool, selects ...string) *configBuilder {
	b.skills = append(b.skills, skillBlock{source: source, agents: agents, global: global, selects: selects})
	return b
}

func (b *configBuilder) AddMCP(name string, agents []string, global bool, command string, args []string, env map[string]string) *configBuilder {
	b.mcps = append(b.mcps, mcpBlock{name: name, agents: agents, global: global, command: command, args: args, env: env})
	return b
}

// String renders the builder as a YAML document that gaal can consume.
// Indentation is fixed (two-space) and ordering is deterministic so test
// failure messages diff cleanly against expected golden output.
func (b *configBuilder) String() string {
	var sb strings.Builder
	sb.WriteString("schema: 1\n")
	if len(b.repos) > 0 {
		sb.WriteString("repositories:\n")
		for _, r := range b.repos {
			fmt.Fprintf(&sb, "  %s:\n", r.path)
			fmt.Fprintf(&sb, "    type: %s\n", r.typ)
			fmt.Fprintf(&sb, "    url: %s\n", quoteIfNeeded(r.url))
			if r.version != "" {
				fmt.Fprintf(&sb, "    version: %s\n", quoteIfNeeded(r.version))
			}
		}
	}
	if len(b.skills) > 0 {
		sb.WriteString("skills:\n")
		for _, s := range b.skills {
			fmt.Fprintf(&sb, "  - source: %s\n", s.source)
			fmt.Fprintf(&sb, "    agents: [%s]\n", quoteAndJoin(s.agents))
			fmt.Fprintf(&sb, "    global: %t\n", s.global)
			if len(s.selects) > 0 {
				sb.WriteString("    select:\n")
				for _, sel := range s.selects {
					fmt.Fprintf(&sb, "      - %s\n", sel)
				}
			}
		}
	}
	if len(b.mcps) > 0 {
		sb.WriteString("mcps:\n")
		for _, m := range b.mcps {
			fmt.Fprintf(&sb, "  - name: %s\n", m.name)
			fmt.Fprintf(&sb, "    agents: [%s]\n", quoteAndJoin(m.agents))
			fmt.Fprintf(&sb, "    global: %t\n", m.global)
			sb.WriteString("    inline:\n")
			fmt.Fprintf(&sb, "      command: %s\n", m.command)
			if len(m.args) > 0 {
				sb.WriteString("      args:\n")
				for _, a := range m.args {
					fmt.Fprintf(&sb, "        - %s\n", quoteIfNeeded(a))
				}
			}
			if len(m.env) > 0 {
				sb.WriteString("      env:\n")
				keys := sortedKeys(m.env)
				for _, k := range keys {
					fmt.Fprintf(&sb, "        %s: %s\n", k, quoteIfNeeded(m.env[k]))
				}
			}
		}
	}
	return sb.String()
}

func quoteAndJoin(items []string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = `"` + s + `"`
	}
	return strings.Join(out, ", ")
}

// quoteIfNeeded YAML-quotes a value when it contains characters that would
// trip the bare-scalar parser. Conservative: anything other than [a-zA-Z0-9._/-]
// gets wrapped in double quotes with embedded quotes escaped.
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '/' || r == '-':
		default:
			return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
		}
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Tiny insertion sort — keeps configs stable for diff-friendly failures.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
