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

### Command source

A command source materializes its assets by running a command you declare. This covers remote repos - including private ones behind SSH or token auth - and any other fetch shape, without tai needing to understand each auth scheme.

```yaml
sources:
  - name: company
    type: command
    command:
      - sh
      - -c
      - git clone --depth=1 git@github.com:company/ai-standards.git "$TAI_DEST_DIR"
    timeout: 2m   # optional, default 30s
```

The command is an argv list executed directly (no shell - wrap with `sh -c` for shell features). tai runs it with a staging directory exposed as `$TAI_DEST_DIR`; whatever the command writes there is atomically swapped into a cache under `~/.cache/tai/sources/` and from then on the source behaves exactly like a local directory - the command's output should contain the usual `personas/`, `skills/`, etc. subdirectories.

The command also sees `TAI_SOURCE` (the source name), `TAI_CONFIG_REPO` (absolute path of the config repo), and `TAI_VERSION`. Your full environment is inherited, so keep credentials in env vars (e.g. `$GITLAB_TOKEN`) rather than in the argv - argv is echoed in error messages, env vars are not.

Freshness works like this:

- `tai install` and `tai reinstall` re-fetch command sources first, so installs always reflect upstream. A failed fetch fails the install and leaves the previous cache intact.
- `tai refresh [source]` fetches without installing - use it on a fresh machine or to update the cache for read-only commands.
- Read-only commands (`list`, `show`, `locate`) serve the cache without fetching. Before the first fetch, resolving assets from the source fails with a hint to run `tai refresh`.

Command sources run arbitrary code with your environment - treat a config repo that declares them the same as running its code.

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
