---
title: Profile Format
description: Complete YAML reference for profile definition files
---

Profiles bind one or more [bundles](/tai/reference/bundle-format/) to a specific target, producing the final set of assets to install. Profile files live in the `profiles/` directory of your config repo.

## Complete example

```yaml
name: work
description: Full work setup
target: claude

bundles:
  - core
  - work-extras

assets:
  - type: persona
    path: personas/brainstormer.md
  - type: statusline
    path: statuslines/detailed.yaml

options:
  # Adapter-specific settings (reserved for future use)

scope: project
```

## Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | | Profile identifier. Must match the filename (without extension). |
| `description` | string | no | | Human-readable description. |
| `target` | string | yes | | Target to install to (`claude`, `codex`, or `opencode`). |
| `bundles` | list of strings | no | | Bundle names to include. |
| `assets` | list of [asset entries](#assets) | no | | Direct asset references. |
| `options` | map | no | | Adapter-specific settings (reserved for future use). |
| `scope` | string | no | `project` | Installation scope for config assets. |

A profile must have at least one of `bundles` or `assets`.

## name

Required. Must match the profile's filename without the `.yaml` or `.yml` extension.

```yaml
# profiles/personal.yaml
name: personal
```

## target

Required. Specifies which AI tool this profile installs to. Determines which adapter handles translation and where files are written.

| Target | Adapter | Target directory |
|--------|---------|------------------|
| `claude` | Claude Code | `~/.claude/` |
| `codex` | Codex | `$CODEX_HOME` or `~/.codex/` |
| `opencode` | OpenCode | `$XDG_CONFIG_HOME/opencode` or `~/.config/opencode/` |

```yaml
target: claude
```

If the target name does not match a registered adapter, tai exits with an error.

## bundles

Optional. A list of bundle names to resolve and include.

```yaml
bundles:
  - core
  - work-extras
```

Bundles are resolved in declaration order. When the same asset path appears in multiple bundles, the **later bundle's version wins**. Tai logs an info message when such overrides occur.

## assets

Optional. Direct asset references, using the same format as [bundle asset entries](/tai/reference/bundle-format/#asset-entry-format).

```yaml
assets:
  - type: persona
    path: personas/brainstormer.md
  - type: file
    path: files/CLAUDE.md
    dest: ""
```

Each entry has `type`, `path`, and optionally `dest` (for file type only). Source-qualified paths (`source:path`) are supported.

## Resolution order

The final asset list is built by:

1. Resolving each bundle in `bundles` order (later overrides earlier on conflict)
2. Appending each entry in `assets` (highest priority)

This means direct `assets` entries in a profile always take precedence over anything from bundles.

```yaml
bundles:
  - base            # Resolved first (lowest priority)
  - overrides       # Resolved second
assets:
  - type: persona
    path: personas/custom.md   # Highest priority
```

## options

Optional. A map of adapter-specific settings. This field is reserved for future use and currently has no effect on installation.

```yaml
options:
  # Future: adapter-specific configuration
```

## scope

Optional. Controls where config assets (statuslines, and future MCPs/hooks) are installed. Defaults to `project`.

| Scope | Description |
|-------|-------------|
| `project` | Install to project-level config files |
| `user` | Install to user-level config files |
| `local` | Install to local config files (project-specific, not checked in) |

```yaml
scope: user  # StatusLines go to ~/.claude/settings.json
```

The scope affects which settings file config assets are merged into. File assets (personas, delegates, skills, files) are not affected by scope - they always install to the target directory.

See [Config Assets](/tai/reference/config-assets/) for details on how scope maps to specific files per target.

## Validation rules

Tai validates profiles on load and fails fast if:

- `name` is missing
- `target` is missing
- Neither `bundles` nor `assets` is specified
- An asset entry is missing `type` or `path`
- An asset entry uses an invalid `type`
- An asset entry uses `dest` on a non-file type
- A `dest` value is an absolute path or contains path traversal (`..`)
