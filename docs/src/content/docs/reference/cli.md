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

Shows each source's name, type (`local`, `command`, or `url`), and path.

```
Sources:
  local       local     .
  work-shared local     ~/work/ai-common
  team-repo   local     ~/team-tai-config
```

### tai list bundles

List available bundles.

```bash
tai list bundles
```

Shows this repo's bundles, then those offered by each source, grouped under the source name. A grouped entry is written exactly as shown, so it can be pasted into a `bundles:` list.

```
Bundles:
  core                Essential assets for daily work
  work-extras         Additional assets from work sources
team-repo:
  team-repo:standards Required company assets
```

A source whose `bundles/` directory cannot be read produces a warning and is skipped, rather than taking out the whole listing.

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
| `name` | yes | Bundle name to inspect, optionally source-qualified (`source:name`) |

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
tai locate assets/personas/senior-swe.md
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

If the config declares [command sources](/tai/concepts/sources/#command-source), install fetches them first so the installed content reflects upstream; a failed fetch fails the install. `--dry-run` never fetches (a preview must not have side effects) - it resolves from the cache and prints what it would refresh. After a successful install, the profile's `post_install` hook runs if declared; a hook failure leaves the install in place but makes tai exit nonzero.

```bash
# Preview what would happen
tai install work --dry-run
# Would install profile 'work' for claude...
# 5 would be added, 0 restored, 0 updated, 0 unchanged, 0 removed

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
5 added, 0 restored, 0 updated, 0 unchanged, 0 removed
Manifest written to /Users/you/.config/tai/manifests/
```

`restored` counts assets tai manages that were missing from the target and got
written back - a skill you deleted by hand, or a hook someone removed from
`settings.json`. They are kept apart from `added` because the manifest already
had them, and from `unchanged` because the target did change.

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

Like `tai install`, reinstall re-fetches any command sources first (once, before all targets). If a profile's `post_install` hook fails, reinstall continues with the remaining targets and exits nonzero at the end.

```bash
# Reinstall all targets
tai reinstall

# Reinstall only Claude
tai reinstall claude

# Preview reinstall
tai reinstall --dry-run
```

If no profiles are currently installed, the command exits with an error message.

## tai refresh

Fetch remote sources into the cache without installing anything. Applies to `command` and `url` sources.

```bash
tai refresh [source]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `source` | no | Source to fetch. If omitted, fetches every source that needs fetching. |

Use this on a fresh machine (read-only commands like `tai list assets` need the cache populated) or to update the cache without touching targets. `tai install` and `tai reinstall` refresh automatically, so an explicit refresh before installing is not required.

Naming a source that is not fetched (a `local` or `tai-repo` source) is an error. A failed fetch leaves the previous cache intact.

```bash
tai refresh            # Fetch all command and url sources
tai refresh company    # Fetch just the "company" source
```

## tai patch

Manage local edits on content from sources you do not control. See [Patches](/tai/concepts/patches/) for the concept and [Patching imported assets](/tai/guides/patching-imported-assets/) for the workflow.

```bash
tai patch <list|diff|bless|workspace|regenerate>
```

Patches are named `source:name`, where `name` is the patch file's path within the source's patch directory, minus the extension. A bare name is accepted wherever it is unambiguous.

### tai patch list

Show every declared patch and whether it still applies.

```bash
tai patch list
```

```bash
tai patch list
# Patches:
#   upstream:use-pnpm    we standardized on pnpm (upstream:skills/rad/SKILL.md)  ok
#   upstream:drop-telem  (upstream:skills/rad/SKILL.md)                          unblessed
```

| Status | Meaning |
|--------|---------|
| `ok` | Applies cleanly; upstream matches the blessed snapshot |
| `ok, upstream drifted` | Applies cleanly, but upstream differs from the snapshot |
| `unblessed` | Applies cleanly; no snapshot recorded yet |
| `BROKEN` | An edit could not be applied. The reason is shown inline. |
| `target file missing` | The source does not carry the file the patch targets |
| `source not fetched` | A command or url source whose fetch has not run - try `tai refresh` |

### tai patch diff

Show what changed upstream since the patch was blessed.

```bash
tai patch diff <name>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | yes | Patch name, bare or `source:name` |

Compares the blessed snapshot against the current pristine upstream file. Errors if the patch has never been blessed - there is nothing to compare against.

### tai patch bless

Record the current upstream content as reviewed.

```bash
tai patch bless <name>
tai patch bless --all
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | one of | Patch name, bare or `source:name` |
| `--all` | one of | Bless every patch that applies cleanly |

Writes a `.blessed` snapshot next to the patch file, inside your config repo. Commit it so drift detection travels with the repo.

Tai refuses to bless a patch that does not apply, targets a missing file, or belongs to an unfetched source - recording a snapshot in those states would bake in a broken patch and silence the warning that would prompt a fix. Under `--all` those become per-patch warnings and the rest still get blessed.

### tai patch workspace

Build an editable copy of a source for authoring or repairing patches.

```bash
tai patch workspace <source>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `source` | yes | Source whose patches you want to edit |

Creates a directory under `~/.cache/tai/workspaces/` containing the source's content with its patches applied. Edit files there, then run `tai patch regenerate`. Patches that could not be applied are reported and left out, so you can re-make their changes by hand.

When an install fails on a broken patch, tai leaves the workspace in place already - the error message names its path.

### tai patch regenerate

Rewrite a patch from your workspace edits.

```bash
tai patch regenerate <name>
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | yes | Patch name, bare or `source:name` |

Diffs the workspace file against the pristine source (plus any sibling patches on the same file), derives `find`/`replace` edits, widens context until each `find` matches exactly once, and verifies the result reproduces your edits byte-for-byte before writing.

Review the rewritten patch, then `tai patch bless` it.

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
#   Profile:        work
#   Config repo:    /Users/you/my-tai-config
#   Installed:      2025-01-21 14:30:05
#   Files:          5
#   Config entries: 2
#   Installed files:
#     output-styles/senior-swe.md
#     output-styles/company-style.md
#     agents/code-reviewer.md
#     skills/council-participant
#     CLAUDE.md
#   Installed config entries:
#     [hook] format-on-save -> /Users/you/.claude/settings.json
#     [mcp] github -> /Users/you/.claude.json
# codex: not installed

tai status claude
# claude:
#   Profile:        work
#   Config repo:    /Users/you/my-tai-config
#   Installed:      2025-01-21 14:30:05
#   Files:          5
#   Config entries: 2
```

Config assets install as entries inside a target's config files, so they never
appear in the file list. They are listed separately, with the file each one
landed in.

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
