---
title: Profiles
description: Profiles bind bundles to a specific target, producing the installable configuration
---

Profiles are what you actually install. A profile binds one or more [bundles](/tai/concepts/bundles/) to a specific [target](/tai/concepts/targets/), producing the final set of [assets](/tai/concepts/assets/) that get installed.

## Structure

A profile is a YAML file in your config repo's `profiles/` directory.

```yaml
# profiles/work.yaml
name: work
description: Full work setup
target: claude

bundles:
  - core
  - work-extras

assets:
  - type: persona
    path: personas/brainstormer.md

options:
  # Adapter-specific settings (passed to the adapter during install)

scope: project
```

## The `target` field

The `target` field determines which adapter is used to translate and install your assets. Currently supported values:

- `claude` - installs to Claude Code (`~/.claude/`)
- `codex` - installs to Codex (`~/.codex/` or `$CODEX_HOME`)

The adapter handles terminology mapping, frontmatter transformation, and path resolution. See [Targets](/tai/concepts/targets/) for details.

## Bundle ordering

Bundle order matters. Bundles are resolved in list order, and **later bundles override earlier ones** when asset paths conflict. Tai logs an info message when an override occurs.

```yaml
bundles:
  - core           # Resolved first
  - work-extras    # Overrides core on conflict
```

If both `core` and `work-extras` include `personas/senior-swe.md`, the version from `work-extras` wins.

## Direct assets

Profiles can include individual assets directly via the `assets` field, in addition to (or instead of) bundles. Direct assets are resolved after all bundles and override bundle assets on conflict.

```yaml
bundles:
  - core

assets:
  - type: persona
    path: personas/brainstormer.md
  - type: file
    path: files/README.md
    dest: ""
```

## Optional fields

### `options`

Adapter-specific settings passed to the target during installation. The available options depend on the target adapter.

```yaml
options:
  # Future example: Claude-specific options
  # dangerously_skip_permissions: false
```

### `scope`

Controls where config assets (statuslines, and in the future MCPs and hooks) are installed. Defaults to `project`.

| Scope | Effect |
|-------|--------|
| `project` | Install to project-level config (e.g., `.claude/settings.json`) |
| `user` | Install to user-level config (e.g., `~/.claude/settings.json`) |

```yaml
scope: user  # StatusLines install to user-level settings
```

## Default profile

You can set a default profile in `tai.yaml` so you don't need to specify it every time:

```yaml
# tai.yaml
version: 1
default_profile: personal
```

With a default set, `tai install` (with no arguments) installs the default profile.

## Complete example

```yaml
# profiles/personal.yaml
name: personal
description: Personal setup for side projects
target: claude

bundles:
  - core

assets:
  - type: persona
    path: personas/brainstormer.md

---

# profiles/work.yaml
name: work
description: Full work setup with company standards
target: claude

bundles:
  - core
  - work-extras

assets:
  - type: file
    path: files/RFC2119.md
    dest: ""

options: {}
scope: project

---

# profiles/codex-personal.yaml
name: codex-personal
description: Personal setup for Codex
target: codex

bundles:
  - core
```
