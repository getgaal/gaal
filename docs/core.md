# Core System — gaal

> **Source of truth** for everything related to the `internal/core` layer: VCS backends, the VCS factory, type detection, the agent registry, path helpers, and security constraints. All other documentation pages reference this file rather than duplicating these details.

---

## Overview — Two Pillars

The core layer provides the two domain primitives that the rest of gaal builds on:

| Pillar | Responsibility | Key package |
|--------|---------------|-------------|
| **VCS** | Abstract interface over version-control backends; factory functions; auto-detection; subprocess and pure-Go implementations | `internal/core/vcs` |
| **Agent Registry** | In-memory, read-only map of coding-agent file-system layouts; built-in definitions embedded at build time; user-extensible without recompilation | `internal/core/agent` |

Both pillars are **consumed only through their public API** — no other package imports unexported helpers or reaches into the structs directly.

---

## VCS Sub-package (`internal/core/vcs`)

### The `VCS` Interface

Every backend must satisfy the single interface declared in `vcs.go`:

```go
type VCS interface {
    Clone(ctx context.Context, url, path, version string) error
    Update(ctx context.Context, path, version string) error
    IsCloned(path string) bool
    CurrentVersion(ctx context.Context, path string) (string, error)
    HasChanges(ctx context.Context, path string) (bool, error)
}
```

| Method | Contract |
|--------|---------|
| `Clone` | Fetch from `url` into `path`; check out `version` (empty = default branch) |
| `Update` | Fetch latest and check out `version` (empty = current tracking branch / HEAD) |
| `IsCloned` | Non-destructive probe: returns `true` iff `path` contains a valid working copy |
| `CurrentVersion` | Human-readable state: tag > branch > short hash / revision |
| `HasChanges` | `true` iff tracked files are modified; untracked files are **ignored** |

Compile-time assertions in `vcs.go` guarantee all five backends keep satisfying the interface — a missing method fails the build with a clear error:

```go
var (
    _ VCS = (*VcsGit)(nil)
    _ VCS = (*VcsMercurial)(nil)
    _ VCS = (*VcsSVN)(nil)
    _ VCS = (*VcsBazaar)(nil)
    _ VCS = (*VcsArchive)(nil)
)
```

### Backends

| Struct | Config type | Implementation strategy | External dependency |
|--------|------------|------------------------|---------------------|
| `VcsGit` | `git` | Pure Go (`go-git/go-git/v5`) | none |
| `VcsMercurial` | `hg` | Subprocess (`hg`) | `hg` binary |
| `VcsSVN` | `svn` | Subprocess (`svn`, `svnversion`) | `svn` / `svnversion` binary |
| `VcsBazaar` | `bzr` | Subprocess (`bzr`) | `bzr` binary |
| `VcsArchive` | `tar`, `zip` | HTTP fetch + stdlib extraction | none |

**Subprocess backends rule:** every method that spawns a subprocess must call `requireBinary(name)` as its **first statement**. This surfaces a clear installation error instead of a cryptic exec failure.

#### `VcsGit` — shallow mode

`VcsGit` carries a single flag:

```go
type VcsGit struct {
    Shallow bool
}
```

When `Shallow` is `true` (created by `NewShallow("git")`):
- `Clone` uses `depth=1` — no history is fetched.
- `Update` hard-resets to `origin/HEAD` (tries `HEAD`, `main`, `master` in order) rather than doing a normal pull — safe after force-pushes and history rewrites.

This mode is used by `internal/skill` for skill caches that never need history.

#### `VcsArchive` — HTTP archives

`VcsArchive` fetches a `.tar.gz` / `.tgz` or `.zip` from a URL and extracts it. The `version` field is treated as an optional **strip-prefix**: a sub-directory name to skip when extracting (mirrors `vcstool` behaviour).

`Update` is a **no-op**: archives have no incremental update semantics. The caller (skill manager) must call `Clone` again to refresh.

### Factory Functions

```go
func New(vcsType string) (VCS, error)
func NewShallow(vcsType string) (VCS, error)
```

`vcsType` values: `"git"`, `"hg"`, `"svn"`, `"bzr"`, `"tar"`, `"zip"`.

`NewShallow` returns `&VcsGit{Shallow: true}` for `"git"` and delegates to `New` for every other type (shallow has no meaning for non-git backends).

Both return `error` for unknown types — callers must handle it.

### Type Detection (`detect.go`)

```go
func DetectType(source string) string
```

`DetectType` is the single entry point for inferring the VCS type from a source URL or local path. It should be called **once per repository** and the result passed to `New` or `NewShallow`.

**Decision tree:**

```
DetectType(source)
  ├─ isLocalFS(source)?
  │     └─ detectLocal(dir)
  │           ├─ .git  → "git"
  │           ├─ .hg   → "hg"
  │           ├─ .svn  → "svn"
  │           ├─ .bzr  → "bzr"
  │           └─ (none found) → "git"   ← default for new dirs
  └─ detectRemote(url)
        ├─ .tar.gz / .tgz → "tar"
        ├─ .zip           → "zip"
        └─ (else)         → "git"
```

`isLocalFS` recognises: `filepath.IsAbs`, Windows drive-letter paths (`C:\`, `C:/`), and the prefixes `/`, `./`, `.\`, `../`, `..\`, `~/`, `~\`. Anything else is treated as a remote URL.

### Utilities (`util.go`)

| Function | Description |
|----------|-------------|
| `requireBinary(name string) error` | Returns a descriptive error if `name` is absent from `PATH`; must be the first call in every subprocess method |
| `cmdOutput(ctx, dir, name string, args ...string) (string, error)` | Runs a command in `dir` and returns captured stdout; respects context cancellation |
| `shortPath(p string) string` | Returns the last two path components for log messages |

### Module Tree

```
internal/core/vcs/
├── vcs.go      — VCS interface, compile-time assertions, New(), NewShallow()
├── git.go      — VcsGit  (go-git, pure Go; shallow mode)
├── hg.go       — VcsMercurial  (subprocess: hg)
├── svn.go      — VcsSVN        (subprocess: svn / svnversion)
├── bzr.go      — VcsBazaar     (subprocess: bzr)
├── archive.go  — VcsArchive    (HTTP fetch + stdlib tar/zip extraction)
├── detect.go   — DetectType(), isLocalFS(), detectLocal(), detectRemote()
└── util.go     — requireBinary(), cmdOutput(), shortPath()
```

---

## Agent Registry Sub-package (`internal/core/agent`)

### Purpose

The agent registry maps a coding-agent **identifier** (e.g. `"claude-code"`, `"github-copilot"`) to its file-system layout (`Info`). `internal/skill` uses it to know where to install skills; `internal/engine/ops` uses it for audit and doctor operations.

The registry is **read-only at runtime**: it is populated once at package initialisation and never mutated.

### `Info` — the agent layout descriptor

```go
type Info struct {
    ProjectSkillsDir       string
    GlobalSkillsDir        string
    ProjectMCPConfigFile   string

    ProjectSkillsSearch []string
    GlobalSkillsSearch  []string
    PmSkillsSearch      []string

    SupportsGenericProject bool
    SupportsGenericGlobal  bool
}
```

| Field | Managed by | Description |
|-------|-----------|-------------|
| `ProjectSkillsDir` | `gaal sync` | Relative path from project root where skills are installed |
| `GlobalSkillsDir` | `gaal sync` | Home-relative (`~/`) path where global skills are installed |
| `ProjectMCPConfigFile` | `gaal sync` | Home-relative (`~/`) path to the agent's MCP config; empty when unsupported |
| `ProjectSkillsSearch` | `gaal audit` | Project-relative dirs scanned 1 level deep; falls back to `ProjectSkillsDir` when empty |
| `GlobalSkillsSearch` | `gaal audit` | Home-relative (`~/`) dirs scanned 1 level deep; falls back to `GlobalSkillsDir` when empty |
| `PmSkillsSearch` | `gaal audit` | Home-relative (`~/`) dirs from the agent's package manager, scanned **recursively** |
| `SupportsGenericProject` | sync + audit | When `true`, project skills are redirected to `generic`'s `ProjectSkillsDir` |
| `SupportsGenericGlobal` | sync + audit | When `true`, global skills are redirected to `generic`'s `GlobalSkillsDir` |

### Built-in Registry (`agents.yaml`)

The file [`internal/core/agent/agents.yaml`](../internal/core/agent/agents.yaml) is **embedded at build time** via `//go:embed agents.yaml`. It is the single source of truth for all built-in agent definitions. The package panics at startup if the embedded file is missing or invalid — this is intentional: a broken embed means a broken build.

The YAML shape:

```yaml
agents:
  <name>:
    project_skills_dir:       <relative path or "">
    global_skills_dir:        <~/ prefixed path or "">
    project_mcp_config_file:  <~/ prefixed path or "">
    project_skills_search:    [<relative paths>]
    global_skills_search:     [<~/ prefixed paths>]
    pm_skills_search:         [<~/ prefixed paths>]
    supports_generic_project: <bool>
    supports_generic_global:  <bool>
```

YAML anchors (`&` / `*`) are used to share common path literals and search lists across agents — they are resolved by the YAML decoder before `loadInto` processes the struct.

### User Extension

Users can add custom agents (or override built-in ones) by creating:

| OS | File path |
|----|-----------|
| Linux | `$XDG_CONFIG_HOME/gaal/agents.yaml` (defaults to `~/.config/gaal/agents.yaml`) |
| macOS | `$XDG_CONFIG_HOME/gaal/agents.yaml` (defaults to `~/.config/gaal/agents.yaml`) |
| Windows | `%AppData%\gaal\agents.yaml` |

Rules:
- A **missing file** is silently skipped (normal first-run state).
- A **parse error** is logged as a warning and the file is entirely skipped; the built-in registry remains intact.
- User entries **can override built-in agents** (`loadInto` is called with `allowOverride: true` for the user file).
- Built-in entries **cannot be overridden** by themselves — the first load uses `allowOverride: false`.

### Initialisation Flow

```
package init()
  1. Read embedded agents.yaml (panic on failure — broken embed = broken build)
  2. loadInto(builtins, registry, allowOverride=false)
  3. userAgentsPath()  → OS-specific path
  4. os.ReadFile(userPath)
       ├─ ErrNotExist  → silently skip
       ├─ other error  → slog.Warn, skip
       └─ ok           → loadInto(userData, registry, allowOverride=true)
                             └─ parse/validate error → slog.Warn, skip
```

### Security Constraints (`validateEntry`)

All agent path fields are validated before being accepted into the registry.
These rules prevent path-traversal attacks from malicious `agents.yaml` files:

| Field | Constraint |
|-------|-----------|
| `project_skills_dir` | Must be **relative** (no `/`, `\`, drive-letter prefix), no `..` segments — OR empty when `supports_generic_project: true` |
| `global_skills_dir` | Must start with `~/` or `~\` — OR empty when `supports_generic_global: true` |
| `project_mcp_config_file` | Must be empty OR start with `~/` / `~\` |
| `project_skills_search[]` | Each entry must be **relative** and contain no `..` segments |
| `global_skills_search[]` | Each entry must start with `~/` or `~\` |
| `pm_skills_search[]` | Each entry must start with `~/` or `~\` |

`isAbsPath(p)` rejects `/foo` and `\foo` even on Windows (where `filepath.IsAbs` alone would miss Unix-style absolute paths).

`containsDotDot(p)` splits on `/` (after `filepath.ToSlash`) and checks each segment individually — rejecting `a/../b` as well as pure `..`.

### Generic Convention

The `generic` agent owns two **shared** skill installation paths:
- Project: `.agents/skills`
- Global: `~/.agents/skills`

Any agent that sets `supports_generic_project: true` or `supports_generic_global: true` has its sync target **silently redirected** to `generic`'s corresponding path via `SkillDir()`. This allows agents like `cline`, `cursor`, and `codex` to read skills from the vendor-neutral `.agents/skills` tree without requiring a separate installation per agent.

### Path Helpers

All callers must use these helpers — never access `Info` fields directly for path resolution:

| Function | Description |
|----------|-------------|
| `SkillDir(name, global, home string) (string, bool)` | Returns the resolved install path for the given agent and scope; applies the generic-convention redirect; expands `~` |
| `ProjectMCPConfigPath(name, home string) (string, bool)` | Returns the absolute MCP config path (home-expanded); `("", false)` when not set |
| `ExpandedProjectSkillsSearch(name string) []string` | Returns project-relative scan dirs; falls back to `ProjectSkillsDir` |
| `ExpandedGlobalSkillsSearch(name, home string) []string` | Returns home-expanded global scan dirs; falls back to `GlobalSkillsDir` |
| `ExpandedPmSkillsSearch(name, home string) []string` | Returns home-expanded package-manager scan dirs |
| `ExpandHome(p, home string) string` | Expands leading `~/` or `~\` to `home`; cross-platform (POSIX + Windows) |

### Platform Awareness (`userAgentsPath`)

The user agents file path follows the same XDG-aware logic as the config system:

| OS | Path |
|----|------|
| Linux | `os.UserConfigDir()` honours `$XDG_CONFIG_HOME` → `$XDG_CONFIG_HOME/gaal/agents.yaml` |
| macOS | Overrides `os.UserConfigDir()`: prefers `$XDG_CONFIG_HOME`, falls back to `~/.config/gaal/agents.yaml` |
| Windows | `os.UserConfigDir()` → `%AppData%\gaal\agents.yaml` |

> macOS intentionally diverges from `os.UserConfigDir()` (which returns `~/Library/Application Support`). Do **not** call `os.UserConfigDir()` for agent-path resolution — use `userAgentsPath()`.

### Public API

| Symbol | Description |
|--------|-------------|
| `Info` | Agent layout descriptor (all path fields) |
| `Entry` | `{ Name string; Info Info }` — pair for iteration |
| `Names() []string` | All registered agent identifiers (unsorted) |
| `List() []Entry` | All agents sorted by name |
| `Lookup(name string) (Info, bool)` | Single agent lookup |
| `SkillDir(name, global, home string) (string, bool)` | Resolved install path |
| `ProjectMCPConfigPath(name, home string) (string, bool)` | Resolved MCP config path |
| `ExpandedProjectSkillsSearch(name string) []string` | Audit scan dirs (project) |
| `ExpandedGlobalSkillsSearch(name, home string) []string` | Audit scan dirs (global) |
| `ExpandedPmSkillsSearch(name, home string) []string` | Audit scan dirs (package manager) |
| `ExpandHome(p, home string) string` | `~` expansion helper |

### Module Tree

```
internal/core/agent/
├── registry.go   — Info, Entry, init(), loadInto(), validateEntry()
│                   containsDotDot(), isAbsPath(), userAgentsPath()
│                   Names(), List(), Lookup(), SkillDir(),
│                   ProjectMCPConfigPath(), Expanded*Search(), ExpandHome()
└── agents.yaml   — embedded built-in definitions (source of truth for all agents)
```

---

## Rules for Future Agents

When extending or modifying `internal/core`, follow these rules:

1. **All new VCS backends must satisfy the compile-time assertion block** in `vcs.go`. Add `_ VCS = (*VcsYourBackend)(nil)` before submitting.

2. **Subprocess backends only.** Call `requireBinary(name)` as the **first statement** in every method that invokes a subprocess. This includes `Clone`, `Update`, `CurrentVersion`, and `HasChanges`. Never skip this step even if a sibling method already checked.

3. **Pure-Go first.** Prefer a pure-Go library over a subprocess when the library is available and maintained. `VcsGit` sets the precedent.

4. **New agents go in `agents.yaml` only.** Do not hard-code agent names, directories, or paths anywhere in Go source. `agents.yaml` is the single source of truth. After editing it, run `make test` — the embedded file is validated at startup in every test run.

5. **Path security is mandatory for all entries.** `validateEntry` enforces the security constraints at load time. Do not bypass it. When adding a new path field to `agentEntry`, add the corresponding validation rule to `validateEntry` **and** a test for each rejection case in `registry_internal_test.go`.

6. **Never widen path rules.** The constraints in `validateEntry` exist to prevent directory traversal. Do not relax them (e.g. allowing `..` or absolute paths) without a security review.

7. **The generic redirect is transparent.** Agents that set `supports_generic_project` or `supports_generic_global` must have empty `project_skills_dir` / `global_skills_dir` respectively. The redirect is applied inside `SkillDir()` — callers must never re-implement it.

8. **Test coverage target: ≥ 90% for `internal/core/vcs` and `internal/core/agent`.** Every new backend method needs at least one test for the happy path, one for the missing-binary path (subprocess backends), and one for each error branch. Use `makeFakeBin(t, name, script)` from `testutil_test.go` to simulate binaries without requiring real tools.
