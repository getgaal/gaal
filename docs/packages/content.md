# `internal/content`

> Generic file and directory sync for agent-owned content that is not a
> native `SKILL.md` package: instruction files, commands, hooks, rules, and
> settings.

## Configuration Shape

```yaml
content:
  - source: my-org/agent-guidance
    targets:
      - agents: ["claude-code"]
        scope: project
        root: workspace
        paths:
          AGENTS.md: CLAUDE.md

      - agents: ["codex"]
        scope: project
        root: workspace
        paths:
          AGENTS.md: AGENTS.md
```

`root: workspace` resolves destinations relative to the current project
directory. `root: agent` resolves destinations relative to the agent config
root inferred from the registry's skills directory, e.g. `~/.claude` from
`~/.claude/skills`.

The shorter form is useful when one target is enough:

```yaml
content:
  - source: gregqualls/dotclaude
    agents: ["claude-code"]
    global: true
    paths:
      commands/: commands/
      rules/: rules/
      settings.json: settings.json
```

## Safety

- Source and destination paths must be relative and cannot contain `..`.
- Symlinks and non-regular files are skipped.
- VCS metadata directories (`.git`, `.hg`, `.svn`, `.bzr`) are skipped.
- Directory copies stage into a sibling temporary directory and then replace
  the destination.

## Relationship To Skills

`skills:` remains the semantic `SKILL.md` resource. Use `content:` for files
and directories that agents consume directly but that are not native skills.
