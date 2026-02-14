---
title: Sources
description: Sources provide assets that bundles and profiles reference - local directories, tai repos, and more
---

Sources are locations that provide [assets](/tai/concepts/assets/). They are defined in `tai.yaml` and referenced by [bundles](/tai/concepts/bundles/) and [profiles](/tai/concepts/profiles/) when specifying asset paths.

## The implicit `local` source

Every config repo has an implicit source called `local` that points to the `./assets` directory in the config repo. You do not need to declare it - tai adds it automatically.

```
my-tai-config/
  tai.yaml
  assets/          <-- This is the "local" source
    personas/
    delegates/
    skills/
    files/
    statuslines/
```

When a bundle references `personas/senior-swe.md` without a source prefix, it resolves to `local`, which means `./assets/personas/senior-swe.md`.

You only need to explicitly declare the `local` source if you want to override its default path.

## Source types

### Local directory

A local directory source points to a folder on disk that contains assets organized in the standard `personas/`, `delegates/`, `skills/`, etc. structure.

```yaml
sources:
  - name: work-shared
    path: ~/work/ai-common
```

The directory at `~/work/ai-common` should contain asset subdirectories directly (not nested under an `assets/` folder).

### Tai repo

A tai-repo source points to another tai config repo. Tai uses that repo's `assets/` directory as the source.

```yaml
sources:
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config
```

This makes `~/personal-tai-config/assets/` available as the `personal` source. The difference from a plain local directory is that tai knows to look inside the `assets/` subdirectory.

### Git remote (planned)

Git remote sources will allow referencing assets from a git repository. This is planned for a future release.

```yaml
# Future syntax
sources:
  - name: company
    git: github.com/company/ai-standards
    ref: main
```

## Source priority

Sources are resolved in declaration order. **Later sources override earlier ones** when asset paths conflict. List base/default sources first and overrides last.

```yaml
sources:
  - name: local        # Base assets
    path: ./assets
  - name: work-shared  # Can override local assets
    path: ~/work/ai-common
```

## Source-qualified paths

Bundles and profiles reference assets from specific sources using the `source:path` syntax:

```yaml
assets:
  - type: persona
    path: personas/senior-swe.md              # Defaults to "local" source
  - type: persona
    path: work-shared:personas/company-style.md  # Explicit source
  - type: delegate
    path: personal:delegates/pair-programmer.md  # From tai-repo source
```

The source name before the colon must match a source defined in `tai.yaml`.

## Configuration examples

### Single-source setup

The simplest config uses only the implicit local source:

```yaml
# tai.yaml
version: 1
default_profile: personal
```

All assets live in `./assets/` and bundles reference them without a source prefix.

### Multi-source setup

A work setup pulling from multiple locations:

```yaml
# tai.yaml
version: 1

sources:
  - name: work-shared
    path: ~/work/ai-common

  - name: personal
    type: tai-repo
    path: ~/personal-tai-config

default_profile: work
```

### Cross-machine portability

Sources defined in `tai.yaml` let the same bundles and profiles work across machines. Each machine just needs the source paths to be valid locally:

```yaml
# Machine A: tai.yaml
sources:
  - name: shared
    path: /opt/ai-configs

# Machine B: tai.yaml
sources:
  - name: shared
    path: ~/company/ai-configs
```

The bundles referencing `shared:personas/company-style.md` work on both machines as long as each has the source directory at the configured path.
