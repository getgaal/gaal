# Agent Capabilities Reference

> Per-agent reference of the on-disk surface gaal cares about — **skills**,
> **plugins/extensions**, **MCP config**, and **behavioural quirks** that drive
> the manager's per-agent rules. This document is the research backing
> [issue #208 — Factorize agent-specific behaviour into a dedicated
> factory/class](https://github.com/getgaal/gaal/issues/208).
>
> Source of truth for the on-disk *paths* is
> [`internal/core/agent/agents.yaml`](../internal/core/agent/agents.yaml).
> This file is the *narrative* counterpart: what each agent actually supports,
> in what format, with what gotchas, and where the canonical vendor docs live.
>
> Last reviewed: 2026-05-15.

## Reading this document

For every agent four sections are filled in:

- **Skills** — native `SKILL.md` support vs. convention-only; project + global
  paths; frontmatter schema.
- **Plugins / extensions** — extension mechanism, on-disk paths, manifest
  format, install workflow.
- **MCP config** — file path(s), format, top-level key, stdio/HTTP entry
  shape, env-var interpolation.
- **Behavioural quirks** — supported platforms, features that are silently
  unavailable on certain OSes or scopes (this is the raw material for the
  `Behavior` factory in issue #208).
- **Sources** — canonical vendor docs only.

When a fact is missing from the vendor docs the entry says **undocumented**
instead of guessing.

---

## At-a-glance matrix

Cross-cutting view of the four "behaviour" fields the #208 refactor needs to
encode. `n/a` means the agent has no MCP feature at all.

| Agent | Native skills | Project MCP | MCP format | Supported OS |
|-------|--------------|-------------|------------|--------------|
| amp | yes (SKILL.md) | yes (`.amp/settings.json`) | JSONC, key `amp.mcpServers` | darwin / linux / windows |
| antigravity | yes (SKILL.md) | **no** (global-only) | JSON, key `mcpServers` | darwin / linux¹ / windows |
| augment | yes (in plugin) | yes (`.mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| claude-code | yes (SKILL.md) | yes (`.mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| claude-desktop | **no** (GUI only) | **no** | JSON, key `mcpServers` | darwin / windows only |
| cline | yes (opt-in flag) | no (global only) | JSON, key `mcpServers` | darwin / linux / windows |
| codex | yes (SKILL.md) | yes (trusted-only) | TOML, key `[mcp_servers.*]` | darwin / linux / windows |
| continue | no (rules instead) | yes (`.continue/`) | YAML, key `mcpServers` | darwin / linux / windows |
| cursor | no (rules instead) | yes (`.cursor/mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| gemini-cli | yes (via extension) | yes (`.gemini/settings.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| generic | yes (convention only) | n/a | n/a | darwin / linux / windows |
| github-copilot | no (custom agents) | yes (`.vscode/mcp.json`) | JSON, key **`servers`** | darwin / linux / windows |
| goose | yes (SKILL.md) | no (global only) | YAML, key `extensions` | darwin / linux / windows |
| kilo | yes (SKILL.md) | yes (`.kilocode/mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| kiro-cli | yes (SKILL.md) | yes (`.kiro/settings/mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| opencode | yes (SKILL.md) | yes (`opencode.json`) | JSON, key **`mcp`** | darwin / linux / windows |
| openhands | yes (SKILL.md) | no (CLI config.toml) | TOML, key `[mcp]` | darwin / linux / windows |
| roo | yes (SKILL.md) | yes (`.roo/mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| trae | yes (SKILL.md, v1.3+) | yes (`.trae/mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| warp | yes (SKILL.md) | yes (`.warp/.mcp.json`) | JSON, key `mcpServers` | darwin / linux / windows |
| windsurf | undocumented² | **no** (global-only) | JSON, key `mcpServers` | darwin / linux / windows |
| zencoder | yes (catalog only) | no (single user file) | JSON, key **`zencoder.mcpServers`** | darwin / linux / windows |

¹ Antigravity Linux support is limited to "select distros" in preview.
² Windsurf has a Skills feature but no documented `SKILL.md` disk format —
skills are dashboard-published.

---

## amp

### Skills
- Native `SKILL.md`-based (Anthropic-style).
- Project: `.agents/skills/<name>/SKILL.md` (committable).
- Global: `~/.config/agents/skills/`, `~/.config/amp/skills/`; legacy
  `~/.claude/skills/` and `.claude/skills/` are also read.
- Frontmatter (YAML): `name` (slug) + `description`. Body loads on-demand.
  A skill folder may include `scripts/`, `assets/`, plus an optional
  `mcp.json` registering per-skill MCP servers loaded only when the skill
  activates.
- Search precedence: project → user global → built-in. Extra roots via the
  `amp.skills.path` setting (`:`-separated on Unix, `;` on Windows).

### Plugins / extensions
- TypeScript plugin files dropped in `.amp/plugins/*.ts` (project) or
  `~/.config/amp/plugins/*.ts` (global). Each `.ts` file is the plugin —
  no central manifest.
- Install by copying the file in; no install command is documented.

### MCP config
- Project: `.amp/settings.json` (or `.jsonc`). Project servers require
  explicit user approval before running.
- Global: `~/.config/amp/settings.json` (Linux/macOS); under `%USERPROFILE%`
  on Windows. Global servers do *not* require approval.
- JSON/JSONC. Top-level key: **`amp.mcpServers`**.
- stdio: `command`, `args`, `env`. HTTP: `url`, `headers`.
- Env interpolation: `${VAR_NAME}`.

### Behavioural quirks
- Officially: macOS, Linux, Windows (terminal recommendations on Windows).
- Workspace MCP servers silently disabled until approved.
- Enterprise "managed settings" override user settings.

### Sources
- https://ampcode.com/manual
- https://ampcode.com/news/agent-skills
- https://ampcode.com/news/lazy-load-mcp-with-skills

---

## antigravity

### Skills
- Native `SKILL.md`; agent semantic-matches the prompt against `description`.
- Project: `<workspace>/.agents/skills/<skill>/SKILL.md` (codelab also
  references `.agent/skills` singular — canonical form undocumented).
- Global: `~/.gemini/antigravity/skills/`.
- Frontmatter (YAML): optional `name` (unique-in-scope), required
  `description`. Folder may contain `scripts/`, `references/`, `assets/`.
- Global rules: `~/.gemini/GEMINI.md`. Project rules: `<workspace>/.agents/rules/`.
- Saved workflows: `~/.gemini/antigravity/global_workflows/` and
  `<workspace>/.agents/workflows/`.

### Plugins / extensions
- Extensions exist (e.g. Google Cloud Data Agent Kit) but the generic
  manifest format and on-disk layout are undocumented. Install via the
  in-app MCP Store / extension store.

### MCP config
- Single global file shared with Gemini CLI / Code Companion. Reported path:
  `~/.gemini/settings/mcp_config.json` (Linux/macOS); under `%USERPROFILE%`
  on Windows. Earlier docs cited `~/.gemini/antigravity/mcp_config.json`.
- JSON. Top-level key: `mcpServers`.
- stdio: `command`, `args`, `env`. HTTP: `url`, `headers`.
- Env interpolation: undocumented.

### Behavioural quirks
- Preview product; tied to personal Gmail accounts. Chrome required for the
  web component.
- Supported on macOS, Windows, and select Linux distros.
- **No per-workspace MCP config** (active feature request) — a server set
  for one project is visible in all workspaces.

### Sources
- https://antigravity.google/docs/skills
- https://antigravity.google/docs/mcp
- https://codelabs.developers.google.com/getting-started-google-antigravity
- https://codelabs.developers.google.com/getting-started-with-antigravity-skills

---

## augment

### Skills
- Native, inside the plugin system. Skills live in a `skills/` directory
  within a plugin (alongside `commands/`, `agents/`, `rules/`, `hooks/`).
- Personal/global: `~/.augment/skills/<name>/` (referenced in vendor docs;
  exact schema undocumented).
- Anthropic-style `SKILL.md` (name/description frontmatter), but the precise
  schema is not formalised in public docs.

### Plugins / extensions
- "Auggie plugins", distributed via **marketplaces** (Git repos).
- Plugin manifest: `.augment-plugin/plugin.json` (`.claude-plugin/` also
  accepted for compatibility). Fields: `name`, `description`, `version`,
  `author`, `keywords`.
- Marketplace manifest: `marketplace.json` listing plugins with `name`,
  `description`, `version`, `source`, `category`, `tags`.
- Install: `auggie plugin marketplace add <owner>/<repo>` then
  `auggie plugin install <plugin>@<marketplace>`, or the `/plugins` TUI.
  Marketplaces auto-update on each interactive launch.
- Variable expansion in plugin configs: `${AUGMENT_PLUGIN_ROOT}`,
  `${WORKSPACE_ROOT}`.

### MCP config
- Project: `.mcp.json` at repo root, or embedded in a plugin's `plugin.json`.
- Top-level key: `mcpServers`. stdio: `command`, `args`, `env`. HTTP entries
  documented to exist; schema not fully detailed in vendor docs.
- Global CLI settings: `~/.augment/settings.json`; auth: `~/.augment/session.json`.
- Project slash commands: `.augment/commands/*.md`.

### Behavioural quirks
- Auggie CLI needs Node 22+; ships via npm.
- VS Code and JetBrains extensions are separate products with their own
  config surfaces.
- Plugin marketplaces silently auto-update on each interactive launch.

### Sources
- https://docs.augmentcode.com/cli/plugins
- https://docs.augmentcode.com/cli/overview
- https://www.augmentcode.com/mcp
- https://github.com/augmentcode/auggie

---

## claude-code

### Skills
- Native `SKILL.md`. Each skill is a directory with `SKILL.md` plus optional
  templates, scripts, refs.
- Project: `.claude/skills/<name>/SKILL.md` (auto-discovered from cwd up to
  repo root; also from `--add-dir` directories).
- Personal/global: `~/.claude/skills/<name>/SKILL.md`.
- Plugin skills: `<plugin>/skills/<name>/SKILL.md`, namespaced as
  `plugin-name:skill-name`.
- Frontmatter (YAML): `name` (lowercase/numbers/hyphens, max 64),
  `description` (combined with `when_to_use`, truncated at 1,536 chars),
  `when_to_use`, `argument-hint`, `arguments`, `disable-model-invocation`
  (bool, default `false`), `user-invocable` (bool, default `true`),
  `allowed-tools` (space-list or YAML list), `model`, `effort`,
  `context: fork`, `agent`, `hooks`, `paths`, `shell` (`bash` | `powershell`).
- Precedence: enterprise > personal > project. Live reload of edits; new
  top-level directories require restart.

### Plugins / extensions
- Native "Plugins". Manifest: `.claude-plugin/plugin.json` (JSON). Required:
  `name`, `description`; optional `version`, `author`, `homepage`,
  `repository`, `license`.
- Plugin root directories: `skills/`, `commands/`, `agents/`, `hooks/` (with
  `hooks.json`), `.mcp.json`, `.lsp.json`, `monitors/monitors.json`, `bin/`,
  `settings.json`. They must **not** live inside `.claude-plugin/`.
- Install / enable: `/plugin` interactive command, marketplaces
  (`/plugin install <name>@<marketplace>`), or local testing with
  `claude --plugin-dir <path>` / `--plugin-url <zip-url>`. `/reload-plugins`
  picks up changes mid-session.

### MCP config
- Project: `.mcp.json` at repo root.
- Local + user scopes both live in `~/.claude.json`:
  - local servers under `projects.<path>.mcpServers`;
  - user servers global within the same file.
- Enterprise: `managed-mcp.json` at
  `/Library/Application Support/ClaudeCode/`, `/etc/claude-code/`, or
  `C:\Program Files\ClaudeCode\`.
- JSON. Top-level key: `mcpServers`.
- stdio: `{"type":"stdio","command":"<bin>","args":[...],"env":{...}}` —
  `type` defaults to `stdio` if omitted.
- HTTP: `{"type":"http","url":"...","headers":{...}}`; `streamable-http` is
  an alias for `http`. SSE: `{"type":"sse",...}` (deprecated). Optional
  fields: `oauth` (`clientId`, `callbackPort`, `authServerMetadataUrl`,
  `scopes`), `headersHelper`, `alwaysLoad`.
- Env interpolation in `command`, `args`, `env`, `url`, `headers`:
  `${VAR}` and `${VAR:-default}`. Plugin servers also expand
  `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}`, `${CLAUDE_PROJECT_DIR}`.
- Precedence: local > project > user > plugin > claude.ai connector.
  Reserved server name: `workspace`.

### Behavioural quirks
- Platforms: macOS 10.15+, Ubuntu 20.04+/Debian 10+, Windows 10+ (WSL1/WSL2
  or Git for Windows native).
- `--add-dir` only loads `.claude/skills/`, not other `.claude/` config from
  added dirs.
- `claude mcp add-from-claude-desktop` only works on macOS and WSL.
- `CLAUDE_PROJECT_DIR` lives in the spawned MCP server's environment, *not*
  Claude Code's — `${CLAUDE_PROJECT_DIR}` in a user/project `.mcp.json` needs
  `${CLAUDE_PROJECT_DIR:-.}` to be evaluated at parse time (plugin configs
  are the exception).
- Skill descriptions are truncated under a 1% context-window budget;
  surfaceable via `/doctor`.

### Sources
- https://code.claude.com/docs/en/skills
- https://code.claude.com/docs/en/plugins
- https://code.claude.com/docs/en/mcp
- https://code.claude.com/docs/en/plugins-reference
- https://docs.claude.com/en/docs/claude-code/settings

---

## claude-desktop

### Skills
- **No on-disk `SKILL.md` mechanism.** Skills are managed through the GUI
  (Settings > Capabilities > Skills, Customize > Skills). Custom skills are
  uploaded as ZIP archives containing a folder with a required `SKILL.md`
  at the root; on-disk storage paths are undocumented.
- No project-scope skills (no project/repo concept in the desktop app).
- Global on-disk skills directory: **undocumented** — `~/.claude/skills/` is
  a Claude *Code* feature, not Claude Desktop.
- Frontmatter for uploaded SKILL.md: `name` (max 64), `description`
  (~200 chars, used to decide invocation), optional `dependencies`
  (e.g. `"python>=3.8, pandas>=1.5.0"`).
- Requires code execution to be enabled. Team/Enterprise can provision
  skills org-wide.

### Plugins / extensions
- **Desktop Extensions (MCPB)** — a `.mcpb` file is a ZIP bundling a local
  MCP server plus a `manifest.json`.
- Required manifest keys: `manifest_version` (e.g. `"0.3"`), `name`,
  `version`, `description`, `author`, `server` (`type` = `node` | `python` |
  `binary` | `uv`, `entry_point`, `mcp_config: {command, args, env,
  platform_overrides}`).
- Optional: `display_name`, `icons`, `tools`, `prompts`, `user_config`,
  `compatibility.platforms` (`darwin`, `win32`, `linux`), `keywords`,
  `license`, `repository`. Path substitutions: `${__dirname}`,
  `${user_config.KEY}`, `${HOME}`, `${DESKTOP}`, `${DOCUMENTS}`,
  `${DOWNLOADS}`, `${pathSeparator}` / `${/}`.
- Install: double-click `.mcpb`, drag-drop, or Settings > Extensions >
  Advanced settings > Install Extension. Per-user install; on-disk path
  undocumented.

### MCP config
- Single JSON config, user scope only:
  - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
  - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
  - Linux: **undocumented** (desktop app unsupported on Linux).
- Top-level key: `mcpServers`.
- stdio: `{"command":"<bin>","args":[...],"env":{...}}` — `type` not
  required in documented examples; HTTP/SSE shape for direct config-file
  use is **undocumented** (remote servers are typically added via the
  in-app Connectors UI).
- Env interpolation: undocumented. Windows `${APPDATA}` doesn't expand
  automatically and must be passed explicitly in `env`.
- No project-scope MCP config (single global file).

### Behavioural quirks
- **macOS (darwin) and Windows (win32) only.** Linux is unsupported by the
  desktop app or by MCPB.
- No project-scope skills, no project-scope MCP, no `.claude/` directory
  model.
- MCP failures may not surface in UI — logs at `~/Library/Logs/Claude/mcp*.log`
  (macOS) or `%APPDATA%\Claude\logs\mcp*.log` (Windows).
- Claude Code can import desktop MCP servers via
  `claude mcp add-from-claude-desktop` (macOS / WSL only).

### Sources
- https://modelcontextprotocol.io/quickstart/user
- https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop
- https://claude.com/docs/connectors/building/mcpb
- https://support.claude.com/en/articles/12512180-use-skills-in-claude
- https://support.claude.com/en/articles/12512198-how-to-create-custom-skills

---

## cline

### Skills
- Native, **experimental** — must be enabled at Settings > Features >
  Enable Skills.
- Project: `.cline/skills/` (also `.clinerules/skills/`, `.claude/skills/`).
- Global: `~/.cline/skills/` (POSIX); `C:\Users\<USER>\.cline\skills\` (Windows).
- Frontmatter (YAML): required `name` (kebab-case, matches directory) and
  `description` (≤ 1024 chars). On name collision, global wins.

### Plugins / extensions
- No general-purpose plugin manifest. Extensibility goes through (a) the
  MCP Marketplace (one-click MCP installs from the Extensions panel) and
  (b) an SDK plugin system that registers tools and lifecycle hooks
  programmatically.
- Installed MCP servers are written into `cline_mcp_settings.json`; no
  separate plugin manifest file.

### MCP config
- Global file (VS Code extension `saoudrizwan.claude-dev`):
  - macOS: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
  - Linux: `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
  - Windows: `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`
- CLI variant: `~/.cline/mcp.json`.
- JSON. Top-level key: `mcpServers`. Mirrors Claude Desktop schema.
- stdio: `command`, `args`, `env`, `disabled`, `autoApprove`. HTTP/SSE:
  `url`, `headers`, `disabled`, `autoApprove`.

### Behavioural quirks
- Available on VS Code, VS Code Insiders, Cursor, Windsurf (substitute
  editor name in the globalStorage path).
- Skills require explicit feature toggle — silently inactive otherwise.
- **gaal's registry maps cline to `~/.vscode/settings.json` for MCP — that
  path is incorrect per vendor docs.** Real path is the
  globalStorage file above.
- Windows `.cline/` under `%USERPROFILE%` is separate from globalStorage
  (issue #3983).

### Sources
- https://docs.cline.bot/customization/skills
- https://docs.cline.bot/mcp/configuring-mcp-servers
- https://docs.cline.bot/mcp/mcp-marketplace
- https://cline.bot/blog/cline-3-48-0-skills-and-websearch-make-cline-smarter

---

## codex

### Skills
- Native `SKILL.md`.
- Project: `.agents/skills/` (scanned from cwd up to repo root).
- User: `$HOME/.agents/skills/`. Admin scope: `/etc/codex/skills/`. System
  scope is bundled.
- A skill is a directory with `SKILL.md` plus optional `scripts/`,
  `references/`, `assets/`, `agents/openai.yaml`.
- Frontmatter: required `name` + `description`. Skill list capped at
  ~8,000 characters in context.
- Layered instructions also via `AGENTS.md` / `AGENTS.override.md`; global
  file at `~/.codex/AGENTS.md`.

### Plugins / extensions
- "Codex plugins" via plugin marketplaces.
- Install via `codex plugin marketplace add <github-shorthand|git-url|local-path>`
  and the `/plugins` slash command. Enable/disable with
  `[plugins."name@source"] enabled = false` in `~/.codex/config.toml`.
- Manifest filename/format: **referenced but undocumented** in the public
  reference page.
- Built-in `@plugin-creator` skill scaffolds plugins.

### MCP config
- Global: `~/.codex/config.toml`. Project: `.codex/config.toml`
  (loaded only for *trusted* projects).
- **TOML**, not JSON. Top-level key: `[mcp_servers.<id>]` (table per server).
- stdio: `command`, `args`, `cwd`, `env`, `env_vars`. HTTP: `url`,
  `http_headers`, `env_http_headers`, `bearer_token_env_var`. Common:
  `enabled`, `enabled_tools`, `disabled_tools`, `startup_timeout_sec`,
  `tool_timeout_sec`.
- `env_vars` accepts plain names or `{ name = "X", source = "local" | "remote" }`;
  `source = "remote"` requires remote MCP stdio.

### Behavioural quirks
- Platforms: macOS, Linux, Windows.
- **Project-scope `.codex/config.toml` is silently ignored unless the
  project is marked trusted** — common automation pitfall.
- TOML schema — not interchangeable with Cursor / Windsurf / Copilot.

### Sources
- https://developers.openai.com/codex/config-reference
- https://developers.openai.com/codex/mcp
- https://developers.openai.com/codex/skills
- https://developers.openai.com/codex/guides/agents-md
- https://developers.openai.com/codex/plugins

---

## continue

### Skills
- **No `SKILL.md` mechanism.** Closest equivalents:
  - Rules: `.continue/rules/*.md` with YAML frontmatter (`name`, `globs`,
    `regex`, `alwaysApply`).
  - Prompts: slash commands defined in YAML/Markdown.
- Both have project (`.continue/`) and global (`~/.continue/`) scopes.

### Plugins / extensions
- "Blocks" composed inside Assistants/Agents, plus Continue Hub
  (now "Mission Control").
- Blocks are reusable YAML units (models, rules, prompts, mcpServers, docs,
  data, context) referenced by hub slug (`owner/item-name@version`) or
  defined locally under `.continue/<kind>/`.
- Install: import from the Hub via `uses:` slugs in `config.yaml`, or drop
  files into the appropriate `.continue/<kind>/` subdirectory.
- A `config.ts` file (in `~/.continue/`) is supported for programmatic
  configuration.

### MCP config
- Global: `~/.continue/config.yaml` (legacy `config.json` still loaded if
  present, deprecated).
- Project: workspace `.continue/` directory with subdirectories `models/`,
  `rules/`, `mcpServers/`, etc. MCP entries can be standalone YAML/JSON
  files in `.continue/mcpServers/` or inline under `mcpServers:` in the
  main config.
- YAML preferred (JSON accepted). Top-level key in inline configs:
  `mcpServers`.
- Per entry: `name`, `version`, `schema: v1` (standalone files), plus
  `type` (`stdio` | `sse` | `streamable-http`), `command` + `args` + `env`
  for stdio, or `url` + `headers` for remote. Secrets resolve from `.env`
  at project root or `<workspace>/.continue/.env` via mustache syntax.

### Behavioural quirks
- VS Code and JetBrains IDEs (and CLI).
- MCP tools are usable only in Agent mode, not in chat or autocomplete.
- If both `config.yaml` and `config.json` exist in `~/.continue/`, YAML
  silently wins.
- Workspace-level blocks have known detection issues (#5239, #6905).

### Sources
- https://docs.continue.dev/reference
- https://docs.continue.dev/customize/deep-dives/mcp
- https://docs.continue.dev/customize/deep-dives/configuration
- https://docs.continue.dev/customize/deep-dives/rules
- https://docs.continue.dev/guides/understanding-configs

---

## cursor

### Skills
- **No native `SKILL.md`.** Cursor uses "Rules" instead; `AGENTS.md` is
  also accepted.
- Project rules: `.cursor/rules/` (`.md` or `.mdc`). Subdirectories allowed.
- Global/user rules: stored in Cursor Settings (no on-disk directory
  documented); user-level `AGENTS.md` is undocumented.
- Format: MDC = markdown + YAML frontmatter (`description`, `globs`,
  `alwaysApply`). Community sources mention `priority`; undocumented.

### Plugins / extensions
- VS Code-style extensions (Cursor is a VS Code fork).
- On disk: `~/.cursor/extensions/` referenced by community docs, undocumented
  in vendor docs.
- Manifest: standard VS Code `package.json` (inside the VSIX).
- Install: command-palette "Extensions: Install from VSIX", drag-and-drop,
  or `cursor --install-extension <file.vsix>`. Default registry is Open VSX.

### MCP config
- Project: `.cursor/mcp.json`. Global: `~/.cursor/mcp.json` (project wins
  on collision).
- JSON. Top-level key: `mcpServers`.
- stdio: `{ "command", "args", "env" }`. HTTP: `{ "url" }` (`type` not
  documented for Cursor; headers undocumented for the JSON file but
  configurable in UI).
- Env interpolation: undocumented.

### Behavioural quirks
- Platforms: macOS, Linux, Windows.
- Skills (in the SKILL.md sense) are not native — automation has to
  translate to rules or `AGENTS.md`.
- JSON shape matches Claude Desktop, so configs are mostly portable.

### Sources
- https://cursor.com/docs/cli/mcp
- https://cursor.com/help/customization/extensions

---

## gemini-cli

### Skills
- Native — **only as part of an installed extension**. Skills placed at
  `<extension>/skills/<name>/SKILL.md`. There is no documented standalone
  `~/.gemini/skills/`; skills ride along with extensions.
- Extensions (and their skills) live under `~/.gemini/extensions/<name>/`.
- Anthropic-style `SKILL.md` frontmatter (`name`, `description`).

### Plugins / extensions
- First-class **Gemini CLI Extensions**.
- On disk: `~/.gemini/extensions/<extension>/` (a copy is made at install;
  `gemini extensions update` pulls source changes). Project-level
  extensions directory: undocumented.
- Manifest: `gemini-extension.json` at the extension root. Properties:
  `name` (lowercase), `version`, `description`, `mcpServers`,
  `contextFileName` (defaults to `GEMINI.md`), `excludeTools`, `plan`.
  Extensions can also include `commands/*.toml`, themes, hooks, sub-agents
  and `skills/`.
- Install:
  `gemini extensions install <github-url-or-local-path>` (optional `--ref`,
  `--auto-update`). Enable/disable per-workspace or globally via
  `gemini extensions enable|disable <name>`.

### MCP config
- User: `~/.gemini/settings.json`. Project: `<workspace>/.gemini/settings.json`.
- JSON. Top-level key: `mcpServers`. Sibling `mcp` object holds global
  allow/exclude lists (`mcp.allowed`, `mcp.excluded`, `mcp.serverCommand`).
- stdio: `command`, `args`, `env`, `cwd`, `timeout`, `trust`. HTTP:
  `httpUrl`, `headers`, `timeout`. SSE: `url`, `headers`, `timeout`.
- Env interpolation: `$VAR` / `${VAR}` on all platforms; `%VAR%` on Windows.

### Behavioural quirks
- Platforms: macOS, Linux, Windows.
- Tool-name normalisation replaces non-`[A-Za-z0-9_.\-:]` characters with
  underscores → silent name collisions possible.
- Underscores in server names discouraged (policy engine misinterprets);
  use hyphens.
- MCP connection failures only show a small hint at startup — silent
  failure mode without explicit diagnostic commands.
- `settings.json` edits need a restart to take effect.

### Sources
- https://geminicli.com/docs/tools/mcp-server/
- https://geminicli.com/docs/extensions/
- https://geminicli.com/docs/extensions/reference/
- https://geminicli.com/docs/extensions/writing-extensions/
- https://geminicli.com/docs/cli/tutorials/mcp-setup/

---

## generic

### Skills
- gaal-owned **shared-skills convention**, *not* a vendor product.
- Project: `.agents/skills/`. Global: `~/.agents/skills/`.
- Follows the open AgentSkills SKILL.md convention (YAML frontmatter
  `name` + `description`, plus markdown body and optional bundled
  resources) so any compliant agent can read it.

### Plugins / extensions
- None. This is purely a path convention, not a runtime.

### MCP config
- None. `generic` only governs skills.

### Behavioural quirks
- Activated per-agent by `supports_generic_project` / `supports_generic_global`
  in `internal/core/agent/agents.yaml`; gaal then redirects skill operations
  for that agent to the `.agents/skills` tree instead of the vendor's
  native location.
- Agents that don't opt in fall back to their native skills directory.

### Sources
- No vendor docs — internal gaal convention.

---

## github-copilot

### Skills
- **No native `SKILL.md`.** Closest equivalents:
  - Custom agents: `.agent.md` (project `.github/agents/`,
    user `~/.copilot/agents/`; VS Code also recognises `.claude/agents/`).
  - Prompt files: `.prompt.md` (`.github/prompts/*.prompt.md`).
  - Instructions: `.instructions.md` / `copilot-instructions.md`
    (`.github/copilot-instructions.md` project,
    `$HOME/.copilot/copilot-instructions.md` user).
- `.agent.md` frontmatter: `name`, `description`, `tools`, `model`,
  `agents`, `handoffs`, `user-invocable`, `hooks` (preview).
- Additional custom-agent locations via the `chat.agentFilesLocations`
  setting.

### Plugins / extensions
- VS Code extensions (Copilot ships as the GitHub Copilot Chat extension;
  it can host MCP servers and custom agents).
- Extensions live in VS Code's standard directories (`~/.vscode/extensions/`
  etc.) — not Copilot-specific.
- Manifest: standard VS Code `package.json`.
- Install: VS Code Marketplace or `code --install-extension`.

### MCP config
- Workspace: `.vscode/mcp.json`. User-profile: `mcp.json` inside the VS Code
  profile directory (exact OS path is per VS Code profile docs).
- JSON. **Top-level key: `servers`** (not `mcpServers`). Optional top-level
  `inputs` array for prompted secrets.
- stdio: `{ "type": "stdio", "command", "args", "env" }`.
- HTTP/SSE: `{ "type": "http" | "sse", "url", "headers" }`.
- Interpolation: `${input:variable-id}` (resolved from `inputs`) and
  `${workspaceFolder}`. Optional `sandboxEnabled` / `sandbox` object for
  per-server sandboxing.

### Behavioural quirks
- Platforms: macOS, Linux, Windows (via VS Code).
- **`servers` vs `mcpServers` is the single most common porting mistake** —
  configs copy-pasted from Cursor/Windsurf load silently as nothing.
- Custom agents, prompt files, and instructions are three separate
  mechanisms with different precedence (personal > repo > org).

### Sources
- https://code.visualstudio.com/docs/copilot/reference/mcp-configuration
- https://code.visualstudio.com/docs/copilot/customization/mcp-servers
- https://code.visualstudio.com/docs/copilot/customization/custom-agents
- https://docs.github.com/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot
- https://docs.github.com/copilot/customizing-copilot/using-model-context-protocol/extending-copilot-chat-with-mcp

---

## goose

### Skills
- Native (recent addition). Auto-discovered at startup from
  `~/.config/goose/skills/` (or `~/.claude/skills/` for sharing with Claude
  Desktop). Project: `.goose/skills/` (Goose-specific) and `.agents/skills/`
  (portable). Anthropic-style `SKILL.md` (YAML `name`/`description`).
- Distinct from **recipes** — reusable YAML workflows at `~/.goose/recipes/`;
  extra dirs via `GOOSE_RECIPE_PATH`.

### Plugins / extensions
- Mechanism is called **extensions** (effectively MCP servers + builtins).
  Types: `builtin`, `stdio`, `sse`, `streamable_http`, `platform`.
- Configured directly in the main config file — no separate manifest.
- Install / enable via the interactive wizard `goose configure` ("Add
  Extension") or by editing `config.yaml`. The desktop / CLI also fetches
  extensions from a hosted directory.

### MCP config
- `~/.config/goose/config.yaml` (Linux/macOS);
  `%APPDATA%\Block\goose\config\config.yaml` (Windows).
- No per-project config file.
- **YAML.** Top-level key: `extensions` (map keyed by extension name).
- stdio: `type: stdio`, `cmd`, `args` (list), `enabled`, `timeout`,
  `envs` (inline map), `env_keys` (list of names fetched from OS keyring
  / `secrets.yaml` fallback), `description`, `name`, `bundled`.
- HTTP-style: `type: streamable_http` (or `sse`) with `url`. Headers / env
  interpolation undocumented.

### Behavioural quirks
- Platforms: macOS, Linux, Windows. CLI is cross-platform; the desktop and
  `goose configure` keyring integration are most polished on macOS — on
  Linux/WSL the keyring falls back to a plaintext `secrets.yaml`.
- `env_keys` silently leaves an extension non-functional if the named
  secret is missing from the keyring / fallback.

### Sources
- https://block.github.io/goose/docs/guides/config-file/
- https://block.github.io/goose/docs/getting-started/using-extensions/
- https://block.github.io/goose/docs/mcp/developer-mcp/
- https://block.github.io/goose/docs/guides/recipes/

---

## kilo

### Skills
- Native; follows the open Agent Skills spec, so other agents' skill
  folders also load.
- Project: `.kilo/skills/` (also `.claude/skills/`, `.agents/skills/` for
  cross-compat). gaal registry uses `.kilocode/skills/` — verify against
  current upstream rename.
- Global: `~/.kilo/skills/` (POSIX), `C:\Users\<USER>\.kilo\skills\` (Windows).
- Frontmatter (YAML): required `name` (≤ 64 chars, lowercase + hyphens) and
  `description` (≤ 1024 chars); optional `license`, `compatibility`,
  `metadata`.

### Plugins / extensions
- **Kilo Marketplace** (curated Skills, Modes, MCP servers).
- Custom Modes: legacy `.kilocodemodes` or `custom_modes.yaml`
  (auto-migrated on startup) for the VS Code extension; new CLI/v7+ uses
  `kilo.jsonc`.
- One-click marketplace install, or direct file edits.

### MCP config
- **Two coexisting formats:**
  - **Legacy VS Code extension** (`kilocode.kilo-code`): global
    `mcp_settings.json` in `globalStorage/.../settings/`:
    - macOS: `~/Library/Application Support/Code/User/globalStorage/kilocode.kilo-code/settings/mcp_settings.json`
    - Linux: `~/.config/Code/User/globalStorage/kilocode.kilo-code/settings/mcp_settings.json`
    - Windows: `%APPDATA%\Code\User\globalStorage\kilocode.kilo-code\settings\mcp_settings.json`
    - VS Code Server: `~/.vscode-server/data/User/globalStorage/kilocode.kilo-code/...`
    - Project: `.kilocode/mcp.json`. JSON. Top-level `mcpServers`.
      stdio: `command`/`args`/`env`. HTTP: `url`/`headers`.
  - **New v7.0.33+**: global `~/.config/kilo/kilo.jsonc`; project
    `kilo.jsonc` at root or `.kilo/kilo.jsonc`. **JSONC.** Top-level key:
    **`mcp`** (not `mcpServers`). stdio entries: `"type": "local"`,
    `command` (array), `environment`, `enabled`, `timeout`. HTTP entries:
    `"type": "remote"`, `url`, `headers`, `enabled`, `timeout`.

### Behavioural quirks
- VS Code, VS Code Insiders, Cursor, VSCodium, plus standalone CLI; remote
  sessions use the VS Code Server globalStorage path.
- `kilo-code.customStoragePath` VS Code setting can relocate globalStorage.
- After upgrading to v7.0.33+, old `mcp_settings.json` is silently ignored
  (issue #6481) — silent-failure mode.
- stderr from MCP child processes can be treated as failure, causing
  restart loops (issue #6045).
- **gaal registry tracks `.kilocode/skills/` and
  `~/.kilocode/mcp.json` — the vendor has been migrating to `kilo` paths.
  Plan a registry refresh.**

### Sources
- https://kilo.ai/docs/customize/skills
- https://kilo.ai/docs/features/mcp/using-mcp-in-kilo-code
- https://github.com/Kilo-Org/kilocode-legacy/blob/main/docs/file-locations.md
- https://kilo.ai/docs/customize/custom-modes

---

## kiro-cli

### Skills
- Native; open AgentSkills standard.
- Project: `.kiro/skills/<name>/SKILL.md`. Global: `~/.kiro/skills/<name>/SKILL.md`.
  Workspace wins on conflict.
- Frontmatter (YAML): required `name` (lowercase + hyphens, ≤ 64 chars,
  must match folder) and `description` (≤ 1024 chars). Optional `license`,
  `compatibility`, `metadata`. Custom agents reference skills via
  `skill://` URIs with globs.

### Plugins / extensions
- "Steering" + "Hooks":
  - Steering files: `.kiro/steering/*.md` (project),
    `~/.kiro/steering/*.md` (global) — e.g. `product.md`, `tech.md`,
    `structure.md`.
  - Hooks: `*.kiro.hook` files surfaced via the IDE's Agent Hooks panel for
    lifecycle automation.
- Skills are imported via the Agent Steering & Skills panel from GitHub URL
  or local path; imported skills are copied into the user's skills dir.

### MCP config
- Project: `.kiro/settings/mcp.json`. Global: `~/.kiro/settings/mcp.json`.
- JSON. Top-level: `mcpServers`.
- stdio: `{ command, args, env?, disabled?, autoApprove?, disabledTools? }`.
  `autoApprove` may be `"*"` or a tool-name array.
- HTTP: `{ url, headers?, env?, disabled?, autoApprove?, disabledTools? }`.
- Env interpolation: `${VARIABLE_NAME}`.
- Precedence: Agent Config > workspace `mcp.json` > global `mcp.json`.

### Behavioural quirks
- Platforms: macOS, Linux, Windows.
- Known bug: workspace-level skills `allowed-tools` frontmatter ignored
  (kirodotdev/Kiro#6055) — silent-failure mode.

### Sources
- https://kiro.dev/docs/mcp/configuration/
- https://kiro.dev/docs/skills/
- https://kiro.dev/docs/steering/
- https://kiro.dev/docs/cli/mcp/configuration/
- https://kiro.dev/docs/cli/custom-agents/configuration-reference/

---

## opencode

### Skills
- Native. Project: `.opencode/skills/`. Global: `~/.config/opencode/skills/`.
- Same plural-directory family as `agents/`, `commands/`, `modes/`,
  `plugins/`, `tools/`, `themes/`.
- Exact `SKILL.md` frontmatter schema not enumerated in vendor docs (defaults
  to the Anthropic-style convention).

### Plugins / extensions
- Plugins: custom plugin files in `.opencode/plugins/` (project) or
  `~/.config/opencode/plugins/` (global). Also loadable from npm via the
  `plugin` key in `opencode.json` (`"plugin": ["pkg-name", "@scope/pkg"]`).
  No separate manifest — the plugin source itself is the artifact.
- Custom agents additionally as Markdown files in `.opencode/agents/` or
  `~/.config/opencode/agents/`.

### MCP config
- File: `opencode.json` / `opencode.jsonc` (JSON schema at
  `https://opencode.ai/config.json`).
- Locations (precedence low → high): remote `.well-known/opencode`, global
  config, `OPENCODE_CONFIG` env path, project `opencode.json`, `.opencode/`
  directories, `OPENCODE_CONFIG_CONTENT` env, managed files, macOS managed
  preferences.
- Global: `~/.config/opencode/opencode.json` (POSIX);
  `%APPDATA%\opencode\opencode.json` (Windows). System-managed:
  `/Library/Application Support/opencode/`, `/etc/opencode/`,
  `%ProgramData%\opencode\`.
- **Top-level key: `mcp`** (not `mcpServers`). Per-server:
  - `type: "local"` with `command` (array) and optional `environment`,
    `enabled`, `timeout`;
  - or `type: "remote"` with `url`, `headers`, `oauth`, `enabled`, `timeout`.
- Env interpolation: `{env:VAR_NAME}` (not `${VAR}`).

### Behavioural quirks
- Cross-platform. macOS additionally honours OS-level managed preferences
  as highest-priority override.
- Two MCP type vocabularies in docs: `local`/`remote` (canonical) vs
  `stdio`/`http` (example snippets). Both appear; `local`/`remote` is
  authoritative.

### Sources
- https://opencode.ai/docs/config/
- https://opencode.ai/docs/mcp-servers/
- https://opencode.ai/docs/agents/
- https://opencode.ai/docs/cli/

---

## openhands

### Skills
- Native `SKILL.md`. Follows the open AgentSkills standard with
  progressive disclosure (metadata always loaded, body on trigger).
- Project: `.agents/skills/` (recommended); `.openhands/skills/` and
  `.openhands/microagents/` accepted but deprecated.
- Global: `~/.agents/skills/`.
- Per-skill directory with required `SKILL.md` plus optional `scripts/`,
  `references/`, `assets/`. YAML frontmatter `name` + `description`;
  description required for keyword-triggered skills.

### Plugins / extensions
- No formal plugin manifest. Extension happens via skills + MCP. Public
  registry at github.com/OpenHands/skills.

### MCP config
- CLI: `config.toml` (loaded from the repo base; local state under
  `~/.openhands/`).
- Web UI: persists to `settings.json`. Dual-config system is tracked as
  tech debt (issue #9531).
- **TOML.** Under `[mcp]`.
- Top-level arrays: `sse_servers`, `shttp_servers`, `stdio_servers`
  (arrays of inline tables).
- stdio: `{ name, command, args, env }`. shttp: bare URL string OR
  `{ url, api_key, timeout }` (timeout 1–3600 s).
- Env interpolation: undocumented.

### Behavioural quirks
- Platforms: macOS, Linux, Windows (mostly via Docker runtime).
- CLI vs Web UI config drift means a server visible in UI may not exist in
  `config.toml` and vice versa.

### Sources
- https://docs.openhands.dev/openhands/usage/settings/mcp-settings
- https://docs.openhands.dev/overview/skills
- https://github.com/OpenHands/OpenHands/blob/main/config.template.toml
- https://github.com/OpenHands/OpenHands/issues/9531

---

## roo

### Skills
- Native; auto-discovered at startup and via file watchers.
- Project: `.roo/skills/<name>/SKILL.md` (Roo-preferred) or
  `.agents/skills/<name>/SKILL.md` (cross-agent).
- Global: `~/.roo/skills/<name>/SKILL.md` or `~/.agents/skills/<name>/SKILL.md`.
- Frontmatter (YAML): required `name` (1–64 chars, lowercase + hyphens)
  and `description` (1–1024 chars).

### Plugins / extensions
- Roo Code Marketplace + Custom Modes.
- Custom Modes: `.roomodes` (project, YAML or JSON; auto-detected) or a
  global custom-modes file in the extension's globalStorage
  (`settings/custom_modes.yaml`).
- Marketplace installs MCP servers and Modes; no separate plugin manifest
  beyond `.roomodes` and `mcp_settings.json`.

### MCP config
- Global file (extension `rooveterinaryinc.roo-cline`):
  - macOS: `~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/mcp_settings.json`
  - Linux: `~/.config/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/mcp_settings.json`
  - Windows: `%APPDATA%\Code\User\globalStorage\rooveterinaryinc.roo-cline\settings\mcp_settings.json`
- Project: `.roo/mcp.json` (project overrides global on name collision).
- JSON. Top-level: `mcpServers`.
- stdio: `command`, `args` (supports `${env:VAR}` interpolation), `cwd`,
  `env`, plus `alwaysAllow`, `disabled`, `timeout`, `watchPaths`,
  `disabledTools`.
- HTTP: `type: "streamable-http"`, `url`, `headers`. Legacy SSE:
  `type: "sse"`, `url`, `headers`.

### Behavioural quirks
- VS Code, VS Code Insiders, Cursor, Windsurf, VSCodium (substitute editor
  in globalStorage path).
- **gaal's registry maps roo to `~/.vscode/settings.json` for MCP —
  vendor docs point to the globalStorage file above.** Plan a registry
  refresh.
- Windows-only `cmd` wrappers for stdio commands in vendor examples;
  macOS/Linux call binaries directly.

### Sources
- https://docs.roocode.com/features/skills
- https://docs.roocode.com/features/mcp/using-mcp-in-roo
- https://docs.roocode.com/features/mcp/server-transports
- https://docs.roocode.com/features/custom-modes
- https://docs.roocode.com/features/marketplace

---

## trae

### Skills
- Native (v1.3.0+). Step-by-step procedural playbooks loaded on relevance.
- Project: `.trae/skills/`. Global skills directory: undocumented (skills
  documented as project-scoped).
- Frontmatter (YAML): `name`, `description`. Vendor docs note current
  limited interoperability with the broader SKILL.md ecosystem (e.g.
  Superpowers unsupported — issue #2253).

### Plugins / extensions
- VS Code-style extension surface (Trae is a VS Code fork). Agents
  themselves are user-defined and configurable via the AI Management panel.
- "Rules" act as persistent behavioural guidance —
  `.trae/project_rules.md` and `.trae/user_rules.md` (Markdown).

### MCP config
- Project: `.trae/mcp.json` at repo root.
- **Global file: `~/.cursor/mcp.json`** — Trae reads Cursor's global file
  rather than defining its own.
- JSON. Top-level `mcpServers`.
- stdio: `{ command, args, env }`. HTTP: `{ url }` (and optional headers via
  the in-app config dialog).
- Env interpolation: undocumented.

### Behavioural quirks
- macOS, Windows (10/11), Linux as of v1.3.0; native Windows ARM64 pending.
- UI-first configuration; the disk file format is JSON-equivalent to Cursor.
- **gaal registry uses `~/.trae/mcp.json` as the global path — vendor docs
  reuse Cursor's `~/.cursor/mcp.json`.** Plan a registry refresh.

### Sources
- https://docs.trae.ai/ide/add-mcp-servers
- https://docs.trae.ai/ide/skills
- https://docs.trae.ai/ide/rules
- https://traeide.com/news/6

---

## warp

### Skills
- Native — documented agent capability.
- Project / Global on-disk paths: undocumented. Skills are stored in Warp
  Drive (cloud) and surfaced through the UI.
- Frontmatter (YAML): `name`, `description`.

### Plugins / extensions
- **No third-party plugin/extension system.** Team has acknowledged the
  request (warpdotdev/warp#219, #435) but ships only first-party integrations
  (Docker, Raycast, VS Code).
- Customisation: themes (`~/.warp/themes/`),
  keybindings (`~/.warp/keybindings.yaml`),
  YAML workflows (`~/.warp/workflows/`),
  launch configurations (`~/.warp/launch_configurations/`).

### MCP config
- Project: `.warp/.mcp.json` at repo root.
- Global: `~/.warp/.mcp.json` (also auto-detects configs from other agents
  like Claude Code / Codex).
- JSON. Top-level: `mcpServers`.
- stdio: `{ command, args, env?, working_directory? }`. HTTP/SSE:
  `{ url, headers? }`.
- Env interpolation: `${VAR}` in examples, not formally specified; sensitive
  `env` values auto-scrubbed when sharing.

### Behavioural quirks
- macOS, Linux, Windows.
- Project rules: `AGENTS.md` (legacy `WARP.md`; filename **must be
  ALL-CAPS**). Global rules in Warp Drive (cloud), no disk path.
- Project-scoped MCP servers require explicit user approval before spawning.
- **gaal registry uses `~/.warp/launch_configurations/mcp.json` — vendor
  docs point to `~/.warp/.mcp.json`.** Plan a registry refresh.

### Sources
- https://docs.warp.dev/agent-platform/capabilities/mcp/
- https://docs.warp.dev/agent-platform/capabilities/rules/
- https://docs.warp.dev/agent-platform/capabilities/skills/
- https://docs.warp.dev/guides/external-tools/using-mcp-servers-with-warp/
- https://docs.warp.dev/terminal/entry/yaml-workflows/

---

## windsurf

### Skills
- Skills are a Windsurf feature (multi-step procedures with scripts /
  templates), but **no vendor-documented `SKILL.md` frontmatter on disk** —
  skills are versioned and published via the Windsurf dashboard.
- Project rules: `.windsurf/rules/*.md` (walked up to git root).
- Global rules: `~/.codeium/windsurf/memories/global_rules.md` (single
  file, always on, no frontmatter).
- Workspace rules support YAML frontmatter with `trigger` (`always_on` |
  `model_decision` | `glob` | `manual`) and `globs`. Limits: workspace
  12,000 chars, global 6,000 chars.

### Plugins / extensions
- VS Code-style extensions (VS Code OSS fork).
- Extensions directory: undocumented in vendor docs.
- Manifest: standard VS Code `package.json`.
- Install via the Extensions panel, sourced from Open VSX (default) at
  `marketplace.windsurf.com`. Microsoft Marketplace is *not* used by
  default.

### MCP config
- Global: `~/.codeium/windsurf/mcp_config.json` (POSIX);
  `%USERPROFILE%\.codeium\windsurf\mcp_config.json` (Windows).
- **Project-scope MCP config is undocumented** (not supported per vendor
  docs).
- JSON. Top-level: `mcpServers`.
- stdio: `command`, `args`, `env`. HTTP/SSE: `serverUrl` or `url`, plus
  `headers`.
- Env interpolation: `${env:VAR_NAME}` and `${file:/path/to/file}` (tilde
  paths supported). Allowed in `command`, `args`, `env`, `serverUrl`,
  `url`, `headers`.

### Behavioural quirks
- macOS (≠ 10.15), Windows 10+ (x64/arm64), Linux (glibc ≥ 2.28).
- **No project-scope MCP** — automation that writes `.windsurf/mcp.json`
  will fail silently.
- Skills cannot be deployed by file alone; require dashboard publish.
- **gaal registry uses `~/.codeium/windsurf/mcp_settings.json` — vendor
  docs use `mcp_config.json`.** Plan a registry refresh.

### Sources
- https://docs.windsurf.com/windsurf/cascade/mcp
- https://docs.windsurf.com/windsurf/cascade/memories
- https://docs.windsurf.com/windsurf/cascade/workflows
- https://windsurf.com/download/editor

---

## zencoder

### Skills
- Native — "Skills" are documented as reusable instruction packs alongside
  Zencoder's named agents (Coding Agent, Unit Testing Agent, Ask Agent, …).
- Project / Global on-disk dirs: undocumented; managed through the Agent
  Tools menu inside the IDE.
- `SKILL.md` schema: undocumented externally. Zencoder treats agents +
  skills as catalog items rather than on-disk files.

### Plugins / extensions
- Distributed as IDE extensions only: VS Code Marketplace + JetBrains
  plugin. **No standalone CLI.**
- Install: marketplace install + IDE restart.

### MCP config
- `~/.zencoder/settings.json` (single shared settings file across IDEs;
  surfaced via "Edit in settings.json" UI button). Project-scoped MCP:
  undocumented.
- JSON. **Top-level key: `zencoder.mcpServers`** (namespaced).
- stdio: `{ command, args?, env? }`. HTTP entry: undocumented in canonical
  docs (marketplace listings show stdio only).
- Env interpolation: undocumented; vendor examples use `<YOUR_TOKEN>`
  placeholders.

### Behavioural quirks
- macOS, Linux, Windows (via host IDE).
- Namespaced key means the file is **not drop-in compatible** with other
  agents' bare `mcpServers` JSON.
- JetBrains-specific UI quirks documented (separate "JetBrains Screen
  Issues" page).
- **gaal registry uses `~/.zencoder/mcp.json` — vendor docs point to
  `~/.zencoder/settings.json` with the `zencoder.mcpServers` key.** Plan a
  registry refresh.

### Sources
- https://docs.zencoder.ai/features/integrations-and-mcp
- https://docs.zencoder.ai/features/integration
- https://plugins.jetbrains.com/plugin/24782-zencoder-your-mindful-ai-coding-agent
- https://marketplace.visualstudio.com/items?itemName=ZencoderAI.zencoder

---

## Implications for issue #208

The research surfaces a small, well-bounded set of behavioural axes that the
`AgentBehavior` factory needs to encode. Listing them here so the refactor
plan is grounded in real cases, not hypothetical ones:

1. **`supports_skills` (bool)** — false today only for `claude-desktop`
   (GUI app). Drives the existing
   `warnSkillsTargetingClaudeDesktop` check.

2. **`supports_mcp_project` (bool)** — false today for `antigravity`,
   `claude-desktop`, `cline`, `goose`, `windsurf`, `zencoder` (and
   uncertain for `openhands` whose project file is opt-in). Warrants a
   `WarnMCPProjectUnsupported` warning when a `global: false` entry
   targets one of them.

3. **`supported_platforms` ([]string)** — only `claude-desktop` carries a
   non-trivial restriction (`darwin`, `windows` only). Replaces the inline
   `runtime.GOOS == "linux"` check in `mcp/manager.go`.

4. **Open questions surfaced by the research that are *out of scope* for
   #208 but should be filed:**
   - `cline`, `roo`, `windsurf`, `trae`, `warp`, `kilo`, `zencoder` —
     gaal's `agents.yaml` paths drift from current vendor docs. File a
     registry-refresh issue.
   - **MCP top-level key is not always `mcpServers`** — `github-copilot`
     uses `servers`, `opencode` uses `mcp`, `amp` uses `amp.mcpServers`,
     `zencoder` uses `zencoder.mcpServers`. The codec layer in
     `internal/mcp/codec.go` assumes a single key. This is a separate
     correctness bug, not behavioural validation. File as its own issue.
   - **Non-JSON MCP formats** beyond JSON/TOML: `goose` (YAML), `continue`
     (YAML preferred). The codec switch in `internal/mcp/codec.go` only
     dispatches on extension — verify YAML targets work end-to-end.

The behavioural validator can land for #208 without resolving (4); those
are independent follow-ups.
