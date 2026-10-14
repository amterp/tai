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
    path: assets/personas/brainstormer.md

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
    path: assets/personas/brainstormer.md
  - type: file
    path: assets/files/README.md
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

Sets the default installation scope for config assets (statuslines, and in the future MCPs and hooks). Defaults to `user`, matching where every other asset type installs.

| Scope | Effect |
|-------|--------|
| `user` | Install to user-level config (e.g. `~/.claude/settings.json`) |
| `project` | Install to project-level config (e.g. `.claude/settings.json`), resolved against the working directory |
| `local` | Install to local config (e.g. `.claude/settings.local.json`) |

```yaml
scope: user  # StatusLines install to user-level settings
```

Individual assets can override the profile default with their own `scope` field.

### `post_install`

Named steps run after the profile installs successfully - for side effects like notifications, restarting tools, chaining follow-up scripts, or post-processing installed files.

```yaml
post_install:
  - name: notify
    command: [osascript, -e, 'display notification "tai installed"']
  - name: memri
    command: [memri, setup, claude]
post_install_timeout: 1m  # optional, default 30s per step
```

A plain argv list such as `post_install: [./scripts/after-install.sh]` is shorthand for a single step.

Steps see your environment plus `TAI_TARGET`, `TAI_TARGET_DIR`, `TAI_PROFILE`, `TAI_MANIFEST_DIR`, `TAI_SOURCE_DIR`, and `TAI_VERSION` (always set, so `set -u` scripts are safe). They do not run on dry-run or when the install fails.

Bundles can declare [steps of their own](/tai/concepts/bundles/#post-install-steps), which run before the profile's. Each step runs whether or not earlier steps failed. A failing step does not undo the install - files and manifest are already in place. tai prints each step's error with its stderr, lists the failed steps, reports the install as succeeded, and exits nonzero so scripts and CI notice. See [post_install](/tai/reference/profile-format/#post_install) for step fields and how steps that edit installed files are handled.

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
    path: assets/personas/brainstormer.md

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
    path: assets/files/RFC2119.md
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
