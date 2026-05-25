# gaal

[![Release](https://img.shields.io/github/v/release/getgaal/gaal)](https://github.com/getgaal/gaal/releases)
[![CI](https://github.com/getgaal/gaal/actions/workflows/ci.yml/badge.svg)](https://github.com/getgaal/gaal/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](go.mod)
[![GitHub stars](https://img.shields.io/github/stars/getgaal/gaal?style=social)](https://github.com/getgaal/gaal/stargazers)

> **G**overned **A**gent **A**ccess **L**ayer — a single CLI to keep your local repositories, AI agent skills, and MCP server configurations in sync.

📚 Full docs: [docs.getgaal.com](https://docs.getgaal.com)

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

If you use more than one AI coding agent (Claude Code, Cursor, Codex, GitHub Copilot, …) across more than one machine, gaal keeps your skills, MCP servers, and repositories in one YAML and reconciles every agent on every machine to match.

## What it does

| Resource | Description |
|----------|-------------|
| **Repositories** | Clone or update multi-protocol repos (git, hg, svn, bzr, tar, zip) from a single YAML file |
| **Skills** | Download and install `SKILL.md` collections into your local AI agent directories (Claude, Copilot, Cursor, …) |
| **MCPs** | Upsert MCP server entries into agent JSON config files without overwriting your existing configuration |

Plus a lightweight **tools** check: `gaal doctor` and `gaal sync` verify that required CLI binaries (e.g. `gh`, `fnm`) are on PATH and surface install hints when they are missing.

---

## The YAML

One file. Three resources. Every agent.

```yaml
schema: 1

repositories:
  - source: github.com/getgaal/gaal
    path: ~/code/gaal

skills:
  - source: github.com/obra/superpowers
    agents: ["*"]
    global: true

mcps:
  - inline:
      mcpServers:
        context7:
          command: npx
          args: ["-y", "@upstash/context7-mcp"]
    agents: ["claude-code", "cursor", "codex"]
```

Run `gaal sync`. gaal figures out which agents are installed and writes to the right place for each.

---

## Install

### Quick install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/getgaal/gaal/main/scripts/install.sh | sh
```

Installs the latest release binary to `~/.local/bin/gaal`. Pin a specific
version with `VERSION=v0.1.2`, or pick a different directory with
`INSTALL_DIR=/usr/local/bin`.

Pass `GAAL_INSTALL_DEBUG=1` for verbose output, or run
`curl -fsSL https://raw.githubusercontent.com/getgaal/gaal/main/scripts/install.sh | sh -s -- --help`
to see all options.

### With Go

```bash
go install github.com/getgaal/gaal@latest
```

### From source

**Prerequisites:** Go 1.26+

```bash
git clone https://github.com/getgaal/gaal.git
cd gaal
make build
```

The binary is written to `dist/gaal`. Copy it to your `$PATH`:

```bash
sudo cp dist/gaal /usr/local/bin/gaal
# or, user-local:
cp dist/gaal ~/.local/bin/gaal
```

---

## Quick start

The fastest way to get a working `gaal.yaml` is to run the interactive init
wizard. It scans your machine for installed skills and MCP servers, asks you
a few questions, and writes a configuration you can sync right away.

```bash
gaal init
```

The wizard asks two questions before doing anything:

1. **How** to create the file — start from an empty documented skeleton, or
   import the skills and MCP servers detected on this machine.
2. **Where** the configuration applies — project-scoped (`./gaal.yaml`) or
   global-scoped (`~/.config/gaal/config.yaml`).

When you pick the import mode, gaal runs an audit under the selected scope
and presents a multi-select list grouped by agent. Everything is preselected
by default; press Enter to confirm, or Space to toggle individual entries.

### Non-interactive (CI / scripts)

The wizard prompts are bypassable via flags:

```bash
# Empty skeleton, project-scoped
gaal init --empty --scope project

# Import everything detected, global-scoped, overwrite existing file
gaal init --import-all --scope global --force
```

`--empty` and `--import-all` are mutually exclusive; one of them is required
when stdin is not a TTY.

### Manual setup

If you would rather hand-write your configuration, copy `example.gaal.yaml`
and edit it. See [Configuration reference](#configuration-reference) for the
full data model, including the `tools:` block (bare strings or `{name, hint}`
objects) that documents required CLI binaries.

Then run:

```bash
gaal sync
```

---

## Why I built this

Last December I set up a new laptop and it took me half a day. Not because the OS was slow. Because I was re-cloning repos, re-installing the same fifteen skills into Claude Code, then doing it again into Cursor, then registering the same four MCP servers into three different JSON files because each agent reads config from a different place.

I had four config locations across two machines. Eight places where one source of truth should have lived. That's the problem gaal solves.

---

## Supported agents

gaal auto-detects 17 coding agents. Use `agents: ["*"]` and a skill or MCP lands in every detected agent. Extend the registry yourself for anything missing, see [Configuration reference](#configuration-reference).

| | | |
|---|---|---|
| Claude Code | Cursor | Codex |
| GitHub Copilot | Amp | Cline |
| Roo | Continue | Gemini CLI |
| Goose | Kilo | Kiro CLI |
| OpenCode | OpenHands | Trae |
| Warp | Windsurf | |

---

## Usage

### One-shot sync

```bash
gaal sync
```

Clones or updates all repositories, installs skills, and upserts MCP entries.

### Dry-run (preview changes)

```bash
gaal sync --dry-run
```

Runs the full sync planning pipeline but performs no writes to disk.
Prints what sync *would* do — which repos would be cloned or updated,
which skills would be installed, and which MCP entries would be created.

Supports `--output text|table|json` and `--sandbox`. Incompatible with `--service`.

Exit codes: **0** = nothing to change, **1** = changes pending, **2** = error.

### Remove orphan resources

```bash
gaal sync --prune
```

After syncing, removes skills and MCP entries that are no longer declared in the
configuration file. Incompatible with `--service`.

### Force-install into all agents

```bash
gaal sync --force
```

Installs skills into every registered agent even when the agent's configuration
directory does not exist yet. Useful when bootstrapping a machine where some
agents are not installed yet. Only applies to `agents: ["*"]` wildcard entries.

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

### List agents

```bash
gaal agents                  # all registered agents (installed first)
gaal agents --installed      # only agents detected on this machine
gaal agents cursor           # detailed view for one agent
gaal agents -o json          # machine-readable output
```

Lists every registered coding agent and whether it is installed on this machine.
Pass a name for a detailed view with search paths, skill counts, and MCP config.

### Health check

```bash
gaal doctor
```

Runs sanity checks on your configuration: validates gaal.yaml, checks that
skill sources are reachable, verifies MCP target files, and reports agent
and telemetry status.

```bash
gaal doctor --offline     # skip network checks
gaal doctor --no-upsell   # suppress the Community Edition message
gaal doctor -o json       # machine-readable JSON output
```

Exit codes: **0** = all checks passed, **1** = warnings, **2** = errors.

### Discover what is installed

```bash
gaal audit
```

Scans all known agent directories and reports every skill and MCP server found on
the machine, regardless of whether it is declared in `gaal.yaml`. Useful for
spotting skills installed manually or by another tool.

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
| `text`  | Human-friendly plain-text output (default) |
| `table` | Coloured pterm tables |
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

### Shell completion

gaal generates completion scripts for **bash**, **zsh**, **fish**, and **PowerShell**.

**Bash — current session**
```bash
source <(gaal completion bash)
```

**Bash — permanent** (add to `~/.bashrc`)
```bash
gaal completion bash > ~/.local/share/bash-completion/completions/gaal
```

**Zsh — current session**
```zsh
source <(gaal completion zsh)
```

**Zsh — permanent**
```zsh
gaal completion zsh > "${fpath[1]}/_gaal"
```

**Fish**
```fish
gaal completion fish | source
# or permanently:
gaal completion fish > ~/.config/fish/completions/gaal.fish
```

**PowerShell**
```powershell
gaal completion powershell | Out-String | Invoke-Expression
```

---

## Configuration reference

See [`docs/config.md`](docs/config.md) for the full technical reference:
data model, file locations by OS, merge rules, scope restriction policy
(including why workspace cannot override `telemetry`), schema generation,
validation, and agent contribution rules.

**Quick overview — the three config levels (lowest → highest priority):**

| Priority | File |
|----------|------|
| 1 — lowest | `/etc/gaal/config.yaml` (global) |
| 2 | `$XDG_CONFIG_HOME/gaal/config.yaml` (user; defaults to `~/.config/gaal/config.yaml` on Linux / macOS) |
| 3 — highest | `gaal.yaml` in CWD, or `--config` path |

See [`example.gaal.yaml`](example.gaal.yaml) for a fully annotated
configuration file.

### Agent registry customization

gaal ships with a built-in registry of supported coding agents. You can extend
it with your own agent definitions by creating a file at:

| OS | Path |
|----|------|
| Linux | `$XDG_CONFIG_HOME/gaal/agents.yaml` (defaults to `~/.config/gaal/agents.yaml`) |
| macOS | `$XDG_CONFIG_HOME/gaal/agents.yaml` (defaults to `~/.config/gaal/agents.yaml`) |
| Windows | `%AppData%\gaal\agents.yaml` |

Custom entries are merged with the built-in list. User-defined entries **can override** built-in entries. Each entry follows the same format as the built-in registry:

```yaml
agents:
  my-agent:
    project_skills_dir: .my-agent/skills   # relative path, no ".."
    global_skills_dir: ~/.my-agent/skills  # must start with ~/
    mcp_config_file: ~/.my-agent/mcp.json  # empty string if unsupported
```

Use `gaal agents` to list all registered agents (built-in + custom) and verify your additions. `gaal info agent` provides the same information in an alternative layout.

---

## Graduation path to gaal Community

When your team outgrows single-user gaal (shared configs, drift detection,
approval workflows), gaal Community Edition picks up where the standalone
CLI leaves off.

The migration command validates your current configuration and confirms it is
ready to push to a Community instance:

```bash
gaal migrate --to community https://community.example.com
gaal migrate --to community https://community.example.com --dry-run
```

Community Edition is not yet publicly available. Running `gaal migrate` today
validates your YAML and prints what would be migrated. Subscribe at
<https://getgaal.com> to be notified when Community ships.

---

## Development

```bash
make build     # compile to dist/gaal
make test      # run the full unit-test suite
make coverage  # unit tests + coverage reports in report/
make lint      # gofmt + go vet
make sandbox   # one-shot sync in an isolated /tmp directory
```

See [`docs/testing.md`](docs/testing.md) for the e2e test suite, and
[`docs/architecture.md`](docs/architecture.md) for a full description of the internals.

---

## License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0).

Copyright (C) 2026 @Theosakamg / @gmoigneu / @gregqualls .

This program is free software: you can redistribute it and/or modify it under
the terms of the GNU Affero General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version. See the [LICENSE](LICENSE) file for the full text.

---

## Privacy

gaal collects **no data by default**. You can opt in to anonymous usage
telemetry on first run. See the [Privacy Policy](PRIVACY_POLICY.md) for
full details on what is and isn't collected.
