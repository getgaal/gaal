# Packages — gaal

> One page per Go package under `internal/`. Each page covers what the
> package owns, its public API, the runtime flow (rendered as a
> `mermaid` diagram), and where it sits in the dependency graph.

For Pillars-level overviews, see the existing top-level docs:

- [`docs/architecture.md`](../architecture.md) — package map and bootstrap
- [`docs/config.md`](../config.md) — config / scope / schema / validation / platform / template pillars
- [`docs/core.md`](../core.md) — VCS + Agent Registry pillars
- [`docs/discover.md`](../discover.md) — discovery + snapshot pillar

Per-package detail below; per-command flows in
[`docs/commands/`](../commands/).

## Index

### Domain layer

| Package | Page | Role |
|---------|------|------|
| `internal/config` | [config.md](config.md) | YAML loading, merge chain, schema, validation |
| `internal/core/agent` | [core-agent.md](core-agent.md) | Agent registry, path expansion |
| `internal/core/vcs` | [core-vcs.md](core-vcs.md) | VCS interface and backends |
| `internal/discover` | [discover.md](discover.md) | FS-first discovery + snapshot drift |
| `internal/repo` | [repo.md](repo.md) | Repository manager (uses `core/vcs`) |
| `internal/skill` | [skill.md](skill.md) | Skill manager (uses `core/vcs` + `core/agent`) |
| `internal/mcp` | [mcp.md](mcp.md) | MCP entry upsert (JSON / TOML) |

### Orchestration layer

| Package | Page | Role |
|---------|------|------|
| `internal/engine` | [engine.md](engine.md) | Single coupling point between cmd and managers |
| `internal/engine/ops` | [engine-ops.md](engine-ops.md) | Per-command operations (status, audit, init…) |
| `internal/engine/render` | [engine-render.md](engine-render.md) | Output types + renderers |

### Support layer

| Package | Page | Role |
|---------|------|------|
| `internal/httpx` | [httpx.md](httpx.md) | Hardened outbound HTTP client (TLS min, redirect SSRF, body cap, UA) |
| `internal/installscript` | [installscript.md](installscript.md) | `curl \| sh` installer payload generation |
| `internal/logger` | [logger.md](logger.md) | Console + JSON file slog handlers |
| `internal/runner` | [runner.md](runner.md) | Subprocess execution adapter (TTY spinner) |
| `internal/secfile` | [secfile.md](secfile.md) | Atomic 0o600 writes (temp + fsync + rename) |
| `internal/telemetry` | [telemetry.md](telemetry.md) | Anonymous usage telemetry, consent-gated |
| `internal/tools` | [tools.md](tools.md) | External-tool PATH probe |
| `internal/urlx` | [urlx.md](urlx.md) | URL validation + credential redaction |

## Conventions used in these pages

- **`mermaid` diagrams** depict either the dependency graph or the
  per-call flow (whichever is more revealing for the package).
- **Public API** tables list only **exported** symbols. Unexported
  helpers are described in prose where they matter for understanding.
- Pages defer to the top-level Pillars docs (`config.md`, `core.md`,
  `discover.md`) for sub-package detail rather than duplicating it.
