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

An entry is either a bare name or a mapping. The mapping form exists for `exclude`; everything else reads better bare.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Bundle name, or `source:name` to import from another source. |
| `exclude` | string list | no | Root-relative paths or globs to drop from this import. |

```yaml
bundles:
  - core                     # this repo's bundles/core.yaml
  - work:work-tools          # the "work" source's bundles/work-tools.yaml
  - name: work:extras
    exclude:
      - assets/skills/beagle
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

## Validation rules

Tai validates bundles on load and fails fast if:

- `name` is missing
- An asset entry is missing `type` or `path`
- An asset entry uses an invalid `type`
- An asset entry uses `dest` on a non-file type
- A `dest` value is an absolute path or contains path traversal (`..`)
- A `bundles` entry has no name, or names a source but no bundle
- A `bundles` entry's `exclude` pattern is empty, absolute, or contains an interior `**`
