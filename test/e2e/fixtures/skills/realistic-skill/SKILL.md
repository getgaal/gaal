---
name: realistic-skill
description: Multi-file skill (SKILL.md + prompts/ + templates/) used to verify gaal preserves the directory structure during install.
---

# realistic-skill

This skill ships several companion assets:

- `prompts/main.md`  — primary instructions referenced by the SKILL.md body
- `templates/component.tsx` — boilerplate the skill copies into a project

The gaal e2e suite uses this fixture to assert that the entire directory tree
lands inside the agent's skill directory after `gaal sync`.
