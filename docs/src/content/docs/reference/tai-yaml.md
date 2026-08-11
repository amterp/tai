---
title: "tai.yaml"
description: Complete reference for the config repo manifest file
---

The `tai.yaml` file at the root of your config repo is the entry point tai uses to discover sources, bundles, profiles, and assets. Every config repo must have one.

## Complete example

```yaml
version: 1

sources:
  # A directory of assets: personas/, skills/ ... sit at its top level
  - name: work-shared
    path: ~/work/ai-common

  # Another tai config repo: reference its assets as team-repo:assets/...
  - name: team-repo
    path: ~/team-tai-config

default_profile: work
```

## Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | integer | yes | | Schema version. Must be `1`. |
| `sources` | list | no | `[]` | Additional asset sources beyond the implicit local source. |
| `default_profile` | string | no | | Profile to use when `tai install` is called without specifying a profile. |

## version

Required. Must be `1`. This field enables tai to detect and handle future schema changes.

```yaml
version: 1
```

If the version is missing or set to a value other than `1`, tai exits with an error.

## sources

Optional. A list of locations that provide assets. Each source makes its assets available for reference in bundles and profiles.

### Source fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | | Unique identifier for this source. Used in `source:path` references. Must be a plain name (no path separators, `.`, or `..`). |
| `path` | string | local | | Filesystem path to the source root. Supports `~` expansion. Rejected for `command` and `url` sources. |
| `type` | string | no | `local` | Source type: `local`, `command`, or `url`. |
| `command` | string list | command | | Argv list that fetches the source's assets into `$TAI_DEST_DIR`. Only valid with `type: command`. |
| `url` | string | url | | The http(s) URL of the single file to download. Only valid with `type: url`. |
| `filename` | string | no | URL basename | Name the downloaded file is stored under. Must be a plain name. Only valid with `type: url`. |
| `timeout` | string | no | `30s` | Duration bound for the fetch (e.g. `"60s"`, `"2m"`). Only valid with `type: command` or `type: url`. |
| `patches` | string | no | | Config-repo-relative directory of patch files applied to this source's content. See [Patches](/tai/concepts/patches/). |
| `exclude` | string list | no | `[]` | Glob patterns for files this source does not carry at all. Patterns are source-relative. |

### Source types

| Type | Description |
|------|-------------|
| `local` | A directory already on disk. |
| `command` | A directory produced by running `command`, cached under `~/.cache/tai/sources/`. See [Sources](/tai/concepts/sources/) for the fetch contract and freshness rules. |
| `url` | A single file downloaded over http(s) and cached under `~/.cache/tai/sources/`, as a directory holding that one file. Carries no assets; exists to be reached by a skill include and patched. See [Sources](/tai/concepts/sources/). |

Whichever it is, a source is a directory and references are literal paths from its root. Nothing is appended, so a source pointed at a config repo takes `assets/...` references while one pointed at a bare folder of asset type directories does not.

A source also offers any bundles in `<root>/bundles/` for import as `source:bundle-name`. See [Bundles](/tai/concepts/bundles/#importing-bundles-from-other-sources).

### The implicit local source

Every config repo has an implicit source named `local` pointing at the config repo root. You do not need to declare it - tai adds it automatically with the lowest priority.

```yaml
# These two configurations are equivalent:

# Implicit (local added automatically)
version: 1

# Explicit
version: 1
sources:
  - name: local
    path: .
```

You only need to explicitly declare the `local` source if you want to override its path:

```yaml
sources:
  - name: local
    path: ./my-custom-root
```

### Source priority

Sources are evaluated in declaration order. When the same asset path exists in multiple sources, **later sources override earlier ones**. The implicit `local` source is prepended to the list, giving it the lowest priority.

```yaml
sources:
  # local is implicitly first (lowest priority)
  - name: shared
    path: ~/shared-assets        # Overrides local on conflict
  - name: overrides
    path: ~/custom-overrides     # Overrides shared and local on conflict
```

When an override occurs, tai logs an info message so you can track which version of an asset is being used.

### Source name rules

- Names must be unique within a single `tai.yaml`
- Names are used in asset references with the `source:path` syntax
- The name `local` is reserved for the implicit source (you can redefine it, but not duplicate it)

### Path resolution

Paths support tilde (`~`) expansion for the home directory. Relative paths are resolved relative to the config repo root.

```yaml
sources:
  - name: local
    path: .                       # Relative to config repo
  - name: shared
    path: ~/work/ai-common        # Absolute via tilde expansion
  - name: team
    path: ~/team-tai-config       # Referenced as team:assets/...
```

## default_profile

Optional. The profile name to use when `tai install` is called without a profile argument. This is a convenience feature - you can always specify the profile explicitly.

```yaml
default_profile: personal
```

## Validation rules

Tai validates `tai.yaml` on load and fails fast with a clear error if:

- `version` is missing or not `1`
- A source has no `name`, or the name contains path separators or is `.`/`..`
- A `local` source has no `path`, or sets `command`/`url`/`filename`/`timeout`
- A `command` source has no `command`, sets `path`/`url`/`filename`, has an empty argv element, or has a non-positive/unparseable `timeout`
- A `url` source has no `url`, uses a scheme other than http/https, sets `path`/`command`, has a `filename` that is not a plain name, or has a URL with no derivable filename and no `filename` set
- A source `type` is not `local`, `command`, or `url` (`tai-repo` was removed; point `path` at the same directory and write `assets/` in the reference)
- A `patches` or `exclude` value is an absolute path or contains `..`
- Two sources share the same `name`

## Minimal example

The smallest valid `tai.yaml`:

```yaml
version: 1
```

This creates a config repo with only the implicit `local` source, pointing at the repo root. You can define bundles and profiles without any additional sources.
