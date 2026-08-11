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

### URL source

A url source downloads a single file over http or https. Reach for it instead of a command source wrapping `curl` when you want one remote document - most often because you intend to [patch](/tai/concepts/patches/) it.

```yaml
sources:
  - name: upstream-guide
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    patches: patches/upstream-guide/
```

The file is stored in the source's cache directory under its filename, so the source is a **directory holding one file**:

```
~/.cache/tai/sources/<repo-key>/upstream-guide/
  GUIDE.md
```

That shape is deliberate: it lets the rest of tai treat a url source like any other. Patched views build from it, `tai patch` operates on it, and a skill reaches the file with an ordinary [include](/tai/reference/skill-includes/):

```yaml
# assets/skills/mine/tai.yaml
include:
  - from: upstream-guide
    path: GUIDE.md
    dest: refs/guide.md
```

The include receives the source's *patched* content. That combination - a url source with `patches:`, reached by an include - is how you keep a local edit on a document someone else controls.

By default the file is stored under the URL's basename. Set `filename:` when the URL has none:

```yaml
  - name: upstream-guide
    type: url
    url: https://example.com/api/raw?doc=42
    filename: GUIDE.md
```

Tai rejects the config at load time if it cannot derive a filename and you have not set one, rather than failing part-way through an install.

A url source carries **no assets** - it has no `personas/`, `skills/`, or other asset directory, so `tai list assets` shows nothing for it. That is the intended configuration, not a mistake. The source exists to be named by an include's `from:` and by `patches:`.

Freshness matches command sources exactly: fetched before every install, fetched by `tai refresh`, served from cache by read-only commands, and never fetched under `--dry-run`. A failed fetch leaves the previous cache intact, and an unreachable URL fails the install rather than quietly installing yesterday's content.

Requests carry a `tai/<version>` user agent, follow at most 10 redirects, and are capped at 10MB. Unlike a command source, a url source runs no code.

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
