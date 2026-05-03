# `gaal info`

> Detailed per-entry card for a single resource type. Read-only.

## Usage

```
gaal info <repo|skill|mcp|agent> [filter]
```

| Argument | Description |
|----------|-------------|
| Resource type | One of `repo`, `skill`, `mcp`, `agent` |
| Filter | Optional case-insensitive substring match against the resource name |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Rendered (even if zero matches) |
| `2` | Could not load config or resolve resource |

---

## Flow

```mermaid
flowchart TD
    A[gaal info type filter] --> B[engine.Info ctx, type, filter, fmt]
    B --> C[ops.Info]
    C --> D{type}
    D -->|repo| E1[ops.Collect → repos slice]
    D -->|skill| E2[ops.Collect → skills slice]
    D -->|mcp| E3[ops.Collect → mcps slice]
    D -->|agent| E4[agent.List + per-agent path expansion]
    E1 --> F[filter substring]
    E2 --> F
    E3 --> F
    E4 --> F
    F --> G{output format}
    G -->|text| H[compact summary lines]
    G -->|verbose| I[full per-card detail]
    G -->|table| J[pterm table]
    G -->|json| K[stdout JSON]
```

`info` reuses the same `ops.Collect` as `gaal status`, then renders a
**deeper** view: instead of the one-line-per-resource summary it emits a
multi-line card per matching entry (paths, version, install targets,
detected drift, source URL — credentials redacted via
[`urlx.Redact`](../packages/urlx.md)).

## Output format honoring `--verbose`

PR #186 (#129) routes `gaal info` through `effectiveOutputFormat()` so
`--verbose` produces the full card in text mode (rather than the
default compact summary). Without `--verbose`:

- `text` → one summary line per match
- `verbose` (or `-v`) → full card per match
- `table` / `json` → unaffected by `--verbose` (always full detail)

---

## Side effects

Read-only: `gaal info` reads the same paths as `gaal status` (config
chain + agent dirs + snapshots). It writes nothing.

## Related

- [`gaal status`](status.md) — summary view of the same data.
- [`gaal agents`](agents.md) — `gaal info agent <name>` is a superset.
