# gaal — Agent Guidelines

## Language

- All code, comments, identifiers, commit messages, and documentation must be written in **English**.
- Communication with the user is in French.

## Architecture

See [docs/architecture.md](docs/architecture.md) for a full description of the package structure, the configuration hierarchy, the VCS backends, sandbox mode, and cross-platform path conventions.

Key points:
- `internal/engine` is the single orchestrator — add no business logic there.
- For per-user gaal config paths, go through the helpers in `internal/config` and `internal/core/agent` (which intentionally override `os.UserConfigDir()` on macOS to prefer `$XDG_CONFIG_HOME` and otherwise use `~/.config/`). For other paths, use `os.UserHomeDir()` and `os.UserCacheDir()` — never hardcode `%AppData%`, etc.
- The `--sandbox` flag redirects `$HOME` via `os.Setenv("HOME", dir)` — all new code must honour this automatically.
- The `--verbose` flag switches the global `slog` level to `DEBUG` — no other mechanism exists for verbosity.

## Debug Logging

**Every function must emit at least one `slog.Debug` (or `slog.DebugContext`) call** describing what it is about to do.

Rules:
- Use `slog.DebugContext(ctx, ...)` when a `context.Context` is available.
- Use `slog.Debug(...)` for non-context functions.
- Log meaningful key/value pairs (path, url, name, count, …), not bare strings.
- Do not log secrets or credentials.

```go
// correct
slog.DebugContext(ctx, "cloning repository", "url", url, "path", shortPath(path))

// incorrect — no context, no structured fields
log.Printf("cloning %s", url)
```

## Build

After every change run:

```sh
make build
```

The build must succeed with zero errors before considering the task done.

## Tests

- Write a **unit test for every new function or behaviour** added.
- Tests live alongside their package: `internal/foo/foo_test.go`.
- Use table-driven tests (`[]struct{ ... }`) for functions with multiple input cases.
- Mock external I/O (filesystem, HTTP, subprocess) with interfaces or `httptest`/`os.TempDir()` — tests must not require network access or installed VCS binaries.
- Run the full test suite to validate the implementation:

```sh
make test
```

- To measure code coverage (target ≥ 90%) and generate an HTML report:

```sh
make coverage
```

All tests must pass before the task is considered complete.

## VCS Backends

- `VcsGit` uses `go-git` (pure Go, no binary required).
- `VcsMercurial`, `VcsSVN`, `VcsBazaar` spawn subprocesses. Always call `requireBinary(name)` as the **first statement** in every method that invokes a subprocess.
- Every new backend must be added to the compile-time assertion block in `internal/repo/vcs.go`.

## Cross-platform Paths

- Use `filepath.Join`, `filepath.IsAbs`, `filepath.ToSlash` — never string concatenation for paths.
- `~` expansion must handle both `~/` (POSIX) and `~\` (Windows): use `expandHome(p, home)` from `internal/skill/agents.go`.
- Global config: use `globalConfigFilePath()` from `internal/config/config.go`.
- User config: use `userConfigFilePath()` from `internal/config/config.go`. On macOS this returns `$XDG_CONFIG_HOME/gaal/` when set, otherwise `~/.config/gaal/` — do not call `os.UserConfigDir()` directly for user-scoped gaal paths.
- Cache: use `os.UserCacheDir()`.
