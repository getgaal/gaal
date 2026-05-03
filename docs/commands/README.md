# Commands — gaal

> One page per CLI command. Each page covers what the command does, the
> end-to-end execution flow (rendered as a `mermaid` diagram), the
> caching and on-disk effects, the failure modes, and the exit codes.

For the architectural overview that ties commands together (Cobra
bootstrap, `PersistentPreRunE`, sandbox, telemetry), see
[`docs/architecture.md`](../architecture.md).

## Index

| Command | Description |
|---------|-------------|
| [`gaal sync`](sync.md) | Clone/update repos, install skills, upsert MCP entries (one-shot or `--service`) |
| [`gaal status`](status.md) | Snapshot of the current resource state on disk vs. config |
| [`gaal info`](info.md) | Detailed per-entry card for a resource type |
| [`gaal init`](init.md) | Bootstrap a `gaal.yaml` (empty, imported, or via wizard) |
| [`gaal audit`](audit.md) | Discover every skill / MCP server present on this machine |
| [`gaal agents`](agents.md) | List registered coding agents and detect installed ones |
| [`gaal doctor`](doctor.md) | Configuration health checks + actionable hints |
| [`gaal migrate`](migrate.md) | (Stub) Migrate to a Community Edition instance |
| [`gaal schema`](schema.md) | Emit the JSON Schema for `gaal.yaml` |
| [`gaal version`](version.md) | Version string and build timestamp |

## Conventions used in these pages

- **`mermaid` diagrams** depict the runtime flow. Subgraphs scope to a
  package; rectangles are functions; diamonds are decisions; rounded
  terminals are exits.
- **Exit codes** are listed explicitly per command. `gaal` uses a
  `cmd.ExitCodeError{Code,Cause}` wrapper so non-zero exits flow
  through Cobra's `RunE` and still hit `PersistentPostRunE` (telemetry
  flush, consent persistence) — see [`packages/telemetry.md`](../packages/telemetry.md).
- **Side effects** sections list every path the command reads or writes,
  along with the secfile / atomicity semantics (see
  [`packages/secfile.md`](../packages/secfile.md)).
