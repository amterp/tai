---
title: "tai.yaml"
description: Complete reference for the config repo manifest file
---

The `tai.yaml` file at the root of your config repo is the entry point tai uses to discover sources, bundles, profiles, and assets. Every config repo must have one.

## Complete example

```yaml
version: 1

sources:
  # Another local directory providing assets
  - name: work-shared
    path: ~/work/ai-common

  # Another tai config repo (uses its assets/ directory)
  - name: team-repo
    type: tai-repo
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
| `path` | string | local/tai-repo | | Filesystem path to the source directory. Supports `~` expansion. Rejected for `command` sources. |
| `type` | string | no | `local` | Source type: `local`, `tai-repo`, or `command`. |
| `command` | string list | command | | Argv list that fetches the source's assets into `$TAI_DEST_DIR`. Only valid with `type: command`. |
| `timeout` | string | no | `30s` | Duration bound for `command` (e.g. `"60s"`, `"2m"`). Only valid with `type: command`. |

### Source types

| Type | Description |
|------|-------------|
| `local` | A directory containing asset files directly. Assets are read from the directory root. |
| `tai-repo` | Another tai config repo. Assets are read from its `assets/` subdirectory. |
| `command` | Assets are fetched by running `command` and cached under `~/.cache/tai/sources/`. See [Sources](/tai/concepts/sources/) for the fetch contract and freshness rules. |

### The implicit local source

Every config repo has an implicit source named `local` that points to the `./assets` directory relative to the config repo root. You do not need to declare it - tai adds it automatically with the lowest priority.

```yaml
# These two configurations are equivalent:

# Implicit (local added automatically)
version: 1

# Explicit
version: 1
sources:
  - name: local
    path: ./assets
```

You only need to explicitly declare the `local` source if you want to override its path:

```yaml
sources:
  - name: local
    path: ./my-custom-assets-dir
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
    path: ./assets                # Relative to config repo
  - name: shared
    path: ~/work/ai-common       # Absolute via tilde expansion
  - name: team
    type: tai-repo
    path: ~/team-tai-config       # Reads from ~/team-tai-config/assets/
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
- A `local` or `tai-repo` source has no `path`, or sets `command`/`timeout`
- A `command` source has no `command`, sets `path`, has an empty argv element, or has a non-positive/unparseable `timeout`
- A source `type` is not `local`, `tai-repo`, or `command`
- Two sources share the same `name`

## Minimal example

The smallest valid `tai.yaml`:

```yaml
version: 1
```

This creates a config repo with only the implicit `local` source pointing to `./assets`. You can define bundles and profiles without any additional sources.
