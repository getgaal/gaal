# The `gaal status` command — Reading guide

> **Audience:** This guide is for anyone who has never used `gaal` before
> and wants to understand what `gaal status` prints without reading the source code.

---

## How it works

`gaal` reads a configuration file (`gaal.yaml`) that declares three types of
resources to manage on your machine:

| Type | Description |
|------|-------------|
| **Repositories** | Code repositories (git, svn, hg, …) to clone and keep up-to-date locally |
| **Skills** | Collections of `SKILL.md` files to install into your AI agent directories (GitHub Copilot, Claude, Cursor, …) |
| **MCP Configs** | MCP (*Model Context Protocol*) server entries to inject into your agent JSON configuration files |

`gaal status` **does nothing** — it only **reads the current state from disk** and tells
you whether what you declared in `gaal.yaml` matches what is actually installed.

To synchronise (install / update), use `gaal sync`.

---

## Screen layout

The output is divided into **four sections** displayed one after the other.

---

### Section 1 — Repositories

```
── Repositories  (N) ──
┌──────────────┬──────┬────────────────┬──────────────────┐
│ PATH         │ TYPE │ STATUS         │ VERSION / URL    │
```

| Column | Meaning |
|--------|---------|
| **PATH** | Local path where the repository is (or should be) cloned, relative to the current directory |
| **TYPE** | Protocol in use: `git`, `hg`, `svn`, `bzr`, `tar`, `zip` |
| **STATUS** | Current state of the repository (see table below) |
| **VERSION / URL** | For cloned repos: current version + wanted version. For missing repos: source URL |

**Possible STATUS values:**

| Icon | Label | Meaning |
|------|-------|---------|
| ✓ | `synced` | Cloned and at the correct version |
| ⚠ | `dirty` | Cloned but the local version does not match what the config requests |
| ~ | `not cloned` | Not yet downloaded to disk — run `gaal sync` |
| ? | `unmanaged` | Present on disk but not declared in the config |
| ✗ | `error` | Error during the check (permissions, network, …) |

---

### Section 2 — Skills

```
── Skills  (N) ──
┌──────────────────┬────────────┬───────────┬────────────┬──────────────┐
│ SKILL            │ SOURCE     │ SCOPE     │ STATUS     │ INSTALLED IN │
```

This is the most important section if you work with AI agents.

#### Key concepts

A **skill** is a directory containing a `SKILL.md` file that AI agents read to obtain
specialised instructions (e.g. "how to write performant React"). `gaal` copies these
directories into the correct agent directories on your machine.

A **source** is the GitHub repository (or local path) from which `gaal` downloads
skills. For example `vercel-labs/agent-skills` is a public GitHub repository.

#### Columns

| Column | Meaning |
|--------|---------|
| **SKILL** | Name of the skill (extracted from `SKILL.md`) |
| **SOURCE** | Where the skill comes from: a GitHub repository (`owner/repo`) or a local path |
| **SCOPE** | `global` = installed in `~/.copilot/skills` (for all your projects) · `workspace` = installed in `.github/skills` (this project only) |
| **STATUS** | Installation state (see table below) |
| **INSTALLED IN** | Which agents currently have this skill on disk |

**Possible STATUS values:**

| Icon | Label | Meaning |
|------|-------|---------|
| ✓ | `synced` | The skill is installed and its files exactly match the source |
| ⚠ | `dirty` | The skill is installed but some files have been modified locally since the last sync |
| ~ | `partial` | Declared in the config but not yet installed — run `gaal sync` |
| ? | `unmanaged` | Found on disk in an agent directory but not declared in the config |
| ✗ | `error` | Error (source not cached, unknown agent, …) |

**Possible INSTALLED IN values:**

| Value | Meaning |
|-------|---------|
| `all` (green) | Installed in all agents targeted by the config |
| `none` (yellow) | Not yet installed in any agent — `gaal sync` required |
| list of names | Installed only in these agents (a subset of the targeted agents) |

> **Reading tip:** a `~ partial | none` line means the skill is declared in
> `gaal.yaml` but its source has not been downloaded locally yet. Run
> `gaal sync` to fix this.

#### Example rows

```
│ react-best-practices │ vercel-labs/agent-skills │ workspace │ ✓ synced  │ all  │
```
→ The `react-best-practices` skill, sourced from the `vercel-labs/agent-skills` GitHub
repository, is installed in the project's `.github/skills` directory and is in sync
across all targeted agents.

```
│ canvas-design        │ anthropics/skills        │ global    │ ✓ synced  │ all  │
```
→ This skill is installed **globally** (`~/.copilot/skills`): it is available in all
your projects, not just the current one.

```
│ vercel-composition…  │ vercel-labs/agent-…      │ workspace │ ~ partial │ none │
```
→ This skill is in your config but has not been downloaded yet. Run `gaal sync`.

---

### Section 3 — MCP Configs

```
── MCP Configs  (N) ──
┌────────────┬────────────┬───────────────────────────────────┐
│ NAME       │ STATUS     │ TARGET                            │
```

| Column | Meaning |
|--------|---------|
| **NAME** | Name of the MCP entry as declared in `gaal.yaml` |
| **STATUS** | Configuration state (see table below) |
| **TARGET** | Agent JSON config file where the entry should be injected |

**Possible STATUS values:**

| Icon | Label | Meaning |
|------|-------|---------|
| ✓ | `present` | The entry is present in the target file and matches the config |
| ⚠ | `dirty` | The entry exists but has been modified locally since the last sync |
| ~ | `absent` | The entry is not in the target file — `gaal sync` required |
| ✗ | `error` | Cannot read / write the target file |

---

### Section 4 — Supported Agents

```
── Supported Agents  (N) ──
┌───────────────┬───────────┬────────────────────┬───────────────────┬───────────────────┐
│ AGENT         │ INSTALLED │ PROJECT SKILLS DIR │ GLOBAL SKILLS DIR │ PROJECT MCP CONFIG│
```

This section is **informational only** — it shows which AI agents `gaal` knows about
and which ones are detected as present on your machine.

| Column | Meaning |
|--------|---------|
| **AGENT** | Agent identifier (e.g. `github-copilot`, `cursor`, `claude-code`) |
| **INSTALLED** | `✓` if the agent's configuration directory exists on the machine · `—` if absent |
| **PROJECT SKILLS DIR** | Directory (relative to the project) where `gaal` installs workspace skills for this agent |
| **GLOBAL SKILLS DIR** | Absolute path (`~/…`) where `gaal` installs global skills for this agent |
| **PROJECT MCP CONFIG** | Agent JSON file where MCP entries are injected |

> `gaal` only syncs to an agent if it is **installed** (`✓`). Agents marked `—`
> are skipped during `gaal sync`.

---

## Status at a glance

| Icon | Colour | Quick meaning |
|------|--------|---------------|
| ✓ synced / present | green | Everything is up to date |
| ⚠ dirty | yellow | Modified locally since the last sync |
| ~ partial / not cloned / absent | yellow | Declared but not yet installed → `gaal sync` |
| ? unmanaged | cyan | Present on disk but not in the config |
| ✗ error | red | Problem to fix (see the error message) |

---

## Typical workflow

```
1. Edit gaal.yaml          → add / modify resources
2. gaal status             → see what is out of sync
3. gaal sync               → install / update everything
4. gaal status             → confirm everything is ✓ synced
```
