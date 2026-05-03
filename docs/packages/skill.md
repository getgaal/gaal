# `internal/skill`

> Skill manager: resolve sources, discover `SKILL.md` files, filter by
> `select:`, install atomically into the right agent directories.

## Public API

| Symbol | Description |
|--------|-------------|
| `NewManager(skills []ConfigSkill, cacheDir, home, workDir, stateDir string, force bool) *Manager` | Construct |
| `Manager.Sync(ctx) error` | Resolve + install every skill from every source |
| `Manager.Prune(ctx) error` | Remove on-disk skills no longer declared |
| `Manager.Status(ctx) []Status` | Per-source × per-agent status (`Installed`, `Missing`, `Modified`) |
| `Manager.SourcePaths() []string` | Absolute local paths for every configured skill source (for FS-scan filtering) |

## Source lifecycle

```mermaid
flowchart TD
    A[Manager.Sync ctx] --> B[for each ConfigSkill]
    B --> C[resolveSource]
    C --> C1{local path?}
    C1 -- yes --> C1a[expand ~/, return path]
    C1 -- no --> C1b[urlToCacheKey → cacheDir/key]
    C1b --> C1c{IsCloned?}
    C1c -- no --> C1d[NewShallow git Clone depth=1]
    C1c -- yes --> C1e[NewShallow git Update — hard-reset to origin HEAD]
    C1a --> D[discoverSkills]
    C1d --> D
    C1e --> D
    D --> E[filterSkills via select:]
    E --> F[resolveAgents — `[*]` → detected installed]
    F --> G[for each agent × skill: installSkill]
    G --> H[mkdir staging .gaal-skill-tmp-*]
    H --> I[walkDir src → copyFile staging]
    I --> J[RemoveAll dst → Rename staging dst]
    J --> K[writeSkillSnapshot dst]
```

## Atomic install (PR #204 / #121)

`installSkill` stages the entire copy under a sibling temp directory
then atomically swaps:

```
parent/
├── target-skill/                      ← previous content (untouched)
└── .gaal-skill-tmp-XXXXXXXX/          ← staging — the new content
                                          (renamed → target-skill on success)
```

A crash mid-copy leaves either the previous skill intact or the new
one fully installed — never a half-written tree the next sync would
treat as up-to-date. Files removed upstream disappear because `dst`
is replaced wholesale.

## Frontmatter parsing (PR #197 / #133)

`SKILL.md` files start with a YAML frontmatter block:

```markdown
---
name: my-skill
description: A short blurb.
---

# Body...
```

`scan.go` uses `yaml.Unmarshal` over a CRLF-tolerant frontmatter
extractor — replaces the previous hand-rolled `strings.Cut(":")`
parser that misbehaved on quoted values, multi-line strings, and
escaped colons.

## Name validation (PR #195 / #131)

`isSafeSkillDirName` rejects:

- Empty names, `.`, `..`
- Names containing path separators (`/`, `\`)
- Anything `filepath.Clean` rewrites (would imply traversal)

Applied to both directory names and frontmatter `name:` fields so a
malicious source cannot escape its install root.

## File mode (PR #196 / #132)

`copyFile` masks the source mode to `0o644` (or `0o755` if the source
has the exec bit set). This drops setuid/setgid/sticky bits and group
write permissions that a malicious archive might carry.

## Local skill cache layout

| OS | Cache root | Full path |
|----|------------|-----------|
| Linux | `$XDG_CACHE_HOME` or `~/.cache` | `~/.cache/gaal/skills/<key>/` |
| macOS | `~/Library/Caches` | `~/Library/Caches/gaal/skills/<key>/` |
| Windows | `%LocalAppData%` | `%LocalAppData%\gaal\skills\<key>\` |

Sandbox-aware: in `--sandbox <dir>` mode, the entire cache lives under
`<dir>`.

## Installation scope

| `global: false` (default) | `global: true` |
|---------------------------|----------------|
| `<workDir>/<agent.ProjectSkillsDir>/` | `<home>/<agent.GlobalSkillsDir>/` |
| Versionable alongside the project | Shared across all projects |

The agent path is resolved via `agent.SkillDir(name, global, home)` —
see [`packages/core-agent.md`](core-agent.md). Some agents share a
`generic` skills tree (`.agents/skills`) — the redirect happens
transparently inside `SkillDir`.

## Related

- [`packages/core-vcs.md`](core-vcs.md) — backends used to clone / update sources
- [`packages/core-agent.md`](core-agent.md) — agent registry and path expansion
- [`packages/discover.md`](discover.md) — snapshot writer for drift detection
- [`commands/sync.md`](../commands/sync.md#2--skillmanagersync-sequential-per-source) — high-level flow
