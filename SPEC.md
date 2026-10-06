# tai Specification

**Version:** 0.1 (MVP)
**Status:** Implemented

## Overview

tai is a CLI tool for managing AI agent configurations. It provides a framework for organizing, composing, and installing assets (personas, delegates, skills, reference docs) across different AI targets (Claude Code, Codex, Gemini CLI, OpenCode) while keeping configuration version-controlled and portable.

## Core Concepts

### Terminology

| tai term | What it is | Claude Code equivalent |
|----------|------------|------------------------|
| **Target** | The AI tool we install to | Claude Code, Codex, Gemini CLI, OpenCode |
| **Persona** | A behavioral style/role | output-style |
| **Delegate** | A specialized helper for tasks | agents (sub-agents) |
| **Skill** | A procedural capability | skills |
| **File** | Loose file copied as-is | CLAUDE.md, supporting docs |
| **StatusLine** | Status bar configuration | statusLine entry in settings.json |
| **MCP** | Model Context Protocol server | MCP server entry in config |
| **Hook** | Event-triggered shell command | Hook entry in settings |

Additional terms:
- **Asset**: An atomic configuration unit—a single persona, delegate, skill, file, MCP, or hook.
- **Bundle**: A named collection of assets. Acts as a "shopping list" for composing related assets.
- **Profile**: A bundle bound to a specific target. Profiles are what you actually install and run.
- **Source**: A location providing assets—a local directory, another tai config repo, or (future) a git remote.
- **Patch**: A set of search/replace edits applied to a file from a source you don't control, letting you keep a local delta without forking the source or shadowing the asset wholesale.
- **Config Repo**: A repository defining sources, bundles, and profiles. The entry point for tai.
- **Adapter**: Target-specific logic that knows how to build, install, and run a particular AI target.

### Asset Types

Assets come in two shapes based on how they're installed:

| Shape | Asset Types | How Defined | How Installed |
|-------|-------------|-------------|---------------|
| **File assets** | personas, delegates, skills, files | Files/directories in `assets/` | Copied to target paths |
| **Config assets** | statuslines, mcps, hooks | YAML in `assets/` | Merged into target JSON config files |

**File assets** are the traditional model: you define a file (or directory for personas, delegates, skills), and tai copies it to the appropriate location in the target's config directory. The `file` type copies content as-is without any transformation.

**Config assets** are entries that get merged into JSON configuration files. The target reads these JSON files at runtime. tai must read the existing JSON, merge in the new entries, and write it back - being careful to preserve entries it doesn't manage.

This distinction matters because:
1. **Installation logic differs**: copy vs. JSON merge
2. **Uninstallation differs**: delete file vs. remove JSON keys
3. **Conflict handling differs**: file exists vs. key exists
4. **Scope matters more**: config assets often have project/user/local scope options

### Design Principles

1. **Target-agnostic assets**: Assets are defined in tai's terminology (personas, delegates, etc.), not target-specific terms. Adapters handle translation during the build/install step.

2. **Additive installation**: tai never wipes target config directories. It only manages files it installed, leaving other files untouched.

3. **Version-controlled source of truth**: Your config repo is the canonical source. Target directories (like `~/.claude/`) are derived outputs that can be fully reconstructed.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        tai CLI                               │
│  (framework, adapters, core logic)                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ references
┌─────────────────────────────────────────────────────────────┐
│                    Active Config Repo                        │
│  (tai.yaml, profiles/, bundles/, assets/)                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ declares
┌─────────────────────────────────────────────────────────────┐
│                        Sources                               │
│  - Local directories                                         │
│  - Other tai config repos (inherits assets/)                │
│  - Git remotes (future)                                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ provides
┌─────────────────────────────────────────────────────────────┐
│                         Assets                               │
│  File: personas/, delegates/, skills/, files/               │
│  Config: statuslines/, mcps/, hooks/                        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ composed into
┌─────────────────────────────────────────────────────────────┐
│                        Bundles                               │
│  Named collections referencing assets from sources          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ combined into
┌─────────────────────────────────────────────────────────────┐
│                        Profiles                              │
│  Bundle(s) + target                                         │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ installed to (via adapter)
┌─────────────────────────────────────────────────────────────┐
│                    Target Config                             │
│  ~/.claude/, ~/.codex/, ~/.gemini/, ~/.config/opencode/     │
└─────────────────────────────────────────────────────────────┘
```

## Repository Structures

### tai CLI Repository (Public Framework)

```
tai/
├── cmd/
│   └── tai/
│       └── main.go
├── internal/
│   ├── config/           # Config repo loading/parsing
│   ├── adapter/          # Adapter interface and registry
│   │   ├── adapter.go    # Interface definition
│   │   ├── claude/       # Claude Code adapter
│   │   ├── codex/        # Codex adapter
│   │   ├── gemini/       # Gemini CLI adapter
│   │   ├── opencode/     # OpenCode adapter
│   │   └── shared.go     # Shared adapter helpers
│   ├── bundle/           # Bundle resolution
│   ├── profile/          # Profile management
│   ├── patch/            # Search/replace patches for imported content
│   └── install/          # Installation logic
├── go.mod
├── go.sum
└── README.md
```

### tai Config Repository (User's Personal/Work Config)

Assets are **target-agnostic**. The adapter handles translation to target-specific formats.

```
my-tai-config/
├── tai.yaml              # Config repo manifest
├── assets/               # Target-agnostic assets
│   ├── personas/         # Behavioral styles (→ output-styles in Claude)
│   │   ├── senior-swe.md
│   │   └── brainstormer.md
│   ├── delegates/        # Specialized helpers (→ agents in Claude)
│   │   ├── code-reviewer.md
│   │   └── pair-programmer.md
│   ├── skills/           # Procedural capabilities (→ skills in Claude)
│   │   └── council-participant/   # Skills follow agentskills.io standard
│   ├── files/            # Loose files copied as-is (→ target root or dest)
│   │   ├── CLAUDE.md
│   │   └── RFC2119.md
│   ├── statuslines/      # Status line configs (→ statusLine in settings.json)
│   │   └── detailed.yaml
│   ├── mcps/             # MCP server definitions (→ .mcp.json entries)
│   │   ├── github.yaml
│   │   └── postgres.yaml
│   └── hooks/            # Hook definitions (→ settings.json entries)
│       └── format-on-save.yaml
├── bundles/
│   ├── core.yaml
│   └── work-extras.yaml
├── patches/              # Local deltas on sources you don't own
│   └── shared-skills/    # One dir per source, named by its 'patches:' field
│       ├── add-tooling.yaml
│       └── add-tooling.blessed
└── profiles/
    ├── personal.yaml
    └── work.yaml
```

## Configuration Files

### tai.yaml (Config Repo Manifest)

```yaml
# my-tai-config/tai.yaml
version: 1

# Sources provide assets that bundles can reference.
# PRIORITY: Later sources override earlier ones when asset paths conflict.
# This ordering is intentional—list base/default sources first, overrides last.
sources:
  # Local assets in this repo
  # NOTE: The "local" source is IMPLICIT - tai automatically adds it if not
  # explicitly defined. It points to ./assets in the config repo. You only
  # need to declare it if you want to override the path.
  - name: local
    path: .

  # A directory of assets: personas/, skills/ ... at its top level
  - name: work-shared
    path: ~/work/ai-common

  # Another tai config repo: referenced as personal:assets/...
  - name: personal
    path: ~/personal-tai-config

  # A command source: an argv-list command materializes the source's entire
  # asset tree. Covers remote repos behind SSH/token auth (and any other
  # fetch shape) - keep credentials in env vars, not argv.
  - name: company
    type: command
    command:
      - sh
      - -c
      - git clone --depth=1 git@github.com:company/ai-standards.git "$TAI_DEST_DIR"
    timeout: 2m   # optional, default 30s

  # A url source: one file downloaded over http(s). For pulling a single
  # document off a raw host and, usually, patching it.
  - name: upstream-guide
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    patches: patches/upstream-guide/

# Default profile to use when none specified
default_profile: personal
```

#### A Source Is a Directory Root

`path` names a directory, and every reference is a literal path from it. tai
appends nothing and infers no layout, so `work:assets/skills/dredge` opens
exactly `<work path>/assets/skills/dredge`.

This is why a config repo's own assets are referenced as `assets/...`: the root
holds `assets/`, `bundles/`, and `profiles/` side by side, and pointing a source
at the root is what makes all three reachable. A directory that keeps asset type
directories at its top level takes no such prefix.

The earlier `type: tai-repo` appended `/assets` to the path. That was removed:
it put a repo's `bundles/` outside anything a reference could address, and it
made a path's meaning depend on a field elsewhere in the file. Sources are now
`local`, `command`, or `url`, and a config still declaring `tai-repo` is a load
error naming the fix.

Two directories are conventions tai knows, both for things it discovers rather
than resolves:

| Directory | Used for |
|-----------|----------|
| `<root>/bundles/` | Bundles this source offers for import as `source:name` |
| `<root>/<type-dir>/` and `<root>/assets/<type-dir>/` | What `tai list assets` enumerates |

Listing scans both asset locations because both layouts exist; there is no
ambiguity to settle, since each result prints as the literal path it is
referenced by. Bundles stay addressed by name rather than path because a
bundle's identity is its `name:` field, not its filename.

#### Command Sources

A `type: command` source runs its `command` (an argv list, executed directly
without a shell - wrap with `["sh", "-c", ...]` for shell features) with a
tai-managed staging directory exposed as `$TAI_DEST_DIR`. Whatever the command
writes there is atomically swapped into the source's cache directory
(`~/.cache/tai/sources/<repo-key>/<name>`, namespaced per config repo), which
is then treated exactly like a local source: assets are discovered from the
usual `personas/`, `skills/`, etc. layout the command produced.

Field rules:

| Field | Required | Notes |
|-------|----------|-------|
| `command` | Yes | Argv list; no empty elements |
| `timeout` | No | Duration string (e.g. `"60s"`, `"2m"`). Default 30s. |
| `path` | Rejected | Command sources have no on-disk path |

Environment for the command: the user's env is inherited (so `PATH`, `HOME`,
and credential env vars flow through; pre-existing `TAI_*` vars are stripped),
plus `TAI_DEST_DIR` (staging dir), `TAI_SOURCE` (source name),
`TAI_CONFIG_REPO` (absolute path of the active config repo), and
`TAI_VERSION`.

Freshness model:

- `tai install` / `tai reinstall` re-run every command source's fetch first,
  so installed content always reflects upstream. A fetch failure fails the
  install (fail-fast); the previous cache is left intact by the atomic swap.
- `tai refresh [source]` fetches without touching any target - the remedy for
  a fresh machine or after `--dry-run`.
- Read-only commands (`list`, `show`, `locate`, `status`) serve the cache
  without fetching. Resolving assets from a never-fetched source fails with a
  hint to run `tai refresh`.
- `--dry-run` never runs fetch commands (a preview must not have side
  effects); it resolves from the cache and prints what it would refresh.

Trust model matches command skill-includes: source commands run arbitrary
code with your environment. Treat a config repo that declares command sources
the same as running its code.

#### URL Sources

A `type: url` source downloads one file over http or https. It exists so that
patching a single remote document - a skill file off raw.githubusercontent.com,
a reference doc your team publishes - takes one declaration instead of a
`command` source wrapping `curl`.

The file is stored in the source's cache directory under its filename, so the
source is a **directory holding one file**:

```
~/.cache/tai/sources/<repo-key>/upstream-guide/
└── GUIDE.md
```

That shape is what makes the rest of tai work on it unchanged. A patched view
is built from it like any other source, `tai patch` operates on it like any
other source, and a skill reaches the file with an ordinary include:

```yaml
# assets/skills/mine/tai.yaml
include:
  - from: upstream-guide
    path: GUIDE.md
    dest: refs/guide.md
```

Because `resolveSourceInclude` reads through the source's content path, the
include receives the *patched* bytes. Nothing in the include pipeline knows
patches exist.

Field rules:

| Field | Required | Notes |
|-------|----------|-------|
| `url` | Yes | http or https. Scheme is validated when tai.yaml is parsed, not at fetch time. |
| `filename` | No | Name the file is stored under. Defaults to the URL's basename; required when the URL has none (e.g. `https://host/` or a query-only path). Must be a plain name. |
| `timeout` | No | Duration string. Default 30s. |
| `path` | Rejected | url sources have no on-disk path |
| `command` | Rejected | Use `type: command` for that |

A url source carries **no assets** - it has no `personas/`, `skills/`, or other
asset directory, so `tai list assets` shows nothing for it. That is the
intended configuration, not a mistake, and tai does not warn about it. The
source exists to be named by an include's `from:` and by `patches:`.

Freshness matches command sources exactly: fetched before every install,
fetched by `tai refresh`, served from cache by read-only commands, and never
fetched under `--dry-run`. An unreachable URL fails the install rather than
falling back to the cache - silently installing yesterday's content is the
failure mode patches exist to prevent. The atomic swap means a failed fetch
leaves the previous cache intact.

Requests carry `User-Agent: tai/<version>`, follow at most 10 redirects
(re-validating the scheme at each hop), and are capped at 10MB.

Unlike a command source, a url source runs no code. Its trust model is only
that of the content it downloads, which patches and `.blessed` snapshots exist
to keep honest.

### Bundle Definition

Bundles are "shopping lists" of assets. They can reference individual assets or include other bundles for composition.

```yaml
# my-tai-config/bundles/core.yaml
name: core
description: Essential assets for daily work

# Each asset requires an explicit type and path.
# Path uses source:path syntax (source defaults to "local" if omitted).
assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""           # Empty dest = target root
  - type: file
    path: assets/files/RFC2119.md
    dest: ""           # Empty dest = target root
```

```yaml
# my-tai-config/bundles/work-extras.yaml
name: work-extras
description: Additional assets from work sources

assets:
  - type: persona
    path: work-shared:personas/company-style.md
  - type: delegate
    path: work-shared:delegates/security-reviewer.md
```

```yaml
# my-tai-config/bundles/full-setup.yaml
name: full-setup
description: Comprehensive setup combining multiple bundles

# Bundles can include other bundles for composition
bundles:
  - core
  - work-extras

# Plus additional individual assets
assets:
  - type: persona
    path: assets/personas/brainstormer.md
```

#### Importing Bundles From Other Sources

A `bundles:` entry may name a source, importing that bundle from the source's
`bundles/` directory. An entry is a bare name or a mapping; the mapping form
exists for `exclude`.

```yaml
bundles:
  - core                        # This repo's bundles/core.yaml
  - work:work-tools             # The work source's bundles/work-tools.yaml
  - name: work:extras
    exclude:
      - assets/skills/beagle
```

The problem this solves is drift. Without imports, sharing a bundle across
repos means restating it in each consumer, and a restatement is a snapshot: when
the origin gains an asset, the copy stays valid and stays wrong, with nothing to
notice. An import is read at install time, so it cannot go stale.

Resolution rules:

| Reference inside a bundle | Resolves against |
|---------------------------|------------------|
| Unprefixed asset path | The bundle's own repo |
| Unprefixed included bundle name | The bundle's own repo |
| `source:`-prefixed anything | The **consumer's** `tai.yaml` |

The first two are what let an imported bundle keep working: its author wrote
`assets/skills/dredge` meaning their repo, and it has to keep meaning that once
someone else imports it, even if the consumer holds a file at the same path.

Prefixes are the exception because the consumer's `tai.yaml` is the only one tai
loads. An imported bundle reaching into a third source therefore requires the
consumer to declare that source under the same name. tai reports a missing one
by name, quoting how the origin repo defines it when that repo's `tai.yaml` is
readable - what to add, not just what is absent.

Cycle detection keys on the qualified name, so a local `core` and a `work:core`
are distinct bundles.

##### Exclusions

Two scopes drop an asset, answering different questions:

| Where | Means |
|-------|-------|
| `exclude` on a `bundles:` entry | Not from this bundle |
| `exclude` on a source in `tai.yaml` | Never from this source at all |

Per-import exclusions match the same root-relative paths as a source's, and
apply to the import's fully resolved asset set, so they reach assets that
arrived through a nested include: what a user is expressing is "this must not
land on my machine", not "not at this nesting level".

A pattern matching nothing warns rather than failing. Upstream may have deleted
the asset, which already satisfies the intent, and failing a consumer's install
over someone else's deletion costs more than a stale line does. This departs
from the fail-loud rule patches follow because the failure modes invert: a
silently dropped patch reintroduces rejected content, while a redundant
exclusion drops nothing.

A source-level exclusion behaves differently depending on who wrote the
reference. One the user wrote is a hard error - a contradiction inside their own
config. One an imported bundle wrote is filtered and logged, since the user
never made that reference and never gets to edit it. Without the split, source
exclusions and bundle imports could not be used together at all.

### Profile Definition

Profiles bind bundles to a specific target. Bundle order matters: **later bundles override earlier ones** when asset paths conflict. tai logs an info message when such overrides occur.

```yaml
# my-tai-config/profiles/work.yaml
name: work
description: Full work setup
target: claude

# Bundles to include (resolved in order, later overrides earlier on conflict)
# Example: if both 'core' and 'work-extras' have personas/senior-swe.md,
#          work-extras version wins and tai logs the override
bundles:
  - core
  - work-extras

# Can also include individual assets directly
assets:
  - type: persona
    path: assets/personas/brainstormer.md

# Optional: adapter-specific settings (passed to adapter during install/run)
options:
  # Example Claude-specific options (future use)
  # dangerously_skip_permissions: false

# Optional: installation scope for config assets (default: project)
# Affects where MCPs and hooks are installed
scope: project  # or "user" or "local"

# Optional: argv list run after this profile installs successfully.
# For notifications, restarting tools, chaining scripts, or post-processing
# installed files.
post_install:
  - sh
  - -c
  - osascript -e 'display notification "tai installed"'
post_install_timeout: 1m  # optional, default 30s
```

#### Post-Install Hooks

`post_install` runs only after the install completes (never on dry-run, never
when the install itself failed). The command sees the inherited user env plus
`TAI_TARGET`, `TAI_TARGET_DIR`, `TAI_PROFILE`, `TAI_MANIFEST_DIR`, and
`TAI_VERSION` - all always set, so `set -u` scripts are safe.
`TAI_TARGET_DIR` is where the target's files were installed (e.g.
`~/.claude`), so hooks can find what they need to post-process.

A failing hook does **not** roll the install back: files and manifest are
already in place, and side effects can't be un-run (the same model as dpkg,
Homebrew, and npm post-install scripts). tai reports the install as
succeeded, prints the hook failure with a tail of its stderr, and exits
nonzero so scripts and CI notice. `tai reinstall` continues across remaining
targets and aggregates hook failures into its final exit code.

##### Hooks May Modify Managed Files

Hooks are allowed to post-process files tai installed - reformatting them, or
injecting machine-local config a source asset can't carry. Conflict detection
compares each file against the checksum recorded when tai wrote it, so left
alone a hook's own edit would make that file conflict on *every* subsequent
install, forever.

So after a hook **succeeds**, tai re-hashes the files that install wrote and
records the new checksums as the baseline. It reports which managed files the
hook changed:

```
post_install hook modified 1 managed file(s) (manifest baseline updated):
    ~ CLAUDE.md
```

Note what this trades away: tai no longer flags hook-caused edits to managed
files, so a buggy hook's changes are accepted rather than reported as
conflicts. That report line is the signal you get. The damage stays transient -
every install re-renders the file from source before the hook runs again.

Two deliberate carve-outs:

- **A failed hook does not rebaseline.** A hook that edited a file and then
  failed leaves a partial edit, and that conflict is worth surfacing.
- **Files skipped via `--skip-conflicts` are never rebaselined.** They keep
  their stale checksum so they keep conflicting until you resolve them;
  baselining them would silently discard a decision you made.

Hooks that edit managed files should be idempotent: tai overwrites the file
from source on every install, so the hook re-applies its change each run.

### File Asset Formats

#### Tai Frontmatter (Personas, Delegates, Skills)

Personas, delegates, and skills use YAML frontmatter to declare metadata. During installation, tai strips its own frontmatter and the adapter renders target-specific frontmatter. The `file` type is different—it copies content as-is without any frontmatter transformation.

```markdown
---
name: Senior SWE
description: Experienced software engineer co-owner role.
---

# Senior SWE

You are a senior software engineer...
```

**Frontmatter fields:**

| Field | Personas | Delegates | Skills |
|-------|----------|-----------|--------|
| `name` | required | required | required |
| `description` | optional | optional | optional |
| `tools` | — | optional | optional |
| `model` | — | optional | optional |
| `depends` | optional | optional | optional |
| `native` | optional | optional | optional |

- `name`: Human-readable display name.
- `description`: Short description of what this asset does.
- `tools`: Comma-separated list of tools the delegate/skill may use.
- `model`: Model to use for this delegate/skill (e.g., `haiku`, `sonnet`, `opus`).
- `depends`: List of asset dependencies (see [Asset Dependencies](#asset-dependencies) below).
- `native`: Per-target fields passed through verbatim (see [Per-Target Frontmatter](#per-target-frontmatter-native) below).

Any other field is dropped with a warning naming the field and pointing at `native`. The schema version is tracked at the repo level (`version: 1` in `tai.yaml`), not per-file; a leftover `tai-schema-version` warns that it does nothing.

**Note:** The `file` type has no frontmatter requirements—content is copied exactly as-is.

#### Per-Target Frontmatter (`native`)

Targets read frontmatter fields tai has no word for: Claude alone reads `disable-model-invocation`, `user-invocable`, `allowed-tools`, `effort`, `argument-hint`, `paths` and `hooks` from a SKILL.md. Those go under `native`, keyed by target name, and are merged verbatim over the frontmatter the adapter generated:

```markdown
---
name: parallel-work
description: >
  Use when several agent sessions build features simultaneously...
native:
  claude:
    disable-model-invocation: true
    allowed-tools: [Read, Bash]
---
```

Installing to Claude writes `name`, `description`, `disable-model-invocation` and `allowed-tools`. Installing the same asset to any other target writes only `name` and `description`, because `native.claude` is not addressed to them.

This is the same escape hatch hooks and MCP servers use, spelled the same way — see `native` under [Hook Definition](#hook-definition) and [MCP Definition](#mcp-definition).

**What belongs here.** A field only one target understands. A concept several targets share can be promoted to a tai field instead, so it is written once and each adapter translates it — that is what `tools` and `model` already are. The two rules meet at "how many targets have this", not at "how obscure is it".

**A native field wins** over the tai field of the same name, keeping the position tai gave it. `native: {claude: {description: ...}}` replaces the description on Claude and leaves every other target's alone.

**A misspelled target name warns.** `native.cluade` reaches nothing, and silently doing nothing is the failure this feature exists to remove, so tai names the targets it knows.

**Values are not interpreted.** Lists, maps and booleans render as YAML, so a nested field like Claude's `hooks` survives its shape.

#### Adapter Transformation (Claude)

| Asset Type | Installed frontmatter | Notes |
|------------|----------------------|-------|
| Personas | `name`, `description` | Claude output-style format |
| Delegates | `name`, `description`, `tools`, `model` | Claude agent format |
| Skills | `name`, `description`, `tools`, `model` | Same as delegates (SKILL.md in directories) |
| Files | (none—content unchanged) | Copied byte-for-byte to `dest` location |

Every row except `file` also gets that target's `native` entry merged on top.

#### Asset Dependencies

Assets can declare dependencies on other assets using the `depends` field. When an asset is included in a bundle or profile, its dependencies are automatically pulled in.

```markdown
---
name: Code Review
description: Request a code review with specialized reviewers
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - delegates/performance-reviewer.md
  - skills/council-participant
---

# Code Review Skill
...
```

**Syntax:**

Each dependency is a path string in the format `type-dir/name` or `source:type-dir/name`:

```yaml
depends:
  - delegates/bug-hunter.md              # from local source
  - skills/council-participant           # directory-based skill
  - other-source:delegates/helper.md     # from named source
```

**Resolution rules:**

1. **Depth-first ordering**: Dependencies are resolved depth-first, so transitive dependencies appear before the assets that need them.
2. **Deduplication**: If the same asset appears multiple times (directly or as a dependency), it's included once using "later wins" semantics.
3. **Explicit wins**: Assets explicitly included in a bundle/profile override auto-included dependencies.
4. **Cycle detection**: Circular dependencies (A → B → A) are detected and cause installation to fail with a clear error message.
5. **Stripped on install**: The `depends` field is tai metadata—it's consumed during resolution and never written to the target.

**Use case:** A skill like `code-review` that uses multiple specialized delegates doesn't require users to manually include each delegate. They include the skill, and the delegates come along automatically.

**Note:** The `file` type doesn't support dependencies (frontmatter is not parsed for file assets).

#### The `file` Type

The `file` type copies content exactly as-is—no frontmatter parsing, no transformation. Use it for loose files that don't belong to a specific asset category:

```yaml
assets:
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""              # Required: empty = target root (~/.claude/)

  - type: file
    path: assets/files/config.json
    dest: "config"        # Goes to ~/.claude/config/config.json
```

**Key points:**
- `dest` field is **required** for file type—forces explicit intent about where the file lands
- `dest` is only valid for `file` type (rejected on other types)
- Empty `dest` (`""`) means target root directory
- Content is preserved byte-for-byte (frontmatter, if any, is NOT stripped)

#### Directory Assets

Personas, delegates, and skills can all be directories with a main file plus supporting files. This is useful when an asset needs additional resources (scripts, references, examples).

| Asset Type | Main File | Directory Structure |
|------------|-----------|---------------------|
| Persona | `PERSONA.md` | `personas/my-persona/PERSONA.md` + supporting files |
| Delegate | `DELEGATE.md` | `delegates/my-delegate/DELEGATE.md` + supporting files |
| Skill | `SKILL.md` | `skills/my-skill/SKILL.md` + supporting files |

During installation:
1. The main file (PERSONA.md, DELEGATE.md, or SKILL.md) is transformed with target-specific frontmatter
2. Supporting files are copied as-is, preserving directory structure

**Example persona directory:**
```
assets/personas/senior-swe/
├── PERSONA.md              # Main file (transformed)
└── references/
    └── coding-standards.md # Supporting file (copied as-is)
```

Installs to:
```
~/.claude/output-styles/senior-swe/
├── senior-swe.md           # Main file renamed to match directory
└── references/
    └── coding-standards.md
```

Skills follow the agentskills.io standard and have additional features like includes. See the [Skill Includes](#skill-includes) section.

### Config Asset Definitions

Config assets (status lines, MCPs, and hooks) are defined as YAML files that describe entries to be merged into target configuration files.

#### StatusLine Definition

Status line assets control the appearance and behavior of the AI tool's status bar. They can be defined in **tai format** (target-agnostic) or **native format** (target-specific passthrough).

**Tai format** (recommended for portability):

```yaml
# my-tai-config/assets/statuslines/detailed.yaml
level: detailed              # minimal | standard | detailed | hidden | custom
show:                        # Elements to display (when level != hidden/custom)
  - cost                     # Session/total cost
  - model                    # Current model name
  - tokens                   # Token usage
  - latency                  # Response latency
```

```yaml
# my-tai-config/assets/statuslines/custom.yaml
level: custom                # Custom command mode
command: ~/.scripts/statusline.sh
padding: 2                   # Optional padding value
```

**Native format** (passed through to target as-is):

If the YAML file does not contain a `level` field, it's treated as native format and passed through to the target without transformation. This allows using target-specific options that tai doesn't abstract.

```yaml
# Native Claude format - passed through as-is
style: detailed
showCost: true
showModel: true
```

**StatusLine fields (tai format):**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `level` | string | Yes | `minimal`, `standard`, `detailed`, `hidden`, or `custom` |
| `show` | list | No | Elements to display: `cost`, `model`, `tokens`, `latency`, `context` |
| `command` | string | custom only | Path to custom status line command |
| `padding` | int | No | Padding value (only for `level: custom`) |

**Validation rules:**
- `level: custom` requires `command`, disallows `show`
- `level: hidden` disallows both `show` and `command`
- `level: minimal/standard/detailed` disallows `command`

**Singleton behavior:** Only one status line can be active. When multiple status line assets are referenced in a profile, later ones override earlier ones (logged as info).

**Adapter transformation (Claude):**

| tai field | Claude equivalent |
|-----------|------------------|
| `level: detailed` | `style: "detailed"` |
| `level: minimal` | `style: "minimal"` |
| `level: hidden` | `style: "hidden"` |
| `level: custom` | `type: "command"` |
| `show: [cost]` | `showCost: true` |
| `show: [model]` | `showModel: true` |
| `show: [tokens]` | `showTokens: true` |
| `show: [latency]` | `showLatency: true` |
| `command: ...` | `command: ...` |
| `padding: N` | `padding: N` |

#### MCP Definition

```yaml
# my-tai-config/assets/mcps/github.yaml
# The filename (minus extension) becomes the server name

transport: stdio               # Required: "stdio" or "http"
command: npx                   # Required for stdio: executable to run
args:                          # Optional: command arguments
  - "-y"
  - "@modelcontextprotocol/server-github"
env:                           # Optional: environment variables
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

```yaml
# my-tai-config/assets/mcps/internal-api.yaml
# HTTP-based MCP server

transport: http
url: https://api.internal.company.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
```

**MCP fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `transport` | string | Yes | `stdio` (local command) or `http` (remote server) |
| `command` | string | stdio | Executable to run |
| `args` | list | No | Arguments to the command |
| `env` | map | No | Environment variables; `${VAR}` references are rewritten per target |
| `cwd` | string | No | Working directory; dropped with a warning on Claude, which has none |
| `url` | string | http | Server endpoint |
| `headers` | map | No | HTTP headers |
| `enabled` | bool | No | Defaults to true; a disabled server is not installed at all |
| `description` | string | No | For humans; installed only where the target has a field for it |
| `native` | map | No | Per-target fields merged verbatim over the generated entry |

The server's name is its filename with the extension stripped.

Only `stdio` and `http` are abstracted: they are the two transports every
target supports. SSE and WebSocket exist on some and not others, so rather than
a portable-looking option that silently fails on half the targets, they are left
to `native`.

**Environment references** are written `${VAR}` and rewritten per target -
OpenCode wants `{env:VAR}`. tai never reads the variable, so the secret stays in
the environment instead of being baked into an installed file.

**The shapes differ even though the protocol does not.** Claude and Gemini key
servers under `mcpServers`; OpenCode uses `mcp`, folds the command and its
arguments into one argv array, spells the environment `environment`, and
requires an explicit `local`/`remote` type. Each adapter renders its own shape.

| tai | Claude | Gemini | OpenCode |
|-----|--------|--------|----------|
| key | `mcpServers` | `mcpServers` | `mcp` |
| command + args | separate | separate | one array |
| env | `env` | `env` | `environment` |
| cwd | unsupported | `cwd` | `cwd` |

**Scope** picks the file:

| Target | `user` | `project` | `local` |
|--------|--------|-----------|---------|
| Claude | `~/.claude.json` | `.mcp.json` | `~/.claude.json`, under `projects.<path>` |
| Codex | `~/.codex/config.toml` | `.codex/config.toml` | - |
| Gemini | `~/.gemini/settings.json` | `.gemini/settings.json` | - |
| OpenCode | `~/.config/opencode/opencode.json` | `opencode.json` | `opencode.json` |

Claude's two project scopes differ in who else sees the server. Project scope
writes `.mcp.json`, meant to be committed and shared with the repo. Local scope
is the private counterpart, so Claude keeps it under the project's own entry in
`~/.claude.json` - keyed by absolute path, beside that project's trust decision
- rather than in a file in the repo. Writing a `.claude.json` in the project
directory instead would produce a file Claude never reads, so the server would
silently never appear.

`~/.claude.json` also holds account identity, per-project trust decisions, and
a good deal of session state. tai merges into the parsed document, touches only
its own entries, and preserves numbers as literals so nothing it does not
manage is reformatted. Project scope writes `.mcp.json`, a dedicated single-key
file, for anyone who would rather keep tai out of that file entirely.

**Codex asks first.** It has no dedicated MCP config file - servers live in
`~/.codex/config.toml` beside model settings and project trust levels - and
rewriting that file loses comments and key order, since no Go TOML library
round-trips them. tai explains the cost, remembers the answer per machine in
`~/.config/tai/preferences.yaml`, and declines when there is no terminal to ask
in (`--allow-config-rewrite` overrides).

A dry run never asks. Prompting there would put a real, possibly remembered
answer behind a command that promises to change nothing, and a later install
would silently reuse it; the preview reports what it would have asked instead.
Declining leaves servers from an earlier install exactly as they are - it is an
answer about a write, not a request to remove anything.

Codex also does no string interpolation. It takes environment variable *names*
in dedicated fields, so a whole-value reference translates and nothing else
does:

| tai | Codex |
|-----|-------|
| `env: {TOKEN: "${TOKEN}"}` | `env_vars = ['TOKEN']` |
| `headers: {Authorization: "Bearer ${KEY}"}` | `bearer_token_env_var = 'KEY'` |
| `headers: {X-Key: "${KEY}"}` | `env_http_headers = {X-Key = 'KEY'}` |

A reference Codex cannot express - an env key that differs from the variable it
reads, or one embedded in a longer string - is an error, not a literal `${VAR}`
written into a config that will never expand it.

#### Hook Definition

A hook runs a command when something happens in the target.

```yaml
# my-tai-config/assets/hooks/format-on-save.yaml

description: Format files after they are edited   # Optional, for humans
on: post_tool_use          # Required: canonical tai event
tools: [edit, write]       # Optional: narrows a tool event; empty matches all
run: prettier --write "$FILE"   # Required: the command
timeout: 30s               # Optional: duration; converted per target
```

```yaml
# my-tai-config/assets/hooks/security-check.yaml

on: pre_tool_use
tools: [shell]
run: ~/scripts/check-dangerous-commands.sh
```

**Hook fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `description` | string | No | For humans reading the config repo; targets never see it |
| `on` | string | Yes | Canonical tai event that triggers the hook |
| `tools` | list | No | Canonical tool names; empty matches every tool |
| `run` | string | Yes | Command to execute |
| `timeout` | string | No | Duration (`30s`, `2m`); the target's own default when unset |
| `match` | map | No | Raw per-target matcher, keyed by target name |
| `native` | map | No | Per-target fields merged verbatim over the generated handler |

The hook's identity is its filename with the extension stripped, so
`assets/hooks/format-on-save.yaml` installs a hook named `format-on-save`.

**Canonical hook events.** tai names events itself rather than borrowing a
target's vocabulary, because the targets disagree: Claude and Codex call an
event `PreToolUse` where Gemini calls it `BeforeTool`, and Gemini renames every
other event too.

| tai event | When it fires | Claude | Codex | Gemini |
|-----------|---------------|--------|-------|--------|
| `session_start` | A session begins | SessionStart | SessionStart | SessionStart |
| `session_end` | A session ends | SessionEnd | - | SessionEnd |
| `user_prompt_submit` | A prompt is submitted | UserPromptSubmit | UserPromptSubmit | BeforeAgent |
| `stop` | The agent finishes responding | Stop | Stop | AfterAgent |
| `pre_tool_use` | Before a tool runs | PreToolUse | PreToolUse | BeforeTool |
| `post_tool_use` | After a tool runs | PostToolUse | PostToolUse | AfterTool |
| `permission_request` | A permission is requested | PermissionRequest | PermissionRequest | - |
| `subagent_start` | A subagent starts | SubagentStart | SubagentStart | - |
| `subagent_stop` | A subagent finishes | SubagentStop | SubagentStop | - |
| `pre_compact` | Before context is compacted | PreCompact | PreCompact | PreCompress |
| `post_compact` | After context is compacted | PostCompact | PostCompact | - |
| `notification` | The target sends a notification | Notification | - | Notification |

A `-` means the target has no equivalent. A hook using such an event is skipped
on that target with a warning, so one asset can be shared by a profile that
installs to several targets.

**Canonical tool names.** `read`, `write`, `edit`, and `shell`, translated per
target - Claude's `Edit` is Gemini's `replace`, Claude's `Bash` is Gemini's
`run_shell_command`. A tool a target has no name for is an error rather than a
silent drop: dropping it would widen the matcher to every tool, so a hook meant
to guard edits would start firing on reads.

**Escape hatches.** Anything outside the canonical vocabulary - MCP tool
patterns, target-specific handler options - goes in `match` or `native`, keyed
by target name:

```yaml
on: pre_tool_use
run: guard.sh
match:
  claude: "mcp__memory__.*"     # overrides `tools` for Claude
native:
  claude: { async: true }       # merged into Claude's handler object
```

**Timeouts** are written as durations and converted per target, because Claude
and Codex expect seconds while Gemini expects milliseconds. Getting that wrong
by hand fails silently - a 30-second timeout becomes 30 milliseconds.

**Target notes.**

- **OpenCode** has no declarative hooks. Its plugin API is JS modules that
  block by throwing, with no matcher layer and no I/O contract, so hook assets
  are skipped with a warning rather than partially emulated.
- **Codex** will not run a hook until it is trusted, and trust is keyed on a
  hash of the definition, so every change re-arms the prompt. tai says so after
  installing; run `/hooks` in Codex to review them.
- **Codex hooks** go to a dedicated `hooks.json`, never `config.toml`, so tai
  never rewrites the file holding model settings and project trust decisions.

## CLI Interface

### Global State

tai stores its global configuration in `~/.config/tai/` and caches remote sources in `~/.cache/tai/`:

```
~/.config/tai/
├── config.yaml           # CLI settings
└── manifests/            # Installation manifests by target
    └── claude.yaml

~/.cache/tai/
└── sources/              # Cached remote sources (git repos)
    └── github.com/
        └── company/
            └── ai-standards/
                └── <ref>/
```

```yaml
# ~/.config/tai/config.yaml
active_config: /Users/amterp/work-tai-config
```

### Commands

```bash
# Configuration
tai config set <path>           # Set active config repo
tai config show                 # Show current config repo and settings

# Discovery
tai list sources                # List sources in active config
tai list bundles                # List available bundles
tai list profiles               # List available profiles
tai list assets [--source X]    # List assets, optionally filtered by source

# Inspection
tai show profile <name>         # Show resolved assets for a profile
tai show bundle <name>          # Show assets in a bundle
tai locate <asset-path>         # Print full filesystem path to an asset

# Installation
tai install <profile>           # Install profile to target config
tai install <profile> --dry-run # Preview what would be installed (no changes)
tai install <profile> --diff    # Show diffs for conflicting files
tai uninstall <target>          # Remove tai-managed files for a target
tai status [target]             # Show what's currently installed

# Running (convenience wrapper)
tai run claude [args...]        # Run claude with tai config applied
tai run opencode [args...]      # Run opencode with tai config applied

# Command Source Management
tai refresh                     # Fetch all command sources into the cache
tai refresh <source-name>       # Fetch a specific command source

# Patches (local deltas on sources you don't own)
tai patch list                  # List patches and whether they still apply
tai patch diff <name>           # Show what changed upstream since blessing
tai patch bless <name>|--all    # Record current upstream content as reviewed
tai patch workspace <source>    # Editable copy for authoring or repairing
tai patch regenerate <name>     # Rewrite a patch from your workspace edits

# Build (Future)
tai build <profile>             # Build/compile assets without installing
```

### Example Session

```bash
# Initial setup
tai config set ~/my-tai-config
tai list profiles
# Output:
#   personal  claude  Core assets for personal use
#   work      claude  Full work setup

# Preview installation
tai install work --dry-run
# Output:
#   Would install profile 'work' for claude...
#   + personas/senior-swe.md (new)
#   + personas/company-style.md (new)
#   + delegates/code-reviewer.md (new)
#   ...
#   8 files would be added, 0 updated, 0 removed

# Install a profile
tai install work
# Output:
#   Installing profile 'work' for claude...
#   Resolving bundles: core, work-extras
#   ℹ Override: personas/senior-swe.md (core → work-extras)
#   Installing 8 assets to ~/.claude/
#   ✓ output-styles/senior-swe.md
#   ✓ output-styles/company-style.md
#   ✓ agents/code-reviewer.md
#   ...
#   Manifest written to ~/.config/tai/manifests/claude.yaml

# Check status
tai status claude
# Output:
#   Profile: work
#   Installed: 2024-01-21T14:30:00Z
#   Assets: 8 files

# Find where an asset lives
tai locate personas/senior-swe.md
# Output:
#   /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Run claude with tai config
tai run claude
# Equivalent to: claude (tai config already installed to ~/.claude/)
```

## Adapter Interface

Each supported target has an adapter implementing:

```go
type Adapter interface {
    // Name returns the adapter name (e.g., "claude", "opencode")
    Name() string

    // AssetMapping returns how tai asset types map to target paths
    // e.g., "personas" → "output-styles", "delegates" → "agents"
    AssetMapping() map[string]string

    // SupportedAssetTypes returns which asset types this adapter supports
    // Assets of an unsupported type are skipped with a warning, so one
    // profile can carry assets for several targets
    SupportedAssetTypes() []string

    // ValidateAssets checks that assets are valid for this target
    ValidateAssets(assets []Asset) error

    // TargetDir returns where this target's config lives
    TargetDir() string  // e.g., ~/.claude

    // Install copies/links assets to the target's config directory
    // Returns a manifest of what was installed
    Install(assets []ResolvedAsset, opts InstallOptions) (*Manifest, error)

    // Uninstall removes tai-managed files using the manifest
    Uninstall(manifest *Manifest) error

    // Run invokes the target (optional, for wrapper mode)
    Run(args []string) error

    // --- Config asset methods (for mcps, hooks, etc.) ---

    // ConfigAssetTypes returns which asset types are config-based (vs file-based)
    ConfigAssetTypes() []string  // e.g., ["mcps", "hooks"]

    // ConfigFilePath returns the target JSON file for a config asset type and scope
    ConfigFilePath(assetType string, scope string) string

    // MergeConfigAsset merges a config asset into existing JSON content
    MergeConfigAsset(existing []byte, asset ConfigAsset) ([]byte, error)

    // RemoveConfigAsset removes a config asset from existing JSON content
    RemoveConfigAsset(existing []byte, assetID ConfigAssetID) ([]byte, error)
}
```

### Claude Adapter Specifics

The Claude adapter:
- Targets `~/.claude/`
- Maps tai terminology to Claude terminology:
  - `personas/` → `output-styles/`
  - `delegates/` → `agents/`
  - `skills/` → `skills/`
  - `files/` → location specified by `dest` field (empty = root level)
- Installation is file copy (not symlink for reliability)
- May use `--add-dir` for run-time config in future

**Config asset handling:**

| Asset Type | Scope | Target File |
|------------|-------|-------------|
| `statuslines` | user | `~/.claude/settings.json` |
| `statuslines` | project | `.claude/settings.json` |
| `mcps` | project | `.mcp.json` (project root) |
| `mcps` | user | `~/.claude.json` |
| `mcps` | local | `~/.claude.json`, under `projects.<path>.mcpServers` |
| `hooks` | project | `.claude/settings.json` |
| `hooks` | user | `~/.claude/settings.json` |
| `hooks` | local | `.claude/settings.local.json` |

**StatusLine installation:** The `statusLine` key in the target JSON file is set to the transformed config:
```json
{
  "statusLine": {
    "style": "detailed",
    "showCost": true,
    "showModel": true
  }
}
```

### Codex Adapter Specifics

The Codex adapter:
- Targets `$CODEX_HOME` if set, otherwise `~/.codex/`
- Maps tai terminology to Codex-oriented paths:
  - `personas/` → `skills/` (mapped to skills since Codex has no persona concept; single-file personas are wrapped as `skills/<name>/SKILL.md`)
  - `delegates/` → `delegates/` (passive reference files)
  - `skills/` → `skills/`
  - `files/` → location specified by `dest` field (empty = root level)
- Installs files additively like other adapters
- Emits a warning for each `persona` asset noting the persona-to-skill remapping, and for each `delegate` asset because Codex does not natively consume delegates as first-class config entries
- Uses `target_agents_file: AGENTS.md`

`statusline` assets are currently unsupported for Codex and are skipped with a warning.

### Gemini CLI Adapter Specifics

The Gemini adapter:
- Targets `$GEMINI_HOME` if set, otherwise `~/.gemini/`
- Maps tai terminology to Gemini-oriented paths:
  - `personas/` -> `skills/` (mapped to skills since Gemini has no persona concept)
  - `delegates/` -> unsupported (skipped entirely with warning)
  - `skills/` -> `skills/`
  - `files/` -> location specified by `dest` field (empty = root level)
- Personas are installed as skills so their prompt content is still accessible; a warning is emitted explaining the mapping
- Delegates are skipped entirely because Gemini CLI has no user-defined sub-agent system
- Uses `target_agents_file: GEMINI.md`

`statusline` assets are unsupported for Gemini CLI and are skipped with a warning.

### OpenCode Adapter Specifics

The OpenCode adapter:
- Targets `$XDG_CONFIG_HOME/opencode` if `XDG_CONFIG_HOME` is set, otherwise `~/.config/opencode/`
- Maps tai terminology to OpenCode-oriented paths:
  - `personas/` -> `agents/` (description only; no name since OpenCode uses filename)
  - `delegates/` -> `agents/` (description + `mode: subagent` + model; no name or tools)
  - `skills/` -> `skills/`
  - `files/` -> location specified by `dest` field (empty = root level)
- Both personas and delegates map to `agents/` - OpenCode unifies these into a single agent concept, distinguished by the `mode` field
- If a persona or delegate has no `description`, falls back to the `name` value
- Tools are omitted because OpenCode uses a structurally incompatible format (boolean map vs comma-separated string)
- No install warnings for personas/delegates (OpenCode actively consumes agents)
- Uses `target_agents_file: AGENTS.md`

`statusline` assets are unsupported for OpenCode and are skipped with a warning.

**MCP installation:** Entries are added to the `mcpServers` key in the target JSON file:
```json
{
  "mcpServers": {
    "github": { "type": "stdio", "command": "npx", ... }
  }
}
```

**Hook installation:** Entries are added to the `hooks.{EventName}` array in the target JSON file:
```json
{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Edit|Write", "hooks": [{ "type": "command", "command": "..." }] }
    ]
  }
}
```

## Installation Manifest

```yaml
# ~/.config/tai/manifests/claude.yaml
target: claude
profile: work
config_repo: /Users/amterp/work-tai-config
installed_at: 2024-01-21T14:30:00Z

# File assets: tracked by path
files:
  - path: output-styles/senior-swe.md
    source: local
    source_path: /Users/amterp/my-tai-config/assets/personas/senior-swe.md
    checksum: sha256:abc123...

  - path: output-styles/company-style.md
    source: work-shared
    source_path: /Users/amterp/work/ai-common/personas/company-style.md
    checksum: sha256:def456...

  # Files using {{existing}} templating track their backup
  - path: CLAUDE.md
    source: local
    source_path: /Users/amterp/my-tai-config/assets/files/CLAUDE.md
    checksum: sha256:789ghi...
    backup_path: /Users/amterp/.claude/CLAUDE.md.tai-backup
    is_templated: true

# Config assets: tracked by type, name, scope, and the edits that installed them
config_entries:
  - type: statusline
    name: detailed
    source: local
    source_path: /Users/amterp/my-tai-config/assets/statuslines/detailed.yaml
    settings_file: ~/.claude/settings.json
    scope: user
    checksum: sha256:status123...
    edits:
      - file: /Users/amterp/.claude/settings.json
        key_path: [statusLine]
        mode: replace_key
        checksum: sha256:abc...

  - type: hook
    name: format-on-save
    source: local
    source_path: /Users/amterp/my-tai-config/assets/hooks/format-on-save.yaml
    settings_file: /Users/amterp/.claude/settings.json
    scope: user
    checksum: sha256:hook456...
    edits:
      - file: /Users/amterp/.claude/settings.json
        key_path: [hooks, PostToolUse]
        mode: append_to_array
        checksum: sha256:def...
```

`edits` is what makes removal surgical. Each records where an entry went and a
checksum of the entry itself, which is enough to find it again in a shared
array and enough to notice the user has since edited it - without the manifest
carrying a second copy of every installed entry.

Merge modes:

| Mode | Install | Uninstall |
|------|---------|-----------|
| `replace_key` | Set the key outright | Delete the key |
| `upsert_map_entry` | Set one key inside an object | Delete that one key |
| `append_to_array` | Append, leaving existing elements | Delete the single matching element |

Only `replace_key` claims a whole key, and only where tai owns it - a target
displays one status line. Hooks and MCP servers share their containers with
entries tai did not write.

The manifest enables:
- `tai uninstall`: Remove exactly what tai installed
- `tai install`: Diff against previous install, add new, remove stale
- `tai status`: Show what's installed and from where
- Conflict detection: Warn if file was modified externally

## Installation Behavior

### Additive, Non-Destructive

tai installation is **additive**:
- Only touches files it manages (tracked in manifest)
- Preserves files not managed by tai
- Never wipes the entire target directory

### Re-installation

When running `tai install <profile>` with an existing installation:
1. Load current manifest
2. Resolve new profile's assets:
   a. Read source file and apply template substitution
   b. Parse and validate tai frontmatter (all file types) — separates metadata from body
   c. Run adapter's `TransformContent` — renders target-specific frontmatter (or strips it for docs)
   d. Compute checksum on final transformed content
3. Compare:
   - Files to add (in new, not in old)
   - Files to remove (in old, not in new)
   - Files to update (in both, checksum changed)
4. Apply changes
5. Update manifest

### Conflict Handling

| Scenario | Manifest Entry? | File Matches Checksum? | Action |
|----------|-----------------|------------------------|--------|
| New file | No | N/A | Install safely |
| Managed update | Yes | Yes | Overwrite with new version |
| User divergence | Yes | No | **Warning**: Require `--force` |
| Unmanaged conflict | No | N/A | **Error**: File exists but tai doesn't own it |

Options:
- `--diff`: Show unified diffs for conflicting files
- `--force`: Overwrite conflicting files
- `--skip-conflicts`: Leave conflicting files as-is
- `--dry-run`: Preview changes without applying

**Target-path collisions:** Two distinct assets can map to the same install
path for a given target - most commonly an OpenCode persona and delegate of the
same name, which both map to `agents/<name>.md`. tai resolves these with the
same "later wins" rule used for bundle/source conflicts: the last asset in
resolution order is installed and recorded, the earlier one is shadowed (logged
as an override). This keeps reinstall idempotent - a collision that produced two
manifest entries for one path would otherwise report a phantom "updated" on
every run.

### Error Handling

tai follows a **fail-fast** philosophy. Installation MUST fail (not warn-and-continue) when:

- A source path doesn't exist (local directory missing)
- A git source fails to clone/fetch
- An asset referenced in a bundle doesn't exist in its source
- A bundle referenced in a profile doesn't exist

Rationale: Silent partial installs are worse than loud failures. A typo in a bundle definition should not result in a working-but-incomplete installation.

### Config Asset Installation

Config assets (MCPs, hooks) require different handling than file assets because they're merged into shared JSON configuration files.

#### Installation Process

1. **Read target file**: Load the existing JSON config (or `{}` if it doesn't exist)
2. **Merge entries**: Add/update the config asset entry at the appropriate key path
3. **Write back**: Save the modified JSON, preserving all non-tai-managed entries
4. **Record in manifest**: Track the asset name, target file, and checksum

#### Uninstallation Process

1. **Load manifest**: Find config entries tai installed
2. **Read target file**: Load the current JSON config
3. **Identify entries**: Match by type and name (e.g., MCP server name, hook identifier)
4. **Compare checksums**: If entry was modified since install, warn but proceed (user took ownership)
5. **Remove entries**: Delete only the entries tai installed
6. **Write back**: Save the modified JSON (or delete file if empty and tai-created)

#### Conflict Handling for Config Assets

Conflicts are judged per *entry*, not per settings key. Judging by key would be
useless for keys that are shared by design: every Claude user has a `hooks`
object holding entries from several tools, so "the key exists" would flag every
install while telling nobody anything.

| Scenario | Action |
|----------|--------|
| Entry doesn't exist | Add normally |
| Entry exists, not in manifest | **Error**: unmanaged conflict (use `--force` to overwrite) |
| Entry exists, in manifest, unchanged | No-op; the file is not rewritten |
| Entry exists, in manifest, asset changed | Update in place |
| Entry exists, in manifest, edited since install | **Error**: conflict (use `--force` to take it back, `--skip-conflicts` to keep the edit) |
| Entry in manifest, gone from the file | Add it back; reported as `restored` |

The drift case is the same bargain tai strikes for files: an entry that no
longer matches what tai wrote is the user's now, so tai stops and says so
rather than deciding for them. Uninstall makes the opposite call and leaves a
modified entry alone entirely, since removing it would destroy work tai has no
claim on.

Entries that merely sit alongside tai's in the same array cannot conflict:
appending adds to a shared list rather than displacing anything. That is what
lets hooks from tai, the user, and unrelated tools coexist under one event.
Reinstalling rewrites tai's element where it already sits rather than removing
and re-appending it, so a shared array keeps its order and the file is not
rewritten when nothing changed.

#### Scope Resolution

Config assets can be installed at different scopes. The scope is determined by (in order of precedence):

1. Per-asset `scope` in the bundle or profile entry
2. Profile-level `scope` setting
3. Default scope: `user`

The default is `user` because tai's target directory is user-global and every
file asset installs there. A `project` default would write config into whatever
directory `tai install` happened to run from.

```yaml
# Profile with explicit scope
name: work
target: claude
scope: user  # MCPs and hooks install to user-level config files

bundles:
  - dev-tools
```

### Fetched Source Caching

Command and url sources are cached in `~/.cache/tai/sources/<repo-key>/<name>`,
where `<repo-key>` is derived from the active config repo's path - switching
between config repos that declare same-named sources cannot poison each
other's cache.

- `tai install` / `tai reinstall` re-fetch these sources automatically; no
  explicit refresh is needed.
- `tai refresh` fetches without installing.
- Fetches stage into `~/.cache/tai/staging` and swap in atomically; a failed
  fetch leaves the previous cache intact (the staging dir is preserved and
  its path included in the error, for debugging).
- `tai uninstall` leaves the cache alone - it's a cache. Removing a source
  from tai.yaml orphans its cache directory, and staging dirs preserved from
  failed fetches accumulate under `~/.cache/tai/staging` until manually
  removed; a `tai cache clean` command covering both is a possible future
  addition.

## Templating

tai uses Jinja2-style templates (powered by pongo2) for asset processing during installation. This supports variable substitution, conditionals, comments, and asset inlining.

### Syntax

**Variables** use double curly braces: `{{ variable_name }}` (spaces optional - `{{variable_name}}` also works).

```markdown
# Project: {{ project_name }}

Welcome to the {{ project_name }} codebase.
```

**Conditionals** use `{% if %}`, `{% elif %}`, `{% else %}`, `{% endif %}`:

```markdown
# Instructions
{% if target_name == "codex" %}
## Codex-Specific
Include a persona for Codex here.
{% elif target_name == "claude" %}
## Claude-Specific
Claude gets different instructions.
{% endif %}
## Common
Shared across all targets.
```

**Comments** use `{# #}` and are stripped from output:

```markdown
{# This comment won't appear in the installed file #}
# Visible Content
```

**Literal template syntax** can be preserved with `{% verbatim %}`:

```markdown
{% verbatim %}
This {{ will not }} be processed as a template variable.
{% endverbatim %}
```

**Sources that are not templates.** Content written for a target rather than
for tai often contains `{#` (Markdown heading anchors such as
`## Remote transport {#remote-transport}`) or `{{`, which the engine reads as
template syntax. One parse error fails the whole install. A source declaring
`template: false` installs its content as written:

```yaml
sources:
  - name: vendor-skills
    path: ~/src/vendor-skills
    template: false
```

The opt-out follows the content, wherever it lands: the source's own assets, a
file a skill includes from it with `from:`, and an asset inlined from it with
`{% include_asset %}`. A skill from a templated source that includes a file
from this one keeps templating its own files. A file asset's `dest` is still
templated, since it is written in your config, not in the source.

tai does not instead skip a file that fails to parse and install the rest. It
would have to decide which sources are "not yours" to know when skipping is
safe, and a skip installs content that differs from what the user asked for,
silently unless someone reads the warning. A parse error stays fatal, like
every other install error, and the fix is one line in `tai.yaml`.

When installing, pass variables via the `--var` flag:

```bash
tai install work --var project_name=MyApp
```

### Variable Sources

Variables come from multiple sources with a defined priority order (later wins):

| Source | Priority | Example |
|--------|----------|---------|
| Adapter-provided | 1 (lowest) | `target_name`, `target_agents_file` |
| tai.yaml `vars` | 2 | `vars: { project: my-app }` |
| Profile `vars` | 3 | `vars: { env: prod }` |
| CLI `--var` | 4 (highest) | `--var project=override` |

tai.yaml globals are ideal for machine-specific or project-wide values that rarely change. Profile vars let you specialize per target or workflow. CLI `--var` provides one-off overrides.

```yaml
# tai.yaml
version: 1
vars:
  project_name: my-project
  author: team-alpha
sources:
  - name: local
    path: ./assets
```

```yaml
# profiles/work.yaml
name: work
target: claude
vars:
  env: production
bundles:
  - core
```

**Undefined variables** cause an error listing the missing variable names. This catches typos and forgotten definitions. For optional/conditional behavior, define the variable with an empty string:

```yaml
vars:
  optional_section: ""  # defined but falsy for {% if %}
```

Use `{% verbatim %}` to preserve literal `{{ }}` syntax in output.

### Environment Variables

The `env` filter provides access to environment variables in templates:

```markdown
Home directory: {{ "HOME"|env }}
Current user: {{ "USER"|env }}
```

Missing environment variables render as empty string (standard `os.Getenv` behavior). The filter operates at render time, independent of the variable merge chain.

### Adapter-Provided Variables

Adapters provide target-specific variables with the `target_` prefix:

| Variable | Claude | Codex | Gemini | OpenCode |
|----------|--------|-------|--------|----------|
| `target_name` | `claude` | `codex` | `gemini` | `opencode` |
| `target_name_display` | `Claude` | `Codex` | `Gemini` | `OpenCode` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `GEMINI.md` | `AGENTS.md` |

These variables enable target-aware assets. For example, a single "global instructions" file can be named appropriately per target:

```yaml
# bundles/core.yaml
assets:
  - type: file
    path: assets/files/AGENTS.md
    dest: "{{ target_agents_file }}"  # Resolves to CLAUDE.md for Claude, AGENTS.md for Codex
```

**Naming convention:** The `target_` prefix indicates adapter-provided variables. Future tai-provided variables may use `tai_*` (e.g., `tai_version`, `tai_config_repo`).

### Dest Path Templates

File assets can use template variables in the `dest` field:

```yaml
assets:
  - type: file
    path: assets/files/agents.md
    dest: "{{ target_agents_file }}"  # Full path (has extension)

  - type: file
    path: assets/files/config.json
    dest: "{{ target_name }}-config"  # Directory path (no extension)
```

**Dest interpretation:**
- If dest has an extension in the basename (e.g., `CLAUDE.md`, `subdir/app.yaml`) -> treated as full path
- If dest has no extension (e.g., `config`, `subdir`) -> treated as directory, filename appended
- If dest ends with `/` (e.g., `output/`) -> explicitly a directory

This allows a single file asset definition to install to different locations per target.

### Special Variables

#### `{{ existing }}`

The `{{ existing }}` variable injects the current contents of the target file. This is useful for wrapping existing configuration without losing user content.

```markdown
# tai Header

{{ existing }}

# tai Footer
```

When installed to a file that already contains content, the result includes the original content in place of `{{ existing }}`.

**Backup behavior:** When using `{{ existing }}`, tai:
1. Creates a backup of the original file (e.g., `CLAUDE.md.tai-backup`)
2. Records the backup path in the manifest
3. On uninstall, restores the original content from the backup

This ensures users don't lose their customizations when uninstalling tai.

### Asset Inlining

The `{% include_asset %}` tag inlines another asset's body content (frontmatter stripped) at install time:

```markdown
# Project Instructions

{% include_asset "personas/senior-swe.md" %}

## Additional Rules
```

**Source-qualified paths** use the `source:path` format:

```markdown
{% include_asset "work-shared:personas/company-style.md" %}
```

Without a source prefix, defaults to the "local" source.

**Only file-shaped assets** (personas, delegates, skills, files) can be included. Config assets (statuslines, mcps, hooks) produce a hard error.

**Recursive processing:** Included content is template-processed with the same variables, so included assets can themselves use conditionals, variables, and further `{% include_asset %}` calls.

**Cycle detection:** Circular includes (A includes B includes A) are detected and produce a hard error.

**Combining with conditionals** enables the primary use case - target-specific asset composition:

```markdown
# Project Instructions

## Core Rules
Always write tests.
{% if target_name == "codex" %}

## Codex Persona
{% include_asset "personas/company-style.md" %}
{% endif %}
```

This installs the persona content only when targeting Codex, while Claude gets the file without it.

### Disabled Features

The built-in `{% include %}` tag (pongo2/Jinja2 filesystem include) is disabled for security. Use `{% include_asset %}` for controlled asset inclusion through tai's source resolution.

## Skill Includes

Skills can declare external file dependencies via a `tai.yaml` manifest in the skill directory. At install time, tai resolves these includes and assembles them into the installed skill. The `tai.yaml` file itself is not installed—it's a build-time artifact.

### Use Cases

- **Reference documentation**: Include syntax guides, API docs from external sources
- **Shared scripts**: Pull helper scripts from a common repository
- **Examples**: Fetch examples from upstream projects
- **Machine-specific paths**: Define sources in the config repo's `tai.yaml` so paths can vary per machine

### Manifest Format

```yaml
# assets/skills/my-skill/tai.yaml
include:
  # From a named source (defined in config repo's tai.yaml)
  - from: rad                    # Source name
    path: SYNTAX.md              # Path within source
    dest: references/            # Trailing "/" preserves filename

  # From a URL
  - url: https://raw.githubusercontent.com/example/repo/main/file.md
    dest: examples/example.md    # Explicit destination path

  # Directory copy
  - from: shared-scripts
    path: helpers/               # Copy entire directory
    dest: scripts/               # Into skill's scripts/ directory

  # From a command (escape hatch for authenticated/custom fetches).
  # tai runs the argv directly with no shell - wrap in ["sh","-c","..."] if
  # you need redirection, pipes, or env-var expansion. The script must write
  # files into $TAI_DEST_DIR (see Command Includes below).
  - command:
      - sh
      - -c
      - "glab api 'projects/123/repository/files/docs%2Ffoo.md/raw?ref=main' > $TAI_DEST_DIR/foo.md"
    dest: refs/                  # tai stages files here; atomic swap on success
    timeout: 60s                 # optional, default 30s
```

### Include Fields

| Field | Required | Notes |
|-------|----------|-------|
| `from` | Yes* | Named source (mutually exclusive with `url` and `command`) |
| `path` | Yes* | Path within source |
| `url` | Yes* | URL to fetch (mutually exclusive with `from` and `command`) |
| `command` | Yes* | Argv-list to execute (mutually exclusive with `from` and `url`) |
| `timeout` | No | Duration string for `command` includes (e.g. `"60s"`, `"2m"`). Default 30s. Rejected on non-command includes. |
| `dest` | Yes | Destination within skill directory |
| `fields` | No | Map of tai field names to source field names for frontmatter remapping |

*Exactly one of `from`+`path`, `url`, or `command` must be specified.

#### Field Mapping

When including a SKILL.md from an external source whose frontmatter uses different field names, the `fields` mapping renames them to tai's vocabulary before validation and transformation:

```yaml
include:
  - from: external
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title          # Source "title" → tai "name"
      description: summary # Source "summary" → tai "description"
```

Precedence: if the source file already has the tai field name set, the explicit value wins over the mapping.

### Destination Semantics

- **Trailing `/`**: Preserve original filename/dirname (e.g., `dest: references/` + `path: SYNTAX.md` → `references/SYNTAX.md`)
- **No trailing `/`**: Use as exact target path (e.g., `dest: docs/syntax.md`)

### Source Resolution

Sources referenced by `from` must be defined in the config repo's `tai.yaml`:

```yaml
# my-tai-config/tai.yaml
sources:
  - name: rad
    path: ~/src/rad              # Machine-specific path
  - name: shared-scripts
    path: ~/work/common-scripts
```

This keeps skills portable—the same skill definition works on any machine by having each machine define its sources with local paths.

### Command Includes

`command` includes are an escape hatch for any fetch shape that doesn't fit
`url` (authenticated APIs, internal tooling, private artifact stores, etc.).
The user supplies an argv list; tai executes it with a tai-managed staging
directory and atomically swaps the script's output into the install location.

The user is responsible for the script: its authentication, its dependencies
(`gh`, `glab`, `curl`, etc.), and its behavior. Treat the manifest itself as
trusted input — installing a skill someone else wrote runs whatever commands
its manifest declares.

#### Execution model

For each `command:` include:

1. tai creates a temporary staging directory and sets `TAI_DEST_DIR` to its
   absolute path.
2. tai runs the command with `exec.CommandContext` under the include's
   timeout. The command runs directly (no shell) — wrap with `["sh", "-c",
   "..."]` explicitly if shell parsing is needed.
3. The user's environment is inherited, so `PATH`, `HOME`, and any
   secret-bearing variables (`GITLAB_TOKEN`, etc.) are available. The
   `TAI_*` variables below are appended and shadow any pre-existing values.
4. On success with non-empty output, tai removes any existing content at the
   final `dest` and atomically renames the staging dir into place
   (cross-filesystem fallback: recursive copy + remove).

   A `dest` at the skill root (`./`) is merged instead: each file or
   directory the script wrote replaces the same path in the skill, one atomic
   swap per entry, and everything else in the skill stays. This is how a
   command produces the skill's own `SKILL.md` - for example a CLI that prints
   its skill text:

   ```yaml
   include:
     - command: ["sh", "-c", "mytool skill > \"$TAI_DEST_DIR/SKILL.md\""]
       dest: ./
   ```

   The generated SKILL.md is then transformed like any other. Replacing the
   root wholesale is never what a script means, since it would delete every
   other file in the skill; a merge that fails partway can leave earlier
   entries replaced, and the install then fails and removes the skill.
5. On failure (nonzero exit, timeout, empty output, swap failure) tai leaves
   the staging dir in place and surfaces its path plus the script's stderr
   tail in the returned error. The previous install state is untouched.

`--dry-run` runs command includes too. A dry run builds each skill directory
in a scratch directory and discards it, because the manifest records the
checksum of the built directory: without building it, the preview cannot tell
an unchanged skill from an updated one, nor report the frontmatter fields its
SKILL.md would drop. The script's output never reaches the target, but
anything else the script does happens during a preview as well. This differs
from command *sources*, which a dry run never fetches, because fetching one
replaces tai's cached copy of the source.

#### Environment variables injected

All `TAI_*` variables are always set (possibly to empty string), so scripts
running under `set -u` can reference any of them without surprise. Any
pre-existing `TAI_*` values in the user's environment are stripped before
tai's authoritative values are applied, so runtime-specific env-deduplication
rules don't matter.

| Variable | Value |
|----------|-------|
| `TAI_DEST_DIR` | Absolute path to the staging directory the script must write into |
| `TAI_ASSET_TYPE` | The asset type (`skill` today) |
| `TAI_ASSET_NAME` | The asset's name |
| `TAI_TARGET` | The install target (`claude`, `codex`, `gemini`, `opencode`) |
| `TAI_MANIFEST_DIR` | Absolute path to the skill source directory (use this to read sibling helper files) |
| `TAI_VERSION` | tai's version string |

#### Secrets and error output

When a command include fails, tai's error message includes the script's argv
verbatim plus the tail of its stderr. To avoid leaking credentials into logs,
**keep secrets in environment variables, not in argv**, and avoid echoing
them to stderr. The user's environment is inherited, so `$GITLAB_TOKEN` and
similar are already visible to the script.

#### Failure modes

| Cause | Behavior |
|-------|----------|
| Command exits non-zero | Install fails. Error includes exit code, stderr tail, and staging dir path. |
| Command exceeds `timeout` | Process is killed via context cancellation. Error mentions timeout and staging path. |
| Command exits 0 with no files in `TAI_DEST_DIR` | Install fails (silent zero-output success is treated as a bug). |
| `dest` escapes the skill directory | Rejected before the command runs. |

### Security

Skill includes implement several security measures:

- **Path traversal prevention**: Source and destination paths cannot escape their directories (e.g., `../../../etc/passwd` is rejected)
- **Symlink rejection**: Symlinks in source directories are skipped to prevent escaping
- **URL scheme validation**: Only `http://` and `https://` URLs are allowed
- **Size limits**: URL downloads are limited to 10MB per file
- **HTTP timeout**: URL fetches timeout after 30 seconds
- **Redirect validation**: Redirects to non-HTTP schemes are blocked
- **Command execution trust**: `command:` includes run arbitrary processes with the user's environment. Read manifests before installing skills from sources you don't control.

### Error Handling

Includes follow tai's fail-fast philosophy:
- Missing source → installation fails
- Missing file in source → installation fails
- URL fetch failure → installation fails
- Path traversal attempt → installation fails
- Command include failure (nonzero exit, timeout, empty output) → installation fails; previous install state is preserved

### Merge Strategies (Future)

For special files like docs that may need merging rather than replacing:
- **Overwrite**: (Default) Replace entire file
- **Append/Prepend**: Add content to existing file
- **Template**: Inject content at marked locations

Editing *imported* content is a separate concern, solved by
[Patching Imported Assets](#patching-imported-assets): merge strategies combine
tai's content with what already exists at the target, while patches are about
disagreeing with what a source gave you.

## Patching Imported Assets

When you import assets from a source you don't control - a shared skills repo
maintained by another team, or a `command` source fetched into
`~/.cache/tai/sources/` - you'll sometimes need a small local delta. Editing the
checkout in place drifts silently and dies on the next pull; forking the source
loses upstream updates. Patches are the third option: keep tracking upstream,
declare a small delta on top, and be told loudly when upstream moves underneath
it.

### Declaring Patches and Exclusions

Both are declared on the source itself:

```yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    patches: patches/shared-skills/   # dir of *.yaml in this config repo
    exclude:                          # files this source does not carry at all
      - "skills/legacy-*"
```

Declaring the delta where the source is declared is deliberate. The alternative
- a separate materialization step referenced by convention - means every machine
must be told the same convention, so one change touches every config repo.

`exclude:` is independent of `patches:` and works without it. The two express
different couplings:

| | Meaning | Coupling to upstream |
|---|---|---|
| **Patch** | "Change these lines, and tell me when upstream touches them" | Tight - breaks loudly when upstream moves |
| **Exclusion** | "I don't carry this file; upstream can do what it likes" | None |

A pattern matches a path or any of its parent directories, so excluding a
directory excludes its whole subtree. A trailing `/**` is accepted and stripped.
Wildcards do not span `/` - subtree exclusion comes from that parent match, not
from a wildcard - so an interior `**` could never match and is rejected when the
config is parsed rather than silently carrying the file it was meant to drop.
Excluded files are unreachable, not merely hidden from listings: naming one in a
bundle fails with an error that points at the exclusion rather than claiming the
asset does not exist.

### Patch Files

One patch file targets one file. The patch's **name** is its path within the
patch directory, minus the extension; subdirectories are walked, so patches can
be organized freely.

```yaml
# patches/shared-skills/add-tooling-section.yaml
version: 1
file: skills/rad/SKILL.md          # source-relative - the dir is bound to a source
description: Add my personal tooling section
edits:
  - find: |-
      ## Reference
    replace: |-
      ## My Tooling

      Uses `tm` and `rad`.

      ## Reference
  - find: "Telemetry is enabled by default.\n"
    replace: ""                     # empty replace deletes the text
```

Validation at config load: `version` must be 1, `file` must be present and
relative with no `..`, there must be at least one edit, and every `find` must be
non-empty.

### Exact-Once Matching

**Every `find` must match exactly once.** Zero matches fails with "find text not
found"; two or more fails with "ambiguous - matched N locations (lines ...)".

This is the core safety property. A patch is reapplied automatically on every
install, potentially for years, against content that changes underneath it. If
tai silently skipped a `find` that no longer matched, you would quietly get
upstream behavior you had explicitly rejected. If it replaced the first of
several matches, you would quietly get an edit in the wrong place.

Edits within a patch apply in order, each seeing the previous one's result. When
several patches target the same file, they apply in name order, and a later one
may anchor on text an earlier one introduced. `tai patch list` and `tai patch
bless` judge each patch against that same running content, so a chained patch is
never reported broken for depending on its predecessor.

Blessing stays per-patch against pristine upstream regardless, since a snapshot
records what upstream said - folding tai's own output into it would leave drift
detection comparing tai to itself.

Occurrence counting is non-overlapping, so `find: "aa"` against `"aaa"` counts
once and applies.

### How Patches Are Applied

A source that declares patches gets a **materialized patched view** built into
`~/.cache/tai/views/<repoKey>/<source>/`, and all asset resolution reads from
it. The rest of tai never learns patches exist.

This is an invariant rather than a set of call sites: there is exactly one path
where source bytes enter the pipeline, and it is already patched. Applying
patches at each point content is read instead would mean every present and
future read of an asset's source path has to remember to do so, and the failure
mode when one forgets is silent wrong content.

Consequences worth knowing:

- Dependency resolution, statusline parsing, templating, frontmatter parsing and
  adapter transforms all see patched content automatically.
- `{% include_asset %}` and skill `include:` see patched content, so one source
  file never appears two different ways in a single install.
- A skill's own `tai.yaml` manifest can be patched, since includes are resolved
  from the view.
- Installed checksums cover patched bytes, so reinstalling patched content
  correctly reports "unchanged".

**The pristine source is never written to.** Views are built from it and swapped
into place atomically; a failed build leaves the previous view intact. Keeping
upstream an honest mirror is what lets blessed snapshots detect real upstream
drift rather than tai's own edits, and it means `tai locate` still points at the
checkout you can edit.

Views are built once per command invocation, only by commands that install.
Read-only commands (`tai list`, `tai locate`, `tai status`) never build one, so
they neither pay for materialization nor can fail on a broken patch. `--dry-run`
does build a view: a preview that resolved unpatched content would be previewing
something tai will never install.

Symlinked assets are followed and their content copied in, because discovery
follows them too: a view that dropped them would advertise assets it could not
produce, and adding `patches:` for one file would silently stop installing every
symlinked asset in the source.

Rebuilds are unconditional - `.git` is skipped implicitly, so a build is
typically tens of milliseconds. There is no fingerprint cache, because a view
that should have rebuilt but didn't would silently install stale content, which
is the exact failure patches exist to prevent.

### Blessing and Drift

A patch failing to apply answers *"does my edit still fit"*. It cannot answer
*"is my edit still right"*. Upstream can change something a patch has an opinion
about, in a place the patch does not touch:

> Your patch removes a `--force` flag from one line. Six months later upstream
> adds a section documenting a legitimate `--force` use case 200 lines away.
> Your patch still applies. Your installed content now contradicts itself, and
> nothing has told you.

`tai patch bless <name>` records the upstream content the patch was reviewed
against, as `patches/<dir>/<name>.blessed`. Commit it: it lives in the config
repo so drift detection travels with the repo, rather than in the cache, which
is keyed by the config repo's path and therefore machine-specific.

| Situation | Behavior |
|---|---|
| Patch applies; upstream matches the snapshot | Silent |
| Patch applies; upstream differs from the snapshot | **Warning**, install proceeds with patched content |
| Patch applies; no snapshot | One-line notice suggesting `tai patch bless` |
| Patch does not apply | **Install fails**, nothing is written |

Snapshots are per patched file, not per source: a shared repo changes constantly,
and warning about files nobody patched would train people to ignore the warning.
They store content rather than a hash so `tai patch diff` can show what moved.

Blessing refuses a patch that does not apply, whose source is not fetched, or
whose target is gone. A snapshot means "I looked at this and the patch is right
for it"; recording one for content the patch cannot apply to would bake in a
broken state and silence the warning that would prompt a fix.

Snapshots are written only by an explicit `tai patch bless`. Installs never
write into your config repo.

### Authoring and Repairing

Hand-writing search/replace is the weak point of the format, and an append is
the worst case: with nothing to anchor to, the text before the insertion has to
be repeated verbatim in both `find` and `replace`. The workspace removes that.

```bash
tai patch workspace shared-skills   # editable copy, patches already applied
# edit files normally
tai patch regenerate add-tooling-section
tai patch bless add-tooling-section
```

`tai patch regenerate` diffs your edits and derives the `find`/`replace` pairs,
widening each change with surrounding context until it anchors uniquely. The
generated edits are applied back and compared against your edited file before
being written, so a patch that regenerates is known to reproduce the edit.

The workspace is a stable path per source. A failed install promotes its
half-built tree there, so authoring and repairing are the same place and the
same commands. Patches that could not be applied are named, since those are the
changes to re-make by hand.

**Regeneration measures against pristine plus every other patch on the same
file.** This is why tai derives the edits rather than asking you to run a diff
yourself: with two patches on one file, a whole-workspace diff carries both
changes, so writing it into one of them absorbs the other and the pair corrupts
on the next run. tai knows which patch owns which file, so it can exclude only
the one being regenerated.

### Failure Behavior

A failed patch aborts the install before anything is written, and reports where
to fix it:

```
patch 'rad-skill' could not be applied - install aborted, nothing was changed:

  patch:  patches/shared-skills/rad-skill.yaml
  file:   shared-skills:skills/rad/SKILL.md
  edit #1: find text not found - upstream has likely changed since this patch was written
    | Always install with npm.

  To fix:
    1. See what changed upstream:             tai patch diff rad-skill
    2. Edit the file to match what you want:  ~/.cache/tai/workspaces/<repoKey>/shared-skills
    3. Turn that edit into the patch:         tai patch regenerate rad-skill
    4. Review the rewritten edits:            patches/shared-skills/rad-skill.yaml
    5. Accept the new upstream:               tai patch bless rad-skill
    6. Re-run the install
```

Two mistakes account for most failed finds, and both leave text that looks
byte-identical to the eye, so tai diagnoses them explicitly:

- **YAML block scalars append a newline.** `find: |` adds a trailing `\n`; use
  `find: |-` if you don't want one.
- **Line endings.** A CRLF file and an LF patch won't match.

### Non-Goals

- Regex or fuzzy matching, and unified-diff patch files. Search/replace with
  exact-once matching is deliberately the least surprising thing that works, and
  `tai patch regenerate` removes the ergonomic reason to prefer diffs.
- Pulling sources. tai does not `git pull` a local path source; you own that.
  Reporting that a source is behind its remote is a separate concern.
- Patch status in `tai status`; use `tai patch list`.

## Example Workflows

### Fresh Machine Setup

```bash
# Clone your config repo
git clone github.com/me/my-tai-config ~/my-tai-config

# Install tai CLI
go install github.com/amterp/tai/cmd/tai@latest

# Configure and install
tai config set ~/my-tai-config
tai install personal

# Ready to use
claude
```

### Editing an Asset

```bash
# Find the asset
tai locate personas/senior-swe.md
# /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Edit it
vim /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Re-install to apply changes
tai install personal

# Test
claude
```

### Adding a New Asset

```bash
# Create new persona
vim ~/my-tai-config/assets/personas/new-style.md

# Add to a bundle
vim ~/my-tai-config/bundles/core.yaml
# Add:
#   - type: persona
#     path: personas/new-style.md

# Re-install
tai install personal
```

### Switching Profiles

```bash
tai status
# Profile: personal

tai install work
# Removes personal-only assets, adds work assets

tai status
# Profile: work
```

### Work Machine Setup (Multiple Sources)

```yaml
# work-tai-config/tai.yaml
sources:
  - name: local
    path: .
  - name: personal
    path: ~/personal-tai-config
  - name: company
    path: ~/work/company-ai-config
```

```yaml
# work-tai-config/profiles/full-work.yaml
name: full-work
target: claude
bundles:
  - personal:core        # Imported from the personal repo's bundles/
  - company-standards    # Defined in this repo
  - name: company:extras # Imported, minus one asset this machine cannot use
    exclude:
      - assets/skills/beagle
```

## MVP Scope

### Included in v0.1

- [x] Config repo structure (tai.yaml, bundles/, profiles/, assets/)
- [x] Target-agnostic asset structure (personas/, delegates/, skills/, files/)
- [x] Source types: local path, other tai repo, command, url
- [x] Bundle and profile resolution with circular reference detection (fail-fast)
- [x] Diamond dependency deduplication
- [x] Claude adapter with file-copy installation and terminology mapping
- [x] Skills handling per agentskills.io standard (SKILL.md with YAML frontmatter)
- [x] Skill includes via `tai.yaml` manifest (external dependencies from sources or URLs)
- [x] Installation manifest tracking with backup support
- [x] Basic CLI: config, list, install, uninstall, status, locate
- [x] `--dry-run` flag for install preview
- [x] `--force` and `--skip-conflicts` flags for conflict handling
- [x] `tai run` pass-through
- [x] Jinja2-style templating (pongo2): variables, conditionals, comments, verbatim blocks
- [x] `{{ existing }}` templating for wrapping existing file content with backup/restore
- [x] `{% include_asset %}` for inlining asset body content at install time
- [x] StatusLine config assets with tai/native format support
- [x] Asset dependencies via `depends:` frontmatter field with automatic resolution

### Deferred to Future Versions

- [x] **Config assets**
  - [x] StatusLine definitions (`assets/statuslines/`)
  - [x] Hook definitions (`assets/hooks/`)
  - [x] MCP server definitions (`assets/mcps/`)
  - [x] Entry-level merge and surgical removal
  - [x] Scope handling (user/project/local)
  - [x] Config entry manifest tracking
- [x] **Bundle imports across sources** (`source:bundle-name`, with per-import `exclude`)
- [ ] Git remote sources
- [ ] `--add-dir` runtime injection (vs. installing to ~/.claude)
- [ ] Merge strategies (append/prepend/template) - beyond {{ existing }}
- [ ] Asset validation (schema checking)
- [ ] Version compatibility checking
- [ ] `tai lint` command
- [ ] `tai install --strict` mode
- [ ] Profile inheritance

## Future Considerations

Items considered during design but explicitly deferred. These inform future development but are NOT in MVP scope.

### `tai lint` Command
**What:** Static analysis to catch common issues before install.
**Why deferred:** Adds complexity. Documentation can cover common dependency patterns for now.
**Future scope:**
- Warn about missing common asset dependencies (skill needs doc)
- Detect unused assets in bundles
- Catch circular bundle references

### ~~Asset Dependencies (DAG Resolution)~~
**Status:** IMPLEMENTED. Assets can declare `depends:` in frontmatter to list required assets. Dependencies are resolved depth-first with cycle detection. See [Asset Dependencies](#asset-dependencies) in the File Asset Formats section.

### Profile as Context Container
**What:** Profiles hold environment variables, model constraints, and target-specific flags beyond just asset composition.
**Why deferred:** Current profiles (bundles + target + options) are sufficient for MVP. Environment context can be passed via wrapper or shell.
**Future scope:** `env:`, `constraints:`, and richer `options:` blocks in profile definition.

### Namespacing with Aliases
**What:** Full namespacing (`source::type/name`) with explicit alias promotion to global namespace.
**Why deferred:** Current `source:path` syntax handles most cases. "Later wins" with info logging is sufficient.
**Future scope:** `--strict` mode that fails on conflicts; explicit aliasing in tai.yaml.

### Hooks System
**What:** Pre/post install scripts in config repo (`hooks/post-install.sh`).
**Why deferred:** Security concern—arbitrary script execution from potentially remote sources. Requires careful sandboxing.
**Future scope:** Opt-in hooks with explicit user consent; possibly restricted to local sources only.

### Adapter as State Synchronizer
**What:** Adapters sync to "desired state" rather than copying files—supporting future targets that use DBs, APIs, etc.
**Why deferred:** All current targets are file-based. Premature abstraction.
**Future scope:** Refactor adapter interface when first non-file-based target emerges.

### Skill Versioning
**What:** Version requirements and compatibility checking for skills.
**Why deferred:** The skill include system covers the immediate need for external dependencies. Version constraints add complexity.
**Future scope:** Add when skills need version pinning or compatibility matrices.

### Config Assets (MCPs and Hooks)
**What:** Support for MCP servers and hooks as first-class asset types, merged into target JSON config files.
**Why deferred:** Requires different installation logic (JSON merge vs. file copy), manifest tracking changes, and scope handling. File-based assets cover the core use cases for v0.1.
**Design decisions:**
- Config assets stored as YAML files in `assets/mcps/` and `assets/hooks/` for consistency with other assets
- Same referencing syntax in bundles (`mcps/github`, `hooks/format-on-save`)
- Filename becomes identifier (MCP server name, hook name) unless overridden
- Installation scope (project/user/local) controlled at profile level with per-asset override option
- Uninstall matches by identifier; warns if entry was modified (user took ownership)
- Adapters declare which asset types they support; unsupported types in a profile cause fail-fast error
**Target version:** v0.2

## Testing Philosophy

1. **No hardcoded paths**: All filesystem paths are injected via config structs
2. **Temp directories**: All tests create isolated temp dirs via `t.TempDir()`
3. **Fixtures**: Helper functions in `internal/testutil` set up fake config repos
4. **Coverage targets**:
   - Unit tests for all parsing/resolution logic
   - Integration tests for adapters
   - E2E tests for full workflows in `e2e/` directory

## Implementation Notes

### CLI Framework

tai uses [github.com/amterp/ra](https://github.com/amterp/ra) for CLI argument parsing. ra provides a clean, declarative API for defining commands with dual-nature arguments.

### UI Libraries

For creating rich terminal output, the following libraries are approved:
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components

## Open Questions

1. ~~**Conflict resolution strategy**~~: **RESOLVED.** Later source/bundle in list wins. tai logs info message when override occurs.

2. **Checksum on re-install**: Should we warn if source file changed but installed file was also modified externally? (Both diverged from manifest.)

3. ~~**Skills structure**~~: **RESOLVED.** Skills follow the [agentskills.io](https://agentskills.io) standard (SKILL.md with YAML frontmatter). Bundles reference them as `skills/my-skill` (directory path, no trailing slash needed).

4. ~~**CLAUDE.md handling**~~: **RESOLVED.** Use `{{existing}}` templating to wrap user content. tai backs up the original and restores it on uninstall.

5. **Profile inheritance**: Should profiles be able to extend other profiles? Deferred—adds complexity but might be useful in the future.

7. **Config asset scope granularity**: Should scope be per-asset, per-asset-type, or only per-profile? Current design uses profile-level default. May need per-asset override for mixed project/user installs.

6. ~~**Templating syntax**~~: **RESOLVED.** Jinja2-style templates via pongo2: `{{ variable }}` substitution, `{% if %}`/`{% elif %}`/`{% else %}` conditionals, `{% include_asset %}` for inlining other assets' body content, `{% verbatim %}` for literal brace escaping. Special variable `{{ existing }}` for injecting current file content. Undefined variables render as empty string (standard Jinja2 behavior).

---

*This spec is a living document. Feedback welcome.*
