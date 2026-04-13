# FS-aware Discovery — `internal/discover`

## Overview

`internal/discover` provides **configuration-independent** resource discovery: it scans the filesystem to find skills, repositories, and MCP configuration files that are actually installed on the machine, regardless of whether they appear in `gaal.yaml`. This powers the Git-inspired drift detection used by `gaal status` and `gaal info`.

The design follows two principles:

1. **FS-first**: discovered resources are the ground truth; config-declared resources are reconciled on top.
2. **Git index fast path**: `stat()` → size+mtime match → no change; mismatch → sha256 comparison → if hash matches, update mtime only (racy-git repair). VCS-native detection (`vcs.HasChanges`) is used first for directories tracked by a VCS.

---

## Resource Model

### `ResourceType`

| Constant | Value | Description |
|----------|-------|-------------|
| `ResourceSkill` | `"skill"` | A skill directory (contains `SKILL.md`) |
| `ResourceRepo` | `"repo"` | A VCS-tracked repository |
| `ResourceMCP` | `"mcp"` | An MCP JSON configuration file |

### `Scope`

| Constant | Value | Description |
|----------|-------|-------------|
| `ScopeGlobal` | `"global"` | Installed in a global agent directory (e.g. `~/.claude/skills/`) |
| `ScopeUser` | `"user"` | Installed in a user agent directory (e.g. `~/.config/claude/skills/`) |
| `ScopeWorkspace` | `"workspace"` | Discovered inside the current working directory |

### `DriftState`

| Constant | Value | Meaning |
|----------|-------|---------|
| `DriftOK` | `"ok"` | Resource matches its last-synced snapshot |
| `DriftModified` | `"modified"` | Resource has changed since last sync |
| `DriftMissing` | `"missing"` | Resource was recorded in a snapshot but is no longer present |
| `DriftUnmanaged` | `"unmanaged"` | Resource found on disk but not tracked by any snapshot |
| `DriftUnknown` | `"unknown"` | State cannot be determined (e.g. snapshot error) |

### `Resource`

```go
type Resource struct {
    Type    ResourceType
    Scope   Scope
    Path    string          // absolute path to the resource
    Name    string          // display name (skill name, repo URL, MCP name)
    Drift   DriftState
    VCSType string          // for repos: "git", "hg", "svn", "bzr"
    Managed bool            // true if the resource is declared in gaal.yaml
    Meta    map[string]string // extra attributes (e.g. "agent" for skills)
}
```

---

## Snapshot System

Snapshots provide the reference state against which drift is measured. Each snapshot is a JSON file stored in the state directory (`~/.cache/gaal/state/` by default, sandbox-aware).

### `FileRecord`

```go
type FileRecord struct {
    Size    int64
    ModTime time.Time
    Hash    [32]byte // sha256
}
```

### `Snapshot`

A `Snapshot` is a `map[string]FileRecord` keyed by relative path (for directories) or the file's base name (for single-file resources like MCP configs).

### Key functions

| Function | Description |
|----------|-------------|
| `Load(path string) (Snapshot, error)` | Deserialize a snapshot from disk; returns empty snapshot if the file does not exist |
| `Save(path string, s Snapshot) error` | Atomic write (temp file + `os.Rename`) |
| `Record(path string) (FileRecord, error)` | Stat + hash a single file |
| `SnapshotDir(root string) (Snapshot, error)` | Walk a directory tree and record every file |
| `DiffPath(root string, s Snapshot) ([]Change, error)` | Compare current FS state against snapshot using the Git fast-path heuristic |
| `SnapshotPath(stateDir, key string) string` | Canonical path for a snapshot file: `stateDir/<key>.json` |
| `WorkdirKey(path string) string` | 8-character hex key derived from `sha256(path)[:4]` — used to namespace snapshots per installation path |

### Snapshot lifecycle

```
gaal sync
  └─► manager.Sync()
        └─► installSkill() / mergeIntoTarget() / clone+update
              └─► writeSkillSnapshot() / writeMCPSnapshot()
                    └─► discover.SnapshotDir() or discover.Record()
                          └─► discover.Save(SnapshotPath(stateDir, key), snap)

gaal status / gaal info
  └─► discover.Scan()
        └─► computeSkillDrift() / computeRepoDrift() / computeMCPDrift()
              ├─► hasVCSMarker(dir) → vcs.HasChanges()   ← fast-path for VCS dirs
              └─► discover.DiffPath(root, snap)           ← snapshot fallback
```

Snapshots are keyed by `WorkdirKey(path)` so that two different installation paths for the same skill name do not collide.

---

## Scan

### `ScanOptions`

| Field | Default | Description |
|-------|---------|-------------|
| `MaxDepth` | `4` | Maximum depth for workspace FS walk |
| `IncludeWorkspace` | `false` | Whether to scan `workDir` for repos and skills |
| `StateDir` | `""` | Override for the state directory (snapshot storage) |
| `Timeout` | `2s` | Context deadline added around the workspace scan |

### `Scan(ctx, home, workDir string, opts ScanOptions) ([]Resource, error)`

Public entry point. Calls three sub-scanners in sequence:

1. `scanGlobal(ctx, home, workDir, stateDir)` — agent-registry skill directories
2. `scanMCPs(ctx, home, stateDir)` — agent MCP config files
3. `scanWorkspace(ctx, workDir, maxDepth, stateDir)` — depth-limited FS walk (when `IncludeWorkspace` is true, wrapped in a 2-second timeout)

Results are deduplicated by path before being returned.

---

## Per-resource-type scan logic

### Skills — `global.go`

Iterates every registered agent (`core/agent.List()`), resolves its skill directory (global and user scope), then calls `skillsFromDir()`. For each sub-directory a `Resource` is emitted with:

- `Type = ResourceSkill`
- `Scope` = global or user (derived from which path matched)
- `Name` = last path segment of the skill directory
- `Meta["agent"]` = the owning agent's name
- `Drift` = result of `computeSkillDrift()`

`computeSkillDrift()` tries VCS detection first (`hasVCSMarker` + `vcs.HasChanges`); if the directory is not a VCS working copy it falls back to `DiffPath` against the stored snapshot.

### Repositories — `workspace.go`

`scanWorkspace()` walks `workDir` up to `maxDepth` levels, skipping `node_modules`, `vendor`, `dist`, `.cache`, and `bin`. Whenever a VCS marker (`.git`, `.hg`, `.svn`, `.bzr`) is found **inside** a directory (not at the root), that directory is emitted as a `ResourceRepo`:

- `VCSType` = detected VCS string
- `Drift` = `computeRepoDrift()` which calls `vcs.HasChanges()`

Directories containing `SKILL.md` at their root are emitted as `ResourceSkill` with `Scope = ScopeWorkspace`.

### MCPs — `mcp.go`

`scanMCPs()` iterates `core/agent.List()`, resolves each agent's MCP config file path via `agent.MCPConfigPath()`, and for each existing file emits a `ResourceMCP`:

- `Path` = absolute path to the config JSON file
- `Name` = agent name
- `Drift` = `computeMCPDrift()` — stat fast-path against snapshot, sha256 fallback

---

## Integration with `engine/ops`

`Collect()` in `engine/ops/status.go` is the main consumer:

```go
discovered, err := discover.Scan(ctx, home, workDir, discover.ScanOptions{
    IncludeWorkspace: true,
    StateDir:         stateDir,
})
```

After the scan, three reconcile helpers (`reconcileRepos`, `reconcileSkills`, `reconcileMCPs`) merge config-declared entries (from the three managers) with FS-discovered resources. Config-declared resources take priority; FS-discovered resources that have no matching config entry are appended as unmanaged.

The `driftToStatus()` mapper translates `DriftState` to the `render.StatusCode` displayed in the table and JSON output:

| DriftState | StatusCode |
|------------|------------|
| `DriftOK` | `StatusOK` |
| `DriftModified` | `StatusDirty` |
| `DriftMissing` | `StatusNotCloned` |
| others | `StatusOK` |

---

## State directory

The state directory defaults to `filepath.Join(os.UserCacheDir(), "gaal", "state")`:

| OS | Default path |
|----|-------------|
| Linux | `~/.cache/gaal/state/` |
| macOS | `~/Library/Caches/gaal/state/` |
| Windows | `%LocalAppData%\gaal\state\` |

In `--sandbox` mode `os.UserCacheDir()` is redirected, so all snapshot files resolve inside the sandbox tree automatically.

The path can be overridden for testing via `engine.Options.StateDir`.
