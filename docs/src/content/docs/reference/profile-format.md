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
    path: assets/personas/brainstormer.md
  - type: statusline
    path: assets/statuslines/detailed.yaml

options:
  # Adapter-specific settings (reserved for future use)

scope: user
```

## Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | | Profile identifier. Must match the filename (without extension). |
| `description` | string | no | | Human-readable description. |
| `target` | string | yes | | Target to install to (`claude`, `codex`, or `opencode`). |
| `bundles` | list | no | | Bundles to include; same [entry forms](/tai/reference/bundle-format/#entry-forms) as a bundle's own list. |
| `assets` | list of [asset entries](#assets) | no | | Direct asset references. |
| `options` | map | no | | Adapter-specific settings (reserved for future use). |
| `scope` | string | no | `user` | Default installation scope for config assets. |
| `post_install` | list of steps, or list of strings | no | | Commands run after a successful install. See [post_install](#post_install). |
| `post_install_timeout` | string | no | `30s` | Duration bound for each `post_install` step that sets no `timeout` of its own (e.g. `"60s"`, `"2m"`). |

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

Optional. The bundles to resolve and include. Entries take the same forms as in a bundle's own `bundles:` list - a bare name, a `source:name` import, or a mapping carrying `exclude`. See [bundle format](/tai/reference/bundle-format/#entry-forms).

```yaml
bundles:
  - core
  - work:work-tools
  - name: work:extras
    exclude:
      - assets/skills/beagle
```

Bundles are resolved in declaration order. When the same asset path appears in multiple bundles, the **later bundle's version wins**. Tai logs an info message when such overrides occur.

## assets

Optional. Direct asset references, using the same format as [bundle asset entries](/tai/reference/bundle-format/#asset-entry-format).

```yaml
assets:
  - type: persona
    path: assets/personas/brainstormer.md
  - type: file
    path: assets/files/CLAUDE.md
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
    path: assets/personas/custom.md   # Highest priority
```

## options

Optional. A map of adapter-specific settings. This field is reserved for future use and currently has no effect on installation.

```yaml
options:
  # Future: adapter-specific configuration
```

## scope

Optional. Sets the default installation scope for this profile's config assets. Defaults to `user`.

| Scope | Description |
|-------|-------------|
| `user` | Install to user-level config files (e.g. `~/.claude/settings.json`) |
| `project` | Install to project-level config files, resolved against the working directory |
| `local` | Install to local config files - project-specific, conventionally not checked in |

```yaml
scope: user  # StatusLines go to ~/.claude/settings.json
```

The default is `user` because that is what tai does everywhere else: the target directory is user-global, and every file asset installs there. A `project` default would write config into whichever directory you happened to run `tai install` from.

Individual assets can override it:

```yaml
scope: user
assets:
  - type: statusline
    path: assets/statuslines/detailed.yaml
  - type: mcp
    path: assets/mcps/repo-tools.yaml
    scope: project    # this one is shared with the repo
```

Scope affects only config assets. File assets (personas, delegates, skills, files) always install to the target directory.

See [Config Assets](/tai/reference/config-assets/) for how each scope maps to a file per target.

## post_install

Optional. Commands run after the profile installs successfully - for side effects like notifications, follow-up scripts, or post-processing installed files. It is a list of named steps:

```yaml
post_install:
  - name: notify
    command: [osascript, -e, 'display notification "tai installed"']
  - name: memri
    command: [memri, setup, claude]
    timeout: 2m
post_install_timeout: 1m
```

| Step field | Type | Required | Description |
|------------|------|----------|-------------|
| `name` | string | yes | Identifies the step in output. Unique within the profile. |
| `command` | list of strings | yes | Argv to run. It runs directly with no shell; wrap with `["sh", "-c", ...]` for shell features. |
| `timeout` | string | no | Duration bound for this step. Falls back to `post_install_timeout`, then `30s`. |

A plain argv list is shorthand for one unnamed step, and was the only form before steps existed:

```yaml
post_install: [./scripts/after-install.sh]
```

Every step sees the inherited user environment plus `TAI_TARGET`, `TAI_TARGET_DIR`, `TAI_PROFILE`, `TAI_MANIFEST_DIR`, and `TAI_VERSION` (always set, so `set -u` scripts are safe). `TAI_TARGET_DIR` is where the target's files were installed, such as `~/.claude`. Steps do not run on `--dry-run` or when the install fails.

Steps run in order, and each one runs whether or not earlier steps failed. A failing step does not undo the install - files and manifest are already in place. Tai prints each failure with a tail of its stderr, runs the remaining steps, lists the steps that failed, reports the install as succeeded, and exits nonzero.

A step may modify files tai installed. After each step that succeeds, tai records the new checksums of the files that step changed, so its edits are not reported as conflicts on the next install. A failed step's edits are not recorded: the next install reports those files as conflicts, since a step that failed partway may have left a partial edit. Prefer separate steps over chaining commands in one `sh -c`, so one failing command does not hold back the others.

## Validation rules

Tai validates profiles on load and fails fast if:

- `name` is missing
- `target` is missing
- Neither `bundles` nor `assets` is specified
- An asset entry is missing `type` or `path`
- An asset entry uses an invalid `type`
- `post_install` mixes argv strings with steps, or is not a list
- A `post_install` step is missing `name` or `command`, repeats another step's `name`, has an unknown field, or has an invalid `timeout`
- `post_install` contains an empty argv element
- `post_install_timeout` is set without `post_install`, is unparseable, or is not positive
- An asset entry uses `dest` on a non-file type
- A `dest` value is an absolute path or contains path traversal (`..`)
