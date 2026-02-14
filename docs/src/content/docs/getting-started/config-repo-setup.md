---
title: Config Repo Setup
description: Understanding and structuring your tai configuration repository
---

A **config repo** is a directory (ideally a git repository) that serves as the source of truth for your AI agent configurations. It defines what assets you have, how they're grouped into bundles, and which profiles install them to which targets.

Tai never modifies your config repo. It reads from it and writes to target-specific directories like `~/.claude/`. You can always reconstruct your target config from the repo.

## Directory layout

A config repo follows this structure:

```
my-tai-config/
├── tai.yaml              # Required: repo manifest
├── assets/               # Target-agnostic asset files
│   ├── personas/         # Behavioral styles and roles
│   ├── delegates/        # Specialized sub-agents
│   ├── skills/           # Procedural capabilities (directories)
│   ├── files/            # Loose files copied as-is
│   └── statuslines/      # Status bar configurations (YAML)
├── bundles/              # Bundle definitions (YAML)
└── profiles/             # Profile definitions (YAML)
```

Not every directory is required. If you only use personas and delegates, you can skip the rest. Tai ignores empty or missing asset subdirectories.

## tai.yaml

The `tai.yaml` file at the root of your config repo is the entry point. A minimal example:

```yaml
version: 1
default_profile: personal
```

A more complete example with multiple sources:

```yaml
version: 1

sources:
  - name: work-shared
    path: ~/work/ai-common

  - name: team-repo
    type: tai-repo
    path: ~/team-tai-config

default_profile: work
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `version` | yes | Schema version. Currently `1`. |
| `sources` | no | Additional asset sources beyond the implicit local source. |
| `default_profile` | no | Profile to use when `tai install` is run without an argument. |

## The implicit local source

Every config repo has an implicit source called `local` that points to the `./assets` directory. You don't need to declare it in `tai.yaml` - tai adds it automatically.

When a bundle references an asset path without a source prefix, it resolves against `local`:

```yaml
# These are equivalent:
- type: persona
  path: personas/senior-swe.md

- type: persona
  path: local:personas/senior-swe.md
```

You only need to explicitly declare the local source in `tai.yaml` if you want to override its path to point somewhere other than `./assets`.

## Asset directories

Each subdirectory under `assets/` holds a different type of asset:

| Directory | Asset type | Format | Notes |
|-----------|-----------|--------|-------|
| `personas/` | Persona | Markdown with frontmatter | Behavioral styles and roles |
| `delegates/` | Delegate | Markdown with frontmatter | Specialized sub-agents |
| `skills/` | Skill | Directory with `SKILL.md` | Procedural capabilities |
| `files/` | File | Any format | Copied as-is, no transformation |
| `statuslines/` | StatusLine | YAML | Merged into target config files |

Personas, delegates, and skills use [tai frontmatter](/tai/reference/frontmatter/) to declare metadata like `name`, `description`, and `tools`. During installation, tai strips its frontmatter and the target adapter writes the appropriate target-specific format.

Files are the exception - they're copied byte-for-byte with no parsing or transformation.

## Bundles and profiles

**Bundles** (in `bundles/`) are named lists of assets. They act as building blocks you can compose and reuse:

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
```

Bundles can include other bundles for composition. See [Bundles](/tai/concepts/bundles/) for details.

**Profiles** (in `profiles/`) bind one or more bundles to a specific target:

```yaml
# profiles/personal.yaml
name: personal
description: My personal development setup
target: claude

bundles:
  - core
```

See [Profiles](/tai/concepts/profiles/) for the full profile format.

## Version control

We strongly recommend tracking your config repo with git. This gives you:

- **History** of what changed and when
- **Branching** to experiment with new configurations
- **Portability** across machines by cloning the repo
- **Collaboration** by sharing repos with teammates or publishing them as sources

Since tai treats the config repo as read-only, there are no merge conflicts from tai itself - only from your own edits.

## Next steps

- [How Tai Works](/tai/concepts/how-tai-works/) - understand the full data flow from config repo to target
- [Assets](/tai/concepts/assets/) - deep dive into each asset type
- [Sources](/tai/concepts/sources/) - add external asset sources
