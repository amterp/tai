---
title: CLI Reference
description: Complete reference for all tai commands, subcommands, and flags
---

This page documents every tai command, flag, and option. For conceptual overviews, see [How Tai Works](/tai/concepts/how-tai-works/).

## Global flags

| Flag | Description |
|------|-------------|
| `--no-color` | Disable colored output |

## tai config

Manage tai's global configuration.

### tai config set

Set the active config repo path.

```bash
tai config set <path>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `path` | yes | Path to a tai config repository (directory containing `tai.yaml`) |

The path is resolved to an absolute path and stored in `~/.config/tai/config.yaml`. Tilde (`~`) expansion is supported.

```bash
tai config set ~/my-tai-config
# Active config set to: /Users/you/my-tai-config
```

### tai config show

Display current configuration settings.

```bash
tai config show
```

Shows:
- **Config directory** - where tai stores its global config (`~/.config/tai/`)
- **Cache directory** - where tai caches remote sources (`~/.cache/tai/`)
- **Active config** - path to the active config repo, or `(none)` if unset

```
Config directory: /Users/you/.config/tai
Cache directory:  /Users/you/.cache/tai
Active config:    /Users/you/my-tai-config
```

## tai list

List available resources from the active config repo.

All `list` subcommands require an active config repo (set via `tai config set`).

### tai list sources

List configured sources.

```bash
tai list sources
```

Shows each source's name, type (`local` or `tai-repo`), and path.

```
Sources:
  local       local     ./assets
  work-shared local     ~/work/ai-common
  team-repo   tai-repo  ~/team-tai-config
```

### tai list bundles

List available bundles.

```bash
tai list bundles
```

Shows each bundle's name and description.

```
Bundles:
  core          Essential assets for daily work
  work-extras   Additional assets from work sources
```

### tai list profiles

List available profiles.

```bash
tai list profiles
```

Shows each profile's name, target, and description.

```
Profiles:
  personal  claude  My personal development setup
  work      claude  Full work setup
```

### tai list assets

List available assets, optionally filtered by source.

```bash
tai list assets [--source/-s <name>]
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--source` | `-s` | no | Filter assets to only those from the named source |

Without `--source`, lists assets from all sources grouped by source name.

```bash
tai list assets
# local:
#   delegates/code-reviewer.md
#   personas/senior-swe.md
#   skills/council-participant
# work-shared:
#   personas/company-style.md

tai list assets --source local
# Assets from source 'local':
#   delegates/code-reviewer.md
#   personas/senior-swe.md
#   skills/council-participant
```

## tai show

Show details of a specific resource.

### tai show profile

Show resolved assets for a profile.

```bash
tai show profile <name>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | yes | Profile name to inspect |

Resolves all bundles and displays the final asset list with source information.

```
Profile: work
Target:  claude
Assets:  5

  persona/senior-swe.md (local)
  persona/company-style.md (work-shared)
  delegate/code-reviewer.md (local)
  skill/council-participant (local)
  file/CLAUDE.md (local)
```

### tai show bundle

Show assets in a bundle.

```bash
tai show bundle <name>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | yes | Bundle name to inspect |

Resolves nested bundles and shows the flattened asset list.

```
Bundle:      full-setup
Description: Comprehensive setup combining multiple bundles
Assets:      4

  persona/senior-swe.md (local)
  delegate/code-reviewer.md (local)
  persona/brainstormer.md (local)
  persona/company-style.md (work-shared)
```

## tai locate

Print the full filesystem path to an asset.

```bash
tai locate <asset-path>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `asset-path` | yes | Asset path, optionally source-qualified (`source:path`) |

If no source is specified, resolves against the `local` source.

```bash
tai locate personas/senior-swe.md
# /Users/you/my-tai-config/assets/personas/senior-swe.md

tai locate work-shared:personas/company-style.md
# /Users/you/work/ai-common/personas/company-style.md
```

## tai install

Install a profile to its target.

```bash
tai install <profile> [flags]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `profile` | yes | Profile name to install |

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | `-n` | `false` | Preview changes without writing any files |
| `--force` | `-f` | `false` | Overwrite conflicting files |
| `--skip-conflicts` | | `false` | Leave conflicting files as-is |
| `--var` | `-v` | | Template variables as `key1=val1,key2=val2` |
| `--diff` | | `false` | Show unified diffs for conflicting files |

The install pipeline resolves bundles, processes templates, transforms frontmatter, and writes files to the target directory. See [Installation Behavior](/tai/reference/installation-behavior/) for details.

```bash
# Preview what would happen
tai install work --dry-run
# Would install profile 'work' for claude...
# 5 added, 0 updated, 0 unchanged, 0 removed

# Install with template variables
tai install work --var project_name=MyApp

# Install, overwriting any conflicts
tai install work --force

# Show diffs when conflicts are found
tai install work --diff
```

Example output:

```
Installing profile 'work' for claude...
Override: personas/senior-swe.md (core -> work-extras)
5 added, 0 updated, 0 unchanged, 0 removed
Manifest written to /Users/you/.config/tai/manifests/
```

When conflicts are detected without `--force` or `--skip-conflicts`:

```
2 conflict(s) detected:
  output-styles/senior-swe.md: file modified since last install
  CLAUDE.md: unmanaged file exists

Use --force to overwrite, --skip-conflicts to skip, or --diff to inspect.
```

## tai reinstall

Re-run installation for already-installed profiles.

```bash
tai reinstall [target] [flags]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | no | Target to reinstall (e.g., `claude`, `codex`). If omitted, reinstalls all installed targets. |

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | `-n` | `false` | Preview changes without writing any files |
| `--force` | `-f` | `false` | Overwrite conflicting files |
| `--skip-conflicts` | | `false` | Leave conflicting files as-is |
| `--var` | `-v` | | Template variables as `key1=val1,key2=val2` |
| `--diff` | | `false` | Show unified diffs for conflicting files |

Reinstall reads the manifest to determine which profile was last installed, then runs the install pipeline again. This is useful after editing assets in your config repo.

```bash
# Reinstall all targets
tai reinstall

# Reinstall only Claude
tai reinstall claude

# Preview reinstall
tai reinstall --dry-run
```

If no profiles are currently installed, the command exits with an error message.

## tai uninstall

Remove all tai-managed files for a target.

```bash
tai uninstall <target>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | yes | Target to uninstall (e.g., `claude`, `codex`) |

Reads the installation manifest to determine which files tai installed, removes those files, and deletes the manifest. Files that used `{{ existing }}` templating have their original content restored from backups. Config entries (like statuslines) are removed from settings files.

Non-tai files in the target directory are never touched.

```bash
tai uninstall claude
# Uninstalling from claude...
#   - output-styles/senior-swe.md
#   - agents/code-reviewer.md
#   - CLAUDE.md
# 3 files removed
```

## tai status

Show what is currently installed.

```bash
tai status [target]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | no | Target to show status for. If omitted, shows all registered targets. |

```bash
tai status
# claude:
#   Profile:     work
#   Config repo: /Users/you/my-tai-config
#   Installed:   2025-01-21 14:30:05
#   Files:       5
#   Installed files:
#     output-styles/senior-swe.md
#     output-styles/company-style.md
#     agents/code-reviewer.md
#     skills/council-participant
#     CLAUDE.md
# codex: not installed

tai status claude
# claude:
#   Profile:     work
#   Config repo: /Users/you/my-tai-config
#   Installed:   2025-01-21 14:30:05
#   Files:       5
```

## tai run

Run a target with tai config applied.

```bash
tai run <target> [args...]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `target` | yes | Target to run (e.g., `claude`, `codex`) |
| `args` | no | Arguments passed through to the target |

This is a convenience wrapper that invokes the target CLI directly. Since tai installs config to the target's standard directory, no special runtime setup is needed - this simply runs the target binary with any extra arguments forwarded.

```bash
tai run claude
# Equivalent to running: claude

tai run claude --help
# Equivalent to running: claude --help

tai run codex
# Equivalent to running: codex
```

If the target process exits with a non-zero code, tai preserves that exit code.
