---
title: Bundle Format
description: Complete YAML reference for bundle definition files
---

Bundles are named collections of assets that serve as building blocks for [profiles](/tai/reference/profile-format/). Bundle files live in the `bundles/` directory of your config repo and are YAML files.

## Complete example

```yaml
name: full-setup
description: Comprehensive setup combining multiple bundles

bundles:
  - core
  - work-extras

assets:
  - type: persona
    path: assets/personas/brainstormer.md
  - type: delegate
    path: work-shared:delegates/security-reviewer.md
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""
  - type: statusline
    path: assets/statuslines/detailed.yaml
```

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Bundle identifier. Must match the filename (without extension). |
| `description` | string | no | Human-readable description of the bundle's purpose. |
| `bundles` | list | no | Other bundles to include; see [entry forms](#entry-forms). |
| `assets` | list of [asset entries](#asset-entry-format) | no | Individual asset references. |
| `post_install` | list of steps | no | Commands run after any install that includes this bundle. See [post_install](#post_install). |

A bundle must have at least one of `bundles` or `assets` to be useful, but neither is strictly required by validation.

## name

Required. Must match the bundle's filename without the `.yaml` or `.yml` extension.

```yaml
# bundles/core.yaml
name: core  # Must be "core" to match the filename
```

If the name does not match the filename, tai will load the bundle using the name in the file, but this is discouraged as it creates confusion.

## description

Optional. Displayed by `tai list bundles` and `tai show bundle`.

```yaml
description: Essential assets for daily development
```

## bundles

Optional. A list of other bundles to include. This enables composition - you can build complex bundles from simpler ones.

```yaml
bundles:
  - core
  - work-extras
```

### Entry forms

An entry is either a bare name or a mapping. The mapping form exists for `exclude` and `skip_post_install`; everything else reads better bare.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Bundle name, or `source:name` to import from another source. |
| `exclude` | string list | no | Root-relative paths or globs to drop from this import. |
| `skip_post_install` | string list | no | Names of [post_install steps](#post_install) this import does not run. |

```yaml
bundles:
  - core                     # this repo's bundles/core.yaml
  - work:work-tools          # the "work" source's bundles/work-tools.yaml
  - name: work:extras
    exclude:
      - assets/skills/beagle
  - name: work:approval
    skip_post_install:
      - render
```

### Imported bundles

A `source:name` entry resolves against that source's `bundles/` directory. Inside an imported bundle, unprefixed asset paths and unprefixed included bundle names resolve against **that bundle's own repo**, so upstream's `assets/skills/dredge` keeps meaning upstream's file. A prefixed path resolves in **your** `tai.yaml`, which must therefore declare any third source the imported bundle reaches into.

Exclusions apply to the import's fully resolved contents, reaching assets that arrived through a nested include. A pattern that matches nothing warns rather than failing: upstream may have deleted the asset, which already satisfies the intent. Patterns follow the same rules as a source's `exclude:` - see [tai.yaml](/tai/reference/tai-yaml/).

### Resolution order

Included bundles are resolved recursively in declaration order. When the same asset appears in multiple included bundles, the **later bundle's version wins**. Assets from the `assets` list (below) are appended after all included bundles.

```yaml
# full-setup.yaml
bundles:
  - base       # Resolved first (lowest priority)
  - overrides  # Resolved second (overrides base on conflict)
assets:
  - type: persona
    path: assets/personas/custom.md  # Appended last (highest priority)
```

### Cycle detection

Circular bundle references (e.g., A includes B, B includes A) are detected and cause tai to exit with a clear error. This check is recursive - indirect cycles like A includes B includes C includes A are also caught.

## assets

Optional. A list of asset entry objects defining which assets belong to this bundle.

### Asset entry format

Each entry is a YAML mapping with the following fields:

| Field | Type | Required | Applies to | Description |
|-------|------|----------|------------|-------------|
| `type` | string | yes | all | Asset type (see table below) |
| `path` | string | yes | all | Path to the asset, optionally source-qualified |
| `dest` | string | file only | file | Target destination path |

### Asset types

| Type value | Asset type | Description |
|------------|-----------|-------------|
| `persona` | Persona | Behavioral style or role |
| `delegate` | Delegate | Specialized sub-agent |
| `skill` | Skill | Procedural capability (directory with SKILL.md) |
| `file` | File | Loose file copied as-is |
| `statusline` | StatusLine | Status bar configuration (config asset) |

### path

The path to the asset within a source. Paths are relative to the source root.

Without a source prefix, the path resolves against the `local` source:

```yaml
assets:
  - type: persona
    path: assets/personas/senior-swe.md   # local source, i.e. this repo
```

#### Source-qualified paths

To reference assets from other sources, use the `source:path` syntax:

```yaml
assets:
  - type: persona
    path: work-shared:personas/company-style.md
  - type: delegate
    path: team-repo:delegates/security-reviewer.md
```

The source name must match a source defined in your [tai.yaml](/tai/reference/tai-yaml/).

### dest (file type only)

The `dest` field is valid only for `file` type assets. It controls where the file is placed in the target directory.

| dest value | Behavior | Example result (Claude) |
|------------|----------|------------------------|
| `""` (empty) | Target root directory | `~/.claude/README.md` |
| `"config"` | Subdirectory (no extension in basename) | `~/.claude/config/README.md` |
| `"config/"` | Explicit subdirectory (trailing slash) | `~/.claude/config/README.md` |
| `"CLAUDE.md"` | Exact file path (extension in basename) | `~/.claude/CLAUDE.md` |
| `".claude-plugin/"` | Dot-named subdirectory (needs the trailing slash, or it is read as a file name like `.gitignore`) | `~/.claude/.claude-plugin/README.md` |

Using `dest` on a non-file asset type produces a validation error.

The `dest` field supports [template variables](/tai/reference/template-syntax/#dest-path-templates), enabling target-aware file placement.

## Override semantics

When the same asset path appears multiple times during bundle resolution (from included bundles or direct assets), the **last occurrence wins**. Tai logs an info message when an override occurs so you can trace the resolution.

```yaml
# If both 'base' and 'overrides' bundles contain personas/senior-swe.md,
# the version from 'overrides' is used because it appears later.
bundles:
  - base
  - overrides
```

## post_install

Optional. Steps that run after any install whose resolved set includes this bundle, for a bundle that does nothing until a command runs - a policy file a script has to render, a tool that has to register itself. Declaring the step on the bundle means profiles don't each have to repeat it.

```yaml
name: approval
assets:
  - type: file
    path: assets/files/approval-policy.yaml
    dest: ""
post_install:
  - name: render
    command: [./scripts/render-approval.sh]
    timeout: 1m
```

Steps take the same fields as a [profile's steps](/tai/reference/profile-format/#post_install): `name`, `command`, and optional `timeout`. A bundle must use the list of named steps; the plain argv shorthand is rejected.

- **Working directory.** A bundle step runs in the root of the repo the bundle came from - the directory holding its `bundles/` dir - so `./scripts/render-approval.sh` names a script shipped beside the bundle, whichever repo imports it. For an imported bundle that is the source's root, or its patched view or fetched cache where those apply. The step also sees that path as `TAI_SOURCE_DIR`.
- **Patched sources.** A step from a patched source runs in the patched view, so a patched script runs as patched. The view is a copy tai rebuilds each time it loads the source: it has no `.git` and none of the source's excluded files, and anything a step writes inside it is gone after the next tai command. Write output under `TAI_TARGET_DIR` or another path the step owns.
- **Order.** Bundle steps run before the profile's own steps, in bundle resolution order: an included bundle's steps before those of the bundle including it.
- **Once per install.** A bundle reached through two imports runs its steps once; a step is identified by its bundle plus its name. Output names the bundle each step came from, as in `post_install step "render" from bundle "work:approval"`.
- **Timeouts.** A step uses its own `timeout`, then `30s`. A profile's `post_install_timeout` does not apply to bundle steps.

`tai show bundle` and `tai show profile` list the steps that would run. Importing a bundle means trusting its steps to run on your machine at install.

To skip a step on one machine, list its name under `skip_post_install` on the `bundles:` entry that imports it:

```yaml
bundles:
  - name: work:approval
    skip_post_install:
      - render
```

- **Reach.** The skip applies to the import's fully resolved steps, so it reaches steps of bundles the import includes. A name matches every step of that name there; there is no `bundle:step` form.
- **Per entry.** Another `bundles:` entry that reaches the same bundle without the skip still runs the step.
- **Stale names warn.** A name matching no step warns rather than failing: upstream deleting or renaming the step already satisfies the intent.

`tai install` prints a `Skipped:` line for each skipped step, and `tai show` leaves it out of the step list.

## Validation rules

Tai validates bundles on load and fails fast if:

- `name` is missing
- An asset entry is missing `type` or `path`
- An asset entry uses an invalid `type`
- An asset entry uses `dest` on a non-file type
- A `dest` value is an absolute path or contains path traversal (`..`)
- A `bundles` entry has no name, or names a source but no bundle
- A `bundles` entry's `exclude` pattern is empty, absolute, or contains an interior `**`
- A `bundles` entry's `skip_post_install` lists an empty name
- `post_install` is a plain argv list rather than a list of named steps
- A `post_install` step is missing `name` or `command`, repeats another step's `name`, has an unknown field, or has an invalid `timeout`
