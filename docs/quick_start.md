# Quick Start — gaal

> One YAML file to rule your repos, AI agent skills, and MCP servers.

---

## 1. Install

Download the binary for your platform from the [latest release](https://github.com/getgaal/gaal/releases/latest):

```bash
# Linux (amd64)
curl -Lo gaal https://github.com/getgaal/gaal/releases/latest/download/gaal-linux-amd64
chmod +x gaal && sudo mv gaal /usr/local/bin/

# macOS (Apple Silicon)
curl -Lo gaal https://github.com/getgaal/gaal/releases/latest/download/gaal-darwin-arm64
chmod +x gaal && sudo mv gaal /usr/local/bin/

# macOS (Intel)
curl -Lo gaal https://github.com/getgaal/gaal/releases/latest/download/gaal-darwin-amd64
chmod +x gaal && sudo mv gaal /usr/local/bin/

# Windows (amd64) — PowerShell
Invoke-WebRequest -Uri https://github.com/getgaal/gaal/releases/latest/download/gaal-windows-amd64.exe -OutFile gaal.exe
```

Verify the installation:

```bash
gaal version
```

---

## 2. Configure `gaal.yaml`

Create a `gaal.yaml` at the root of your workspace (or any directory you run `gaal` from).

> **Tip:** Run `gaal schema -f schema.json` once to get live YAML validation and auto-completion in VS Code and IntelliJ.

```yaml
# gaal.yaml

# ── Repositories ──────────────────────────────────────────────────────────────
# Clone or update multi-protocol repos (git, hg, svn, bzr, tar, zip).
# Keys are local paths relative to the working directory.
repositories:
  src/my-service:
    type: git
    url: https://github.com/my-org/my-service.git
    version: main            # branch, tag, or commit SHA

  src/legacy:
    type: svn
    url: https://svn.example.com/repos/legacy/trunk

# ── Skills ────────────────────────────────────────────────────────────────────
# Install SKILL.md collections into your local AI agent directories.
# agents: ["*"] auto-detects every installed agent on the machine.
skills:
  - source: vercel-labs/agent-skills   # GitHub shorthand owner/repo
    agents: ["*"]                       # auto-detect: copilot, cursor, claude-code…
    global: false                       # project-local install

  - source: anthropics/skills
    agents:
      - github-copilot
      - claude-code
    global: true                        # user-wide install (~/.copilot/skills, etc.)

# ── MCP Servers ───────────────────────────────────────────────────────────────
# Upsert MCP server entries into agent JSON config files.
# Existing entries are preserved; only the named key is updated.
mcps:
  - name: filesystem
    agents: ["claude-code"]
    global: true
    inline:
      command: uvx
      args: [mcp-server-filesystem, /home/user/projects]

  - name: filesystem
    agents: ["github-copilot"]         # VS Code / GitHub Copilot
    global: true
    inline:
      command: uvx
      args: [mcp-server-filesystem, /home/user/projects]
```

### Configuration reference

For the complete field reference, merge rules, scope restriction policy, and
agent rules, see [**docs/config.md**](config.md).

Quick reference:

| Section | Key | Description |
|---------|-----|-------------|
| `repositories` | `type` | `git`, `hg`, `svn`, `bzr`, `tar`, `zip` |
| `repositories` | `version` | Branch, tag, commit, or SVN revision |
| `skills` | `source` | GitHub shorthand, full URL, SSH URL, or local path |
| `skills` | `agents` | Agent names or `["*"]` to auto-detect |
| `skills` | `global` | `true` = user-wide, `false` = project-local (default) |
| `skills` | `select` | Specific skill names to install (empty = all) |
| `mcps` | `agents` | Agent names or `["*"]` to target all agents with a non-empty MCP config |
| `mcps` | `global` | `true` = user-wide agent config, `false` = project-scoped (default) |
| `mcps` | `target` | _(deprecated)_ Explicit path to the agent JSON config file; prefer `agents` + `global` |
| `mcps` | `inline` | Inline server definition (mutually exclusive with `source`) |
| `mcps` | `source` | URL to a remote JSON file containing an `mcpServers` block |
| `mcps` | `merge` | `true` (default) = upsert; `false` = overwrite |
| _(top-level)_ | `telemetry` | `true` / `false`: opt in/out of anonymous usage telemetry (only `global` and `user` config files; workspace cannot override — see [docs/config.md](config.md#scope-restriction-policy)) |

Supported agent names: `amp`, `claude-code`, `cursor`, `github-copilot`, `cline`, `roo`, `codex`, `continue`, `gemini-cli`, `goose`, `kilo`, `kiro-cli`, `opencode`, `openhands`, `trae`, `warp`, `windsurf`, and more. Run `gaal info agent` for the full list.

---

## 3. Sync

Run a one-shot synchronisation:

```bash
gaal sync
```

This single command:

1. **Clones or updates** every repository listed under `repositories`.
2. **Downloads and installs** every skill collection into the correct agent directories.
3. **Upserts** every MCP server entry into the target JSON config files.

That's it — your repos, skills, and MCP servers are now configured and centralised.

### Optional: run as a background service

Keep everything up to date automatically:

```bash
gaal sync --service --interval 10m
```

Runs a sync loop every 10 minutes. Handles `SIGTERM` / `Ctrl-C` cleanly.

---

## Next step: maintain the registry and detect drift

Once your initial sync is done, two commands help you stay in control.

### `gaal status` — detect drift

Shows the current installation state of every skill and repository compared to your configuration:

```bash
gaal status
```

```
Skills
  source                     agent           global  installed  missing  modified
  vercel-labs/agent-skills   github-copilot  no      12         0        1
  anthropics/skills          claude-code     yes     5          0        0

Repositories
  path             type  url                                  version  state
  src/my-service   git   https://github.com/my-org/…          main     ok
  src/legacy       svn   https://svn.example.com/…            trunk    behind
```

- **missing** — skill present in source but not installed locally → run `gaal sync`.
- **modified** — installed file differs from source → local edit or upstream update; sync will restore it.
- **behind** — repository has upstream commits not fetched locally → run `gaal sync`.

### `gaal audit` — discover what is installed

Scans all known agent directories and reports every skill found on the machine, regardless of whether it is declared in your `gaal.yaml`:

```bash
gaal audit
```

Useful to spot skills installed manually or by another tool (drift from the registry), and to identify which agent owns which skill directory.

### `gaal info` — inspect the registry

Get a detailed card for any resource type:

```bash
gaal info skill                          # all skill entries
gaal info skill vercel-labs/agent-skills # filter by source
gaal info repo src/my-service
gaal info mcp filesystem
gaal info agent github-copilot           # agent directory layout
```

### Keeping the registry up to date

| Scenario | Action |
|----------|--------|
| New skill repo available upstream | Add a `skills` entry to `gaal.yaml`, run `gaal sync` |
| Pin a repository to a new version | Update `version:` in `gaal.yaml`, run `gaal sync` |
| Add a new MCP server | Add an `mcps` entry, run `gaal sync` |
| Check for upstream skill changes | `gaal status` → look at the **modified** column |
| Audit untracked skills | `gaal audit` → compare with `gaal.yaml` |
| CI / scheduled sync | `gaal sync --service --interval 1h` or a cron job calling `gaal sync` |

---

## Sandbox mode (safe testing)

Run any command in an isolated directory without touching your real config:

```bash
gaal --sandbox /tmp/gaal-test --config gaal.yaml sync
```

All writes (clones, skill installs, MCP edits) are redirected inside `/tmp/gaal-test`. Your real `$HOME` is untouched.
