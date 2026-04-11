# Sync must skip uninstalled agents

**Issue**: [gmg-inc/gaal-lite#17](https://github.com/gmg-inc/gaal-lite/issues/17)
**Date**: 2026-04-11
**Status**: Approved, ready for implementation plan

## Problem

`gaal sync` materialises configuration files under agent-owned directories even when the agent is not installed on the host. On a machine that never had zencoder installed, a recent sync run created `~/.zencoder/` and a zencoder MCP config file as a side effect. Sync should leave uninstalled agents entirely alone.

Two independent code paths are responsible:

1. **Skill sync** (`internal/skill/manager.go`) — install-presence is only checked when the config uses `agents: ["*"]`. Explicit agent lists such as `agents: [zencoder]` bypass the check and reach `installSkill`, which happily creates `.zencoder/skills/` (project) or `~/.zencoder/skills/` (global).

2. **MCP sync** (`internal/mcp/manager.go`) — `mergeIntoTarget` calls `os.MkdirAll(filepath.Dir(target), 0o755)` unconditionally before writing. Any entry whose `target:` lives inside an agent-owned directory will cause that directory to be created on a fresh machine.

## Goal

Sync updates agent configurations **only for agents that are installed on this machine**. An agent is considered installed when the directory that would own its configuration (parent of the skills directory, or parent of the MCP config file) already exists on disk — the same rule the skill `*` expansion already applies.

Skipped entries log a `slog.Warn` line and continue; they are not errors.

## Non-goals

- No new `agent:` field on `MCPConfig`.
- No shared `agent.IsInstalled` helper across packages.
- No `--force` / `--allow-uninstalled` override flag.
- No changes to the `init` wizard (it already operates over discovered candidates).
- No changes to status/audit output beyond what naturally results from fewer files being written.

## Design

### Guiding principle

**Sync never creates an agent-owned directory as a side effect.**

If the directory that would contain a resource does not already exist, the agent is treated as not installed and the entry is skipped. This rule is applied uniformly to skill sync and MCP sync.

### Change 1 — Skill sync

File: `internal/skill/manager.go`.

Today:

```go
func (m *Manager) resolveAgents(sc config.SkillConfig) []string {
    if len(sc.Agents) == 0 || (len(sc.Agents) == 1 && sc.Agents[0] == "*") {
        return m.detectInstalledAgents(sc.Global)
    }
    return sc.Agents
}

func (m *Manager) detectInstalledAgents(global bool) []string {
    var found []string
    for _, name := range AgentNames() {
        dir, ok := SkillDir(name, global, m.home)
        if !ok {
            continue
        }
        checkDir := dir
        if !global && !filepath.IsAbs(dir) {
            checkDir = filepath.Join(m.workDir, filepath.Dir(dir))
        } else {
            checkDir = filepath.Dir(expandHome(dir, m.home))
        }
        if _, err := os.Stat(checkDir); err == nil {
            found = append(found, name)
        }
    }
    return found
}
```

Refactor:

1. Extract the stat check into a helper that works for a single agent:

   ```go
   func (m *Manager) isAgentInstalled(name string, global bool) bool {
       dir, ok := SkillDir(name, global, m.home)
       if !ok {
           return false
       }
       checkDir := dir
       if !global && !filepath.IsAbs(dir) {
           checkDir = filepath.Join(m.workDir, filepath.Dir(dir))
       } else {
           checkDir = filepath.Dir(expandHome(dir, m.home))
       }
       _, err := os.Stat(checkDir)
       return err == nil
   }
   ```

2. Rewrite `detectInstalledAgents` as a filter over `AgentNames()` delegating to `isAgentInstalled`. Behaviour is unchanged.

3. Rewrite `resolveAgents` so explicit agent lists are filtered through the same helper:

   ```go
   func (m *Manager) resolveAgents(sc config.SkillConfig) []string {
       if len(sc.Agents) == 0 || (len(sc.Agents) == 1 && sc.Agents[0] == "*") {
           return m.detectInstalledAgents(sc.Global)
       }
       out := make([]string, 0, len(sc.Agents))
       for _, a := range sc.Agents {
           if m.isAgentInstalled(a, sc.Global) {
               out = append(out, a)
               continue
           }
           slog.Warn("skill: skipping uninstalled agent",
               "agent", a, "source", sc.Source, "global", sc.Global)
       }
       return out
   }
   ```

`syncOne` and `installSkill` require no changes; they never see uninstalled agents.

**Generic-path agents.** Agents with `SupportsGenericProject: true` / `SupportsGenericGlobal: true` (e.g. `cursor`, `codex`, `amp`) have their `SkillDir` redirected to the shared `generic` paths (`.agents/skills`, `~/.agents/skills`). `isAgentInstalled` therefore becomes "does `.agents` / `~/.agents` exist" for every such agent at once. This matches what `detectInstalledAgents` already does for `agents: ["*"]`, and is a deliberate consequence of the "never create the directory" principle: on a project that doesn't use `.agents` yet, a user who writes `agents: [cursor]` must `mkdir .agents` themselves before sync will install. This is not a regression relative to the `*` case and is consistent across all generic-supporting agents.

### Change 2 — MCP sync

File: `internal/mcp/manager.go`.

Today:

```go
func mergeIntoTarget(target, name string, entry serverEntry) error {
    slog.Debug("merging mcp entry into target", "name", name, "target", target)
    if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
        return fmt.Errorf("creating config directory: %w", err)
    }
    // ... read/merge/write ...
}
```

Replace the head of the function:

```go
func mergeIntoTarget(target, name string, entry serverEntry) error {
    slog.Debug("merging mcp entry into target", "name", name, "target", target)

    parent := filepath.Dir(target)
    info, err := os.Stat(parent)
    if err != nil {
        if os.IsNotExist(err) {
            slog.Warn("mcp: skipping entry — target parent directory does not exist",
                "name", name, "target", target, "parent", parent)
            return nil
        }
        return fmt.Errorf("stat %s: %w", parent, err)
    }
    if !info.IsDir() {
        return fmt.Errorf("%s is not a directory", parent)
    }

    // ... existing read/merge/write logic, unchanged ...
}
```

The `os.MkdirAll` call is removed entirely. If the parent directory exists, the target file can still be created in place on first sync; if it does not, the entry is silently skipped.

This correctly handles the observed cases:

- `~/.zencoder/mcp.json` on a machine without zencoder → `~/.zencoder` missing → skip.
- `~/.vscode/settings.json` on a machine with VS Code → `~/.vscode` present → merge (shared across cline / github-copilot / roo is fine because they all share the same target).
- `~/.config/claude/claude_desktop_config.json` when Claude Desktop is not installed → `~/.config/claude` missing even though `~/.config` exists → skip.

### Change 3 — Status reporting

No code changes. `mcp.Manager.Status` already stats the target file and reports "not present" when it is missing, which remains true for skipped entries. Skipped skill entries are simply absent from the `Status` result because `resolveAgents` filters them. Richer reporting ("skipped because agent not installed") can be added later if it becomes valuable.

## Tests

### `internal/skill/manager_test.go`

Add a table-driven test for `resolveAgents` with explicit agent lists mixing installed and uninstalled agents. Use `t.TempDir()` for `home` and `workDir`, pre-create a subset of agent directories (`.claude`, `.cursor`, …), and assert that the returned slice contains only the pre-created ones. Cover both `global: true` and `global: false`.

A second test should confirm that an explicit list with **only** uninstalled agents returns an empty slice and does not create any directories on disk afterwards.

### `internal/mcp/manager_test.go`

Two new cases against `Manager.Sync`:

1. **Parent missing** — target is `<tmp>/.zencoder/mcp.json`, `<tmp>/.zencoder` does not exist. Expect: `Sync` returns `nil`, `<tmp>/.zencoder` is still absent, `mcp.json` is not created.
2. **Parent present** — target is `<tmp>/.claude/mcp.json`, `<tmp>/.claude` pre-created. Expect: `mcp.json` is created and contains the inline server entry. This is a regression guard for the happy path.

Both cases use `t.TempDir()` for isolation.

## Acceptance criteria

- [ ] Running `gaal sync` on a config targeting an uninstalled agent produces no files under that agent's tree and emits a warning log line.
- [ ] Running `gaal sync` for installed agents works exactly as before.
- [ ] `agents: ["*"]` expansion continues to match today's behaviour.
- [ ] `make test` passes with the new cases.
- [ ] `make build` succeeds.
