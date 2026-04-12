# Configuration System — gaal

> **Source of truth** for everything related to gaal configuration: data model,
> file locations, merge strategy, scope restriction policy, schema generation,
> and validation. All other documentation pages reference this file rather than
> duplicating these details.

---

## Overview — Three Pillars

The configuration system rests on three pillars that are deliberately kept
separate and independently swappable:

| Pillar | Responsibility | Key package |
|--------|---------------|-------------|
| **Config** | Holds the configuration data; two representations: files on disk (offline) and `*Config` / `*ResolvedConfig` in memory (runtime) | `internal/config` |
| **Schema** | Generates a JSON Schema that is always 1-to-1 with the runtime structs; used by IDEs for live YAML validation | `internal/config/schema` |
| **Validation** | Bridges memory ↔ files; guarantees perfectly consistent data at every load/merge boundary | `internal/config/schema` |

The schema generator and validator are both **swappable abstractions** — the
active implementation can be replaced at program start-up without touching any
calling code (useful for tests or for switching the underlying library).

---

## Configuration Levels

`gaal` loads and merges up to **three** configuration files, from lowest to
highest priority:

| Priority | Scope | Linux / macOS | Windows |
|----------|-------|---------------|---------|
| 1 — lowest | `global` | `/etc/gaal/config.yaml` | `%PROGRAMDATA%\gaal\config.yaml` |
| 2 | `user` | `$XDG_CONFIG_HOME/gaal/config.yaml` (defaults to `~/.config/gaal/config.yaml`) | `%AppData%\gaal\config.yaml` |
| 3 — highest | `workspace` | `--config` value (default: `gaal.yaml` in CWD) | ← same |

> macOS intentionally departs from `os.UserConfigDir()` (`~/Library/Application
> Support`) to prefer `~/.config` / XDG. Use `userConfigFilePath()` from
> `internal/config` — never call `os.UserConfigDir()` directly for gaal-scoped
> user paths.

Missing files are silently skipped. At least one file must be present; if none
is found `LoadChain` returns an error listing all three attempted paths.

---

## Data Model

### `Config` — top-level structure

`Config` maps 1-to-1 with a single YAML file on disk. It is **not** aware of
merging; merging is the responsibility of `LoadChain`.

Every field carries **four tag families**:

| Tag | Purpose |
|-----|---------|
| `yaml:"..."` | YAML key for file deserialization |
| `json:"..."` | JSON key (schema generator + JSON renderer) |
| `jsonschema:"description=...,enum=..."` | Annotations emitted into the JSON Schema |
| `validate:"..."` | Runtime validation rules (`go-playground/validator`) |

A fifth tag family, **`gaal:"maxscope=<scope>"`**, controls the scope
restriction policy (see [Scope Restriction Policy](#scope-restriction-policy)
below). Fields without this tag have no restriction — any level may override
them.

### Module tree

```
internal/config/
├── manager.go          — Config, LevelConfigs, ResolvedConfig
│                         Load(), LoadChain(), GenerateSchema()
├── scope.go            — ConfigScope, ScopeGlobal/User/Workspace, ParseConfigScope()
├── policy.go           — buildMergePolicy(), allowedAt()  [unexported]
├── utils.go            — indexOf(), deduplicate()  [unexported generics]
├── platform.go         — path constants, expandPaths(), isRemoteURL()
├── platform_unix.go    — GlobalConfigFilePath(), userConfigDir()
├── platform_darwin.go  — GlobalConfigFilePath(), userConfigDir()  (XDG override)
├── platform_windows.go — GlobalConfigFilePath(), userConfigDir()
└── schema/
    ├── generator.go           — Generator interface, Default, Set(), Generate()
    ├── generator_invopop.go   — GeneratorInvopop  (invopop/jsonschema)
    ├── validator.go           — Validator interface, DefaultValidator, SetValidator(), Validate()
    └── validator_playground.go — PlaygroundValidator  (go-playground/validator/v10)
```

### Type relationships

```
ResolvedConfig
├── *Config  (embedded — merged, runtime source of truth)
└── Levels  LevelConfigs
             ├── Global    *Config   (raw global file, nil if absent)
             ├── User      *Config   (raw user   file, nil if absent)
             └── Workspace *Config   (raw workspace file, nil if absent)

Config
├── Schema        *int
├── Repositories  map[string]ConfigRepo
├── Skills        []ConfigSkill
├── MCPs          []ConfigMcp
│                   └── Inline  *ConfigMcpItem
├── Telemetry     *bool          gaal:"maxscope=user"
└── SourcePath    string         yaml:"-"  (runtime only)
```

---

## Merge Strategy

`LoadChain` builds the merged config by calling `mergeFrom` for each level
in ascending priority order: `global → user → workspace`.

```
LoadChain:
  merged = {}
  merged.mergeFrom(global,    ScopeGlobal)
  merged.mergeFrom(user,      ScopeUser)
  merged.mergeFrom(workspace, ScopeWorkspace)
```

Per-field merge rules:

| Field | Rule |
|-------|------|
| `schema` | Source wins if non-nil; otherwise destination is preserved |
| `telemetry` | Source wins if non-nil **and** `scope ≤ maxscope=user` (workspace is silently ignored — see below) |
| `repositories` | Map merge — source entry wins on key conflict |
| `skills` | Upsert by `Source` — source entry replaces the existing entry with the same `Source` |
| `mcps` | Upsert by `Name` — source entry replaces the existing entry with the same `Name` |

Intra-file duplicates (same `Source` or `Name` within a single file) are
silently dropped, keeping the first occurrence. Cross-level deduplication
follows the upsert rules above.

---

## Scope Restriction Policy

### Motivation

Some configuration properties should not be overridable at every level. For
example, telemetry consent is a user-level decision — a project's `gaal.yaml`
must not be able to silently re-enable telemetry if the user has opted out.

### Mechanism — `gaal:"maxscope=<scope>"` tag

A field annotated with `gaal:"maxscope=<scope>"` declares the **highest scope
at which that field may be overridden**. Any config level whose scope is
strictly higher than the declared maximum is silently ignored for that field.

Scopes are ordered `global(0) < user(1) < workspace(2)`.

Examples:

| Annotation | Meaning |
|-----------|---------|
| `gaal:"maxscope=user"` | Only `global` and `user` may set/override this field; `workspace` is ignored |
| `gaal:"maxscope=global"` | Only `global` may set this field |
| _(no tag)_ | Any level may override (default behaviour) |

### Implementation

The restriction is **declarative and co-located** with the field definition.
At package initialisation, `buildMergePolicy` uses `reflect` to scan all
fields of `Config` once and build `fieldMergePolicy` (a `map[string]ConfigScope`).
The `allowedAt(field, scope)` helper then provides an O(1) lookup during
`mergeFrom`.

```
internal/config/policy.go
  buildMergePolicy(t reflect.Type) map[string]ConfigScope   — reflect scan, called once
  var fieldMergePolicy                                       — package-level cache
  allowedAt(field string, scope ConfigScope) bool           — scope ≤ max → true
```

Both `fieldMergePolicy` and `allowedAt` are **unexported** (package-internal).
`ConfigScope` and its constants are **exported** for use by diagnostics or
logging outside the package.

### Scope type

```go
type ConfigScope int

const (
    ScopeGlobal    ConfigScope = 0
    ScopeUser      ConfigScope = 1
    ScopeWorkspace ConfigScope = 2
)
```

`ParseConfigScope(s string) (ConfigScope, error)` accepts `"global"`, `"user"`,
`"workspace"` (case-sensitive).

### Current restrictions

| Field | maxscope | Effect |
|-------|---------|--------|
| `Telemetry` | `user` | The `workspace` level (`gaal.yaml`) cannot override telemetry; only `global` and `user` config files can |

### Adding a new restriction (future agents)

1. Add the `gaal:"maxscope=<scope>"` tag to the field in `Config`.
2. That is the **only** change required — `buildMergePolicy` and `allowedAt`
   pick up the new restriction automatically at next build.
3. Add a test in `manager_test.go`:
   - A `_CanOverride` test at the declared max scope (must succeed).
   - A `_CannotOverride` test one scope above (value must be silently ignored).

> ⚠️ `mergeFrom` currently has explicit scope guards only for fields that carry
> `gaal:"maxscope=..."`. For a new field at a higher scope to be blocked
> automatically, the guard in `mergeFrom` follows the pattern:
>
> ```go
> if src.MyField != nil && allowedAt("MyField", scope) {
>     c.MyField = src.MyField
> }
> ```
>
> This pattern must be manually added to `mergeFrom` for each new pointer field
> with a scope restriction. For non-pointer fields the design needs a sentinel /
> zero-value strategy.

---

## Schema Generation

The `Generator` interface produces a JSON Schema (draft-2020-12) from the
`Config` struct. The schema is always **1-to-1 with the runtime struct**;
`JSONSchemaExtend` customises it post-generation to tighten the contract for
IDE consumers (e.g. `schema` is marked `required` and constrained to `enum=[1]`).

| Symbol | Description |
|--------|-------------|
| `schema.Generator` | `interface{ Generate(v any) ([]byte, error) }` |
| `schema.Default` | Active `Generator` instance (init: `NewGeneratorInvopop()`) |
| `schema.Set(g Generator)` | Replace the active instance (call before `GenerateSchema`) |
| `schema.Generate(v any) ([]byte, error)` | Convenience wrapper |
| `config.GenerateSchema() ([]byte, error)` | Public entry-point used by `gaal schema` |

The default implementation uses `github.com/invopop/jsonschema` with
`AllowAdditionalProperties: false` — unknown YAML keys are rejected by
schema-aware IDEs.

---

## Validation

The `Validator` interface validates any struct value using the `validate` struct
tags. Errors reference YAML field names (e.g. `type: required`) rather than Go
identifiers.

| Symbol | Description |
|--------|-------------|
| `schema.Validator` | `interface{ Validate(v any) error }` |
| `schema.DefaultValidator` | Active `Validator` instance (init: `NewPlaygroundValidator()`) |
| `schema.SetValidator(v Validator)` | Replace the active instance |
| `schema.Validate(v any) error` | Convenience wrapper |

The default implementation uses `github.com/go-playground/validator/v10`.

Key validation rules:

| Field | Rule |
|-------|------|
| `ConfigRepo.Type` | `required, oneof=git hg svn bzr tar zip` |
| `ConfigRepo.URL` | `required` |
| `ConfigSkill.Source` | `required` |
| `ConfigMcp.Name` | `required` |
| `ConfigMcp.Target` | `required` |
| `ConfigMcp.Source` | `required_without=Inline` |
| `ConfigMcpItem.Command` | `required` |

---

## Public API

| Symbol | Description |
|--------|-------------|
| `Load(path string) (*Config, error)` | Parse + validate + expand a single file; intra-file duplicates dropped |
| `LoadChain(workspacePath string) (*ResolvedConfig, error)` | Merge global → user → workspace with scope-aware policy |
| `GlobalConfigFilePath() string` | System-wide config path for the current OS |
| `UserConfigFilePath() string` | Per-user config path (XDG-aware on Linux / macOS) |
| `GenerateSchema() ([]byte, error)` | JSON Schema for `Config`; delegates to `schema.Generate` |
| `ConfigScope` | Type representing a config scope (exported for diagnostics) |
| `ScopeGlobal`, `ScopeUser`, `ScopeWorkspace` | Scope constants |
| `ParseConfigScope(s string) (ConfigScope, error)` | Parse a scope from its string representation |

---

## Path Resolution

`expandPaths()` is called by `Load()`, anchored to the directory of the loaded
file. Rules:

| Input | Result |
|-------|--------|
| `~/` or `~\` | Expanded to `$HOME + rest` |
| Relative path (`./`, `../`, bare name) | `filepath.Join(baseDir, p)` |
| Remote URL (`http://`, `https://`, `git@`, `ssh://`) | Unchanged |
| GitHub shorthand (`owner/repo`) | Unchanged |
| Absolute path | Unchanged |

---

## Rules for Future Agents

When working on any code that touches the config system, follow these rules:

1. **One function = one responsibility.** `Load` = parse/validate/expand one
   file. `LoadChain` = orchestrate levels. `mergeFrom` = merge two `Config`
   values. Keep them that way.

2. **Tag all four families** on every field of `Config` and its nested structs
   (`yaml`, `json`, `jsonschema`, `validate`). Add `gaal:"maxscope=<scope>"`
   when the field must not propagate beyond a certain level.

3. **Never call `os.UserConfigDir()` directly** for gaal user-scoped paths —
   use `UserConfigFilePath()` / `userConfigFilePath()`. macOS intentionally
   overrides `UserConfigDir()` to prefer `~/.config`.

4. **Schema = runtime.** Any new field added to `Config` must be reflected in
   the generated `dist/schema.json`. Run `make build` (which regenerates the
   schema) and commit the updated `dist/schema.json`.

5. **Test coverage target: 100% for `internal/config`.** Every new function or
   behaviour needs at least one test. Use table-driven tests for functions with
   multiple input cases. Mock FS access with `os.TempDir()`.

6. **Scope restriction: declarative only.** To restrict a field, add
   `gaal:"maxscope=<scope>"` to its struct tag and add a guard in `mergeFrom`
   using `allowedAt`. Do not encode scope logic anywhere else.

7. **`ResolvedConfig.Levels` is read-only diagnostics.** Never write to any
   `Config` inside `Levels` after `LoadChain` returns — it holds the raw
   per-file snapshot.
