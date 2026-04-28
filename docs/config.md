# Configuration System — gaal

> **Source of truth** for everything related to gaal configuration: data model,
> file locations, merge strategy, scope restriction policy, schema generation,
> and validation. All other documentation pages reference this file rather than
> duplicating these details.

---

## Overview — Five Pillars

The configuration system rests on five pillars that are deliberately kept
separate and independently swappable:

| Pillar | Responsibility | Key package |
|--------|---------------|-------------|
| **Config** | Holds the configuration data; two representations: files on disk (offline) and `*Config` / `*ResolvedConfig` in memory (runtime) | `internal/config` |
| **Scope Policy** | Declares which config levels may override each field (`gaal:"maxscope="` tag); enforced declaratively at merge time | `internal/config` |
| **Schema** | Generates a JSON Schema always 1-to-1 with the runtime structs; used by IDEs for live YAML validation | `internal/config/schema` |
| **Validation** | Bridges memory ↔ files; guarantees perfectly consistent data at every load/merge boundary | `internal/config/schema` |
| **Platform** | Resolves OS-specific config file paths; isolated from build constraints | `internal/config/platform` |
| **Template** | Generates the documented YAML skeleton written by `gaal init`; auto-synced with struct tags (field names, descriptions, enums, required/optional) | `internal/config/template` |

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
> Support`) to prefer `~/.config` / XDG. Use `UserConfigFilePath()` from
> `internal/config` (which delegates to `internal/config/platform`) — never
> call `os.UserConfigDir()` directly for gaal-scoped user paths.

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
│                         UserConfigFilePath()  [thin wrapper → platform]
├── scope.go            — ConfigScope, ScopeGlobal/User/Workspace, ParseConfigScope()
├── policy.go           — buildMergePolicy(), allowedAt()  [unexported]
├── utils.go            — indexOf(), deduplicate()  [unexported generics]
│                         isRemoteURL(), isGitHubShorthand(), expandPaths()
├── platform/
│   ├── manager.go      — path constants, UserConfigFilePath(), userConfigFilePath()
│   ├── unix.go         — GlobalConfigFilePath(), userConfigDir()  (!windows && !darwin)
│   ├── darwin.go       — GlobalConfigFilePath(), userConfigDir()  (XDG override)
│   └── windows.go      — GlobalConfigFilePath(), userConfigDir()
└── schema/
│   ├── generator.go           — Generator interface, Default, Set(), Generate()
│   ├── generator_invopop.go   — GeneratorInvopop  (invopop/jsonschema)
│   ├── validator.go           — Validator interface, DefaultValidator, SetValidator(), Validate()
│   └── validator_playground.go — PlaygroundValidator  (go-playground/validator/v10)
└── template/
    ├── reflector.go    — Reflect(t) → []FieldSpec  (struct tag introspection)
    └── manager.go      — Generate() → documented YAML skeleton
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

## Schema Sub-package (`internal/config/schema`)

Schema generation and struct validation are isolated in their own package so
that both concerns can be **swapped independently** without touching any calling
code — useful for tests or for changing the underlying library.

### Responsibilities

| Concern | Interface | Default implementation |
|---------|-----------|----------------------|
| JSON Schema generation | `Generator` | `GeneratorInvopop` (invopop/jsonschema) |
| Struct validation | `Validator` | `PlaygroundValidator` (go-playground/validator/v10) |

### Generator

Produces a JSON Schema (draft-2020-12) from any Go value. The schema is always
**1-to-1 with the runtime struct**; `JSONSchemaExtend` on `Config` customises
it post-generation (e.g. `schema` marked `required`, constrained to `enum=[1]`).

| Symbol | Description |
|--------|-------------|
| `Generator` | `interface{ Generate(v any) ([]byte, error) }` |
| `Default` | Active instance (init: `NewGeneratorInvopop()`) |
| `Set(g Generator)` | Swap the active instance (call before `GenerateSchema`) |
| `Generate(v any) ([]byte, error)` | Convenience wrapper around `Default` |

Default behaviour of `GeneratorInvopop`: `AllowAdditionalProperties: false` —
unknown YAML keys are rejected by schema-aware IDEs.

### Validator

Validates any struct value using `validate` struct tags. Errors reference YAML
field names (e.g. `type: required`) rather than Go identifiers, thanks to a
custom `RegisterTagNameFunc` that reads the `yaml` tag.

| Symbol | Description |
|--------|-------------|
| `Validator` | `interface{ Validate(v any) error }` |
| `DefaultValidator` | Active instance (init: `NewPlaygroundValidator()`) |
| `SetValidator(v Validator)` | Swap the active instance |
| `Validate(v any) error` | Convenience wrapper around `DefaultValidator` |

Key validation rules applied to `Config` structs:

| Field | Rule |
|-------|------|
| `ConfigRepo.Type` | `required, oneof=git hg svn bzr tar zip` |
| `ConfigRepo.URL` | `required` |
| `ConfigSkill.Source` | `required` |
| `ConfigMcp.Name` | `required` |
| `ConfigMcp.Target` | _(deprecated)_ explicit path; prefer `agents` + `global` |
| `ConfigMcp.Source` | `required_without=Inline` |
| `ConfigMcpItem.Command` | `required` |

### Relationship with `internal/config`

`internal/config` never imports the underlying libraries directly. Instead:

```
config.GenerateSchema()        — public entry-point (manager.go)
  └─→ schema.Generate(v)       — convenience wrapper
        └─→ schema.Default.Generate(v)   — GeneratorInvopop (swappable)

config.Load() → cfg.validate()
  └─→ schema.Validate(v)       — convenience wrapper
        └─→ schema.DefaultValidator.Validate(v)  — PlaygroundValidator (swappable)
```

### Tests

Tests live in `schema/generator_test.go` and `schema/validator_test.go`
(package `schema_test`). They use lightweight stub structs to keep tests fully
independent of `Config`. Swapping the active implementation is tested to ensure
the abstraction boundary holds.

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

## Platform Sub-package (`internal/config/platform`)

OS-specific path resolution is isolated in its own package so that the rest of
`internal/config` stays free of build constraints and `//go:build` tags.

### Responsibilities

| Symbol | Description |
|--------|-------------|
| `GlobalConfigFilePath() string` | System-wide config path for the current OS |
| `UserConfigFilePath() string` | Per-user config path (exported accessor) |
| `userConfigFilePath() string` | Internal implementation — resolves via `userConfigDir()` with fallback to `~/.config` |
| `userConfigDir() (string, error)` | OS-specific directory lookup (defined per build constraint) |

### Build-constraint files

| File | Build constraint | Behaviour |
|------|-----------------|-----------|
| `unix.go` | `!windows && !darwin` | Delegates to `os.UserConfigDir()` which honours `$XDG_CONFIG_HOME` |
| `darwin.go` | `darwin` | Prefers `$XDG_CONFIG_HOME`; falls back to `~/.config` (overrides `os.UserConfigDir()` which returns `~/Library/Application Support`) |
| `windows.go` | `windows` | Global path uses `%PROGRAMDATA%`; user path delegates to `os.UserConfigDir()` (`%AppData%`) |

### Relationship with `internal/config`

`internal/config` never calls `os.UserConfigDir()` directly. Instead:

```
config.UserConfigFilePath()          — public wrapper in manager.go
  └─→ platform.UserConfigFilePath()  — exported from platform/manager.go
        └─→ platform.userConfigFilePath()
              └─→ platform.userConfigDir()  (OS-specific, one per build tag)
```

`config.LoadChain()` calls `platform.GlobalConfigFilePath()` and
`platform.UserConfigFilePath()` directly for the global and user candidates.

### Tests

All path-resolution tests live in `platform/manager_test.go` (package `platform`).
They cover: correct suffix on each OS, XDG override on macOS/Linux, fallback
when `HOME` is broken.

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
   use `UserConfigFilePath()` from `internal/config` (or `platform.UserConfigFilePath()`
   from `internal/config/platform`). macOS intentionally overrides `UserConfigDir()`
   to prefer `~/.config`.

4. **Schema = runtime.** Any new field added to `Config` must be reflected in
   the generated schema output. Run `make build` (which regenerates the
   schema) and keep the generated schema in sync with the current build output
   location.

5. **Test coverage target: 100% for `internal/config` and `internal/config/platform`.**
   Every new function or behaviour needs at least one test. Tests for OS-specific
   path helpers live in `platform/manager_test.go`; tests for path expansion
   helpers live in `utils_test.go`. Use table-driven tests for functions with
   multiple input cases. Mock FS access with `os.TempDir()`.

6. **Scope restriction: declarative only.** To restrict a field, add
   `gaal:"maxscope=<scope>"` to its struct tag and add a guard in `mergeFrom`
   using `allowedAt`. Do not encode scope logic anywhere else.

7. **`ResolvedConfig.Levels` is read-only diagnostics.** Never write to any
   `Config` inside `Levels` after `LoadChain` returns — it holds the raw
   per-file snapshot.
