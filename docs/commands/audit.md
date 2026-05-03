# `gaal audit`

> Discover every skill and MCP server installed on this machine,
> regardless of whether it is declared in `gaal.yaml`.

`audit` is the **config-independent** counterpart to `gaal status`:
where status shows declared resources reconciled with disk, audit shows
**only what is on disk** — useful for spotting skills installed
manually, by another tool, or by a previous `gaal` config that was
since edited.

## Usage

```
gaal audit
```

Inherits global flags only. The `--config` is optional — `audit` works
without any config file present (it sets the engine to an empty
config).

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Inventory rendered |
| `2` | Discovery scan failed (rare; FS error) |

---

## Flow

```mermaid
flowchart TD
    A[gaal audit] --> B[PreRunE: load config optional]
    B --> C[telemetry.Track audit]
    C --> D[engine.NewWithOptions empty config]
    D --> E[engine.Audit ctx, format]
    E --> F[ops.Audit]
    F --> G[discover.Scan ctx, home, workDir, opts]

    subgraph Scan
      G --> G1[scanGlobal — agent.List × global+user dirs]
      G --> G2[scanMCPs — agent.List × MCP config files]
      G --> G3[scanWorkspace — depth-limited FS walk if opts.IncludeWorkspace]
    end

    G1 --> H[buildAuditReport]
    G2 --> H
    G3 --> H
    H --> R{format}
    R -->|text| RT[compact summary]
    R -->|verbose| RV[full per-skill / per-mcp lines]
    R -->|table| RB[pterm boxed tables]
    R -->|json| RJ[stdout JSON]
    RT --> Z([exit 0])
    RV --> Z
    RB --> Z
    RJ --> Z
```

## What audit scans

For each registered agent (from
[`internal/core/agent`](../packages/core-agent.md)):

- **Project skills**: `<workDir>/<ProjectSkillsSearch>/...` (1-level deep)
- **Global skills**: `~/<GlobalSkillsSearch>/...` (1-level deep)
- **PM (package manager) skills**: `~/<PmSkillsSearch>/...` (recursive)
- **MCP config**: agent's `ProjectMCPConfigPath` and `GlobalMCPConfigPath`

PR #193 (#137) added `scanMCPs` coverage of both project- and
global-scoped MCP files; PR #194 (#128) added `claude-desktop` install
detection.

## Drift annotation

For every discovered skill, `audit` annotates whether it is **managed**
(declared in the merged config) or **unmanaged**. Detection uses the
same `discover.Scan` snapshot machinery as `status` so the labels are
consistent across the two commands. See
[`docs/packages/discover.md`](../packages/discover.md).

---

## Side effects

Read-only.

## Related

- [`gaal status`](status.md) — config-aware view.
- [`docs/packages/discover.md`](../packages/discover.md) — scan internals.
