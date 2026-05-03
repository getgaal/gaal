# `internal/core/agent`

> Read-only registry mapping coding-agent identifiers to their on-disk
> layout (skill dirs, MCP config paths). Embedded YAML is the source of
> truth; user can extend via `~/.config/gaal/agents.yaml`.

> **Pillar reference:** the full agent registry pillar — `Info`
> descriptor, built-in registry, user extension, generic-redirect
> convention, security constraints — lives in [`docs/core.md — Agent
> Registry Sub-package`](../core.md#agent-registry-sub-package-internalcoreagent).
> This page is the package-level summary.

## Public API

| Symbol | Description |
|--------|-------------|
| `Names() []string` | All registered agent identifiers (unsorted) |
| `List() []Entry` | All entries sorted by name |
| `Lookup(name string) (Info, bool)` | Single-agent lookup |
| `SkillDir(name, global, home string) (string, bool)` | Resolved skill install path; applies generic-convention redirect |
| `ProjectMCPConfigPath(name, home string) (string, bool)` | Project-scope MCP config path (home-expanded) |
| `GlobalMCPConfigPath(name, home string) (string, bool)` | Global-scope MCP config path |
| `ExpandedProjectSkillsSearch(name string) []string` | Project-relative scan dirs |
| `ExpandedGlobalSkillsSearch(name, home string) []string` | Home-expanded global scan dirs |
| `ExpandedPmSkillsSearch(name, home string) []string` | Home-expanded package-manager scan dirs |
| `ExpandHome(p, home string) string` | `~/` expansion |

## Initialisation

```mermaid
flowchart TD
    A[package init] --> B[//go:embed agents.yaml]
    B --> C[loadInto builtins, allowOverride=false]
    C --> D[userAgentsPath]
    D --> E{file exists?}
    E -- no --> Z([registry ready — built-ins only])
    E -- yes --> F[os.ReadFile]
    F --> G[loadInto user, allowOverride=true]
    G --> G1{validateEntry per agent}
    G1 -- ok --> H[merged registry ready]
    G1 -- err --> SK[slog.Warn, skip entry]
```

## Security constraints (`validateEntry`)

| Field | Constraint |
|-------|-----------|
| `project_skills_dir` | Relative; no `..` segments — or empty when `supports_generic_project: true` |
| `global_skills_dir` | Must start with `~/` or `~\` — or empty when `supports_generic_global: true` |
| `project_mcp_config_file`, `global_mcp_config_file` | Empty or `~/`-prefixed |
| `*_search[]` | Same rules as their non-`_search` counterparts |

`isAbsPath` rejects POSIX-style absolutes even on Windows (where
`filepath.IsAbs` would miss them). `containsDotDot` splits on `/`
(after `filepath.ToSlash`) and checks each segment.

## Generic redirect

Agents with `supports_generic_project: true` or `supports_generic_global:
true` install into the shared `generic` agent's tree:

- Project: `.agents/skills`
- Global: `~/.agents/skills`

The redirect is applied transparently inside `SkillDir()` — callers
must never re-implement it.

## Recent fixes worth knowing

| PR | Issue | Effect |
|----|-------|--------|
| #194 | #128 | `claude-desktop` install detection via macOS app path / Windows `%APPDATA%` |
| #189 | #127 | `AgentEntry` carries `GlobalMCPConfigFile` so summary rollups can match per-agent MCP targets |
| #187 | #130 | Replaced string concat with `filepath.Join` in `ops/agents.go` |

## Related

- [`docs/core.md`](../core.md) — full pillar description
- [`packages/skill.md`](skill.md), [`packages/mcp.md`](mcp.md) — main consumers
