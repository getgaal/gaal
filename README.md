# gaal

> **G**it **A**gent **A**utomation **L**ayer — a single CLI to keep your local repositories, AI agent skills, and MCP server configurations in sync.

```
  ██████╗  █████╗  █████╗ ██╗
 ██╔════╝ ██╔══██╗██╔══██╗██║
 ██║  ███╗███████║███████║██║
 ██║   ██║██╔══██║██╔══██║██║
 ╚██████╔╝██║  ██║██║  ██║███████╗
  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝
  Repository · Skills · MCP
```

---

## What it does

| Resource | Description |
|----------|-------------|
| **Repositories** | Clone or update multi-protocol repos (git, hg, svn, bzr, tar, zip) from a single YAML file |
| **Skills** | Download and install `SKILL.md` collections into your local AI agent directories (Claude, Copilot, Cursor, …) |
| **MCPs** | Upsert MCP server entries into agent JSON config files without overwriting your existing configuration |

---

## Quick start

Create a `gaal.yaml` in your project (or copy `example.gaal.yaml`):

```yaml
repositories:
  src/myrepo:
    type: git
    url: https://github.com/example/myrepo.git
    version: main

skills:
  - source: vercel-labs/agent-skills
    agents: ["*"]

mcps:
  - name: filesystem
    target: ~/.config/claude/claude_desktop_config.json
    inline:
      command: uvx
      args: [mcp-server-filesystem, /home/user/projects]
```

Then run:

```bash
gaal sync
```

---

## Usage

### One-shot sync

```bash
gaal sync
```

Clones or updates all repositories, installs skills, and upserts MCP entries.

### Continuous service mode

```bash
gaal sync --service --interval 10m
```

Runs a sync loop every 10 minutes. Handles `SIGTERM` / `Ctrl-C` cleanly.

### Status report

```bash
gaal status
```

Prints the current state of every repository, skill, and MCP entry.

### Detailed info

```bash
gaal info <repo|skill|mcp|agent> [name]
```

Shows a full information card for every entry of the given package type, combining the configuration spec with the current runtime state.

```bash
gaal info skill                          # all skill entries
gaal info skill vercel-labs/agent-skills # filter by source (substring, case-insensitive)
gaal info repo workspace/myrepo
gaal info mcp claude
gaal info agent                          # list all registered agents
gaal info agent cursor
```

### Generate the JSON Schema

```bash
gaal schema
```

Prints the JSON Schema (draft-07) that describes the full structure of a `gaal.yaml`
configuration file. Useful for IDE validation, documentation, and LLM JSON mode.

```bash
gaal schema -f schema.json   # write to a file
```

**VS Code:** the workspace `settings.json` already maps `schema.json` to every
`*.gaal.yaml` file — run `gaal schema -f schema.json` once and YAML
auto-completion / inline validation activate automatically.

**GoLand / IntelliJ:** go to _Languages & Frameworks → Schemas and DTDs →
JSON Schema Mappings_, add `schema.json` and associate it with your `gaal.yaml` files.

---

### Output format

Both `status` and `info` support the `-o` / `--output` flag:

```bash
gaal status -o json        # machine-readable JSON
gaal info repo -o json
```

| Format | Description |
|--------|-------------|
| `table` | Human-friendly coloured tables (default) |
| `json`  | Structured JSON, suitable for scripting / CI |

When `--output json` is set the ASCII banner is automatically suppressed.

### Sandbox mode (safe for CI / testing)

```bash
gaal --sandbox /tmp/my-sandbox sync
```

Redirects all writes to the sandbox directory. Nothing outside it is touched.

### Custom config file

```bash
gaal --config /path/to/custom.yaml sync
```

### Suppress the banner

```bash
gaal --no-banner sync
```

### Verbose / debug output

```bash
gaal --verbose sync
```

### Log to file (JSON)

```bash
gaal --log-file /var/log/gaal.json sync
```

---

## Configuration reference

See [`example.gaal.yaml`](example.gaal.yaml) for a fully annotated configuration.

gaal merges up to three configuration files in order:

| Priority | File |
|----------|------|
| 1 — lowest | `/etc/gaal/config.yaml` (global) |
| 2 | `~/.config/gaal/config.yaml` (user) |
| 3 — highest | `gaal.yaml` in CWD, or `--config` path |

### Agent registry customization

gaal ships with a built-in registry of supported coding agents (claude-code, github-copilot, cursor, windsurf, …). You can extend it with your own agent definitions by creating a file at:

| OS | Path |
|----|------|
| Linux | `$XDG_CONFIG_HOME/gaal/agents.yaml` (defaults to `~/.config/gaal/agents.yaml`) |
| macOS | `~/Library/Application Support/gaal/agents.yaml` |
| Windows | `%AppData%\gaal\agents.yaml` |

Custom entries **extend** the built-in list — they cannot override built-in entries. Each entry follows the same format as the built-in registry:

```yaml
agents:
  my-agent:
    project_skills_dir: .my-agent/skills   # relative path, no ".."
    global_skills_dir: ~/.my-agent/skills  # must start with ~/
    mcp_config_file: ~/.my-agent/mcp.json  # empty string if unsupported
```

Use `gaal info agent` to list all registered agents (built-in + custom) and verify your additions.

---

## Development

```bash
make build     # compile to dist/gaal
make test      # run the full test suite
make coverage  # tests + coverage reports in report/
make lint      # go vet
make sandbox   # one-shot sync in an isolated /tmp directory
```

See [`docs/architecture.md`](docs/architecture.md) for a full description of the internals.

---

## Install from source

**Prerequisites:** Go 1.26+

```bash
git clone https://github.com/gmg-inc/gaal-lite.git
cd gaal-lite
make build
```

The binary is written to `dist/gaal`. Copy it to your `$PATH`:

```bash
sudo cp dist/gaal /usr/local/bin/gaal
# or, user-local:
cp dist/gaal ~/.local/bin/gaal
```

To install directly with Go:

```bash
go install github.com/gmg-inc/gaal-lite@latest
```

---

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0).

Copyright (C) 2026 @Theosakamg / @gmoigneu / @gregqualls .

This program is free software: you can redistribute it and/or modify it under
the terms of the GNU Affero General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version. See the [LICENSE](LICENSE) file for the full text.
