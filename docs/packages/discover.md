# `internal/discover`

> FS-first resource discovery + Git-inspired snapshot drift detection.

> **Pillar reference:** the full discovery pillar — resource model,
> snapshot lifecycle, scan options, per-resource-type scan logic — lives
> in [`docs/discover.md`](../discover.md). This page is the
> package-level summary.

## Public API

| Symbol | Description |
|--------|-------------|
| `Scan(ctx, home, workDir string, opts ScanOptions) ([]Resource, error)` | Public entry point |
| `ScanOptions` | `MaxDepth`, `IncludeWorkspace`, `StateDir`, `Timeout` |
| `Resource` | `{Type, Scope, Path, Name, Drift, VCSType, Managed, Meta}` |
| `Snapshot` | `map[string]FileRecord` |
| `Load`, `Save`, `Record`, `SnapshotDir`, `DiffPath`, `SnapshotPath`, `WorkdirKey` | Snapshot helpers |
| `ResourceType`, `Scope`, `DriftState` | Enum types and constants |

## Drift heuristic

```mermaid
flowchart LR
    A[FS entry] --> B{vcs marker present?}
    B -- yes --> C[vcs.HasChanges fast path]
    B -- no --> D{stat: size+mtime match snapshot?}
    D -- yes --> OK([drift: ok])
    D -- no --> E[sha256 of file]
    E --> F{hash matches snapshot?}
    F -- yes --> R[update snapshot mtime — racy-git repair]
    F -- no --> M([drift: modified])
    R --> OK
    C --> OK
```

## Snapshot lifecycle

```mermaid
flowchart TD
    A[gaal sync] --> B[manager Sync]
    B --> C[install / merge / clone]
    C --> D[writeSkillSnapshot / writeMCPSnapshot]
    D --> E[discover.SnapshotDir or Record]
    E --> F[discover.Save SnapshotPath stateDir, key]

    G[gaal status / info] --> H[discover.Scan]
    H --> I[per-resource-type drift]
    I --> J[discover.DiffPath root, snap]
    J --> K[Resource Drift annotation]
```

## State directory

| OS | Default path |
|----|-------------|
| Linux | `~/.cache/gaal/state/` |
| macOS | `~/Library/Caches/gaal/state/` |
| Windows | `%LocalAppData%\gaal\state\` |

Sandbox-aware. Snapshots are keyed by `WorkdirKey(path)` (8-char hex of
`sha256(path)[:4]`) so different installation paths for the same skill
name never collide.

## Per-resource-type scan

| File | Scope |
|------|-------|
| `global.go` | Agent-registry skill dirs (global + user) |
| `mcp.go` | Agent MCP config files (project + global since PR #193) |
| `workspace.go` | Depth-limited FS walk for repos and skills in `workDir` |

## Recent fixes worth knowing

| PR | Issue | Effect |
|----|-------|--------|
| #193 | #137 | `scanMCPs` now scans both `ProjectMCPConfigPath` and `GlobalMCPConfigPath` |

## Related

- [`docs/discover.md`](../discover.md) — full pillar description
- [`commands/status.md`](../commands/status.md), [`commands/audit.md`](../commands/audit.md) — main consumers
- [`packages/secfile.md`](secfile.md) — atomic snapshot writes
