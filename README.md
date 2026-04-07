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

**Prerequisites:** Go 1.22+

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
