---
title: Working with Multiple Sources
description: How to configure and combine assets from multiple sources - personal, team, and external
---

Sources are where tai finds assets. By default, your config repo's `assets/` directory is the only source. But real-world setups often need more: a shared team config, a company standards repo, or a personal collection you use across machines. This guide covers configuring multiple sources and combining their assets.

## Why multiple sources

Common scenarios where multiple sources help:

- **Personal + work**: Your personal preferences (personas, writing style) combined with company-mandated standards (security delegates, coding guidelines)
- **Shared team configs**: A central repository of assets that the whole team uses, with individual overrides
- **Project-specific additions**: Assets tied to a specific project that supplement your base config
- **External skill libraries**: Pulling skills from other tai config repos

## The implicit local source

Every config repo automatically has an implicit `local` source pointing to its `assets/` directory. You do not need to declare it:

```yaml
# my-tai-config/tai.yaml
version: 1
default_profile: personal
```

This is equivalent to:

```yaml
version: 1
sources:
  - name: local
    path: ./assets
default_profile: personal
```

You only need to declare `local` explicitly if you want to override where it points.

## Source types

Tai supports two source types today.

### Local directory

A local directory source points to a folder containing assets in the standard layout (`personas/`, `delegates/`, `skills/`, `files/`, etc.):

```yaml
sources:
  - name: work-shared
    path: ~/work/ai-common
```

The directory at `~/work/ai-common` must contain asset subdirectories:

```
~/work/ai-common/
├── personas/
│   └── company-style.md
├── delegates/
│   └── security-reviewer.md
└── skills/
    └── incident-response/
        └── SKILL.md
```

### Another config repo

Point a source at a config repo root and its assets are reachable under `assets/`:

```yaml
sources:
  - name: personal
    path: ~/personal-tai-config
```

References then read `personal:assets/personas/senior-swe.md`. Nothing is appended for you, which is also what puts that repo's `bundles/` within reach - see [importing bundles](#importing-bundles) below.

## Configuring sources in tai.yaml

Here is a complete example with multiple sources:

```yaml
# ~/work-tai-config/tai.yaml
version: 1

sources:
  # The implicit local source (this repo) is always available.
  # No need to declare it unless overriding.

  # Another local directory with shared team assets
  - name: team
    path: ~/work/team-ai-config

  # Your personal tai config repo
  - name: personal
    path: ~/personal-tai-config

default_profile: work
```

### Source priority

Sources are listed in priority order: **later sources override earlier ones** when asset paths conflict. In the example above:

1. `local` (implicit, lowest priority)
2. `team` (overrides local on conflicts)
3. `personal` (overrides team and local on conflicts)

This ordering lets you establish a base layer of defaults (local), apply team standards on top, and then override with personal preferences.

Tai logs an info message whenever a source override occurs, so you always know when one source's asset replaces another's.

## Referencing assets across sources

In bundles and profiles, use the `source:path` syntax to reference assets from a specific source:

```yaml
# bundles/work-setup.yaml
name: work-setup
description: Combined work configuration

assets:
  # From the implicit local source (no prefix needed)
  - type: persona
    path: assets/personas/senior-swe.md

  # From the team source
  - type: delegate
    path: team:delegates/security-reviewer.md
  - type: persona
    path: team:personas/company-style.md

  # From the personal source
  - type: delegate
    path: personal:assets/delegates/quick-summarizer.md
```

Without a source prefix, the path resolves against the `local` source. The prefix syntax works everywhere assets are referenced - in bundles, profiles, and `depends` fields.

## Importing bundles

Referencing another repo's assets one at a time means restating its bundles in yours. That works exactly once. When the other repo adds a skill to a bundle, your copy does not change and nothing errors - it is a valid bundle, just an old one. Drift like this is quiet by construction, and you find it when you go looking for a skill that should have been there.

Import the bundle instead:

```yaml
# ~/work-tai-config/profiles/work.yaml
bundles:
  - full-work           # this repo's own bundle
  - personal:core       # ~/personal-tai-config/bundles/core.yaml
```

Now `personal:core` is read from the personal repo at install time. Whatever it contains today is what you get.

A source offers whatever is in `<root>/bundles/`. `tai list bundles` shows them grouped by source, so you can see what is importable before writing the reference.

### Imported bundles keep their own bearings

`core` was written for the personal repo, so its unprefixed paths mean *that* repo:

```yaml
# ~/personal-tai-config/bundles/core.yaml
assets:
  - type: persona
    path: assets/personas/senior-swe.md   # personal's copy, always
```

This holds even if your work repo has a file at the same path. The same applies to bundles that `core` itself includes.

The exception is a prefixed path. If `core` reaches into a third source with `recl:assets/skills/recl`, that name is looked up in **your** `tai.yaml`, because yours is the only one tai loads. Declare a source called `recl` too, or the install fails - naming the missing source and, where it can read the origin repo's `tai.yaml`, what that repo points it at.

### Dropping assets you cannot use

Machines differ. If a bundle carries a skill that cannot work where you are installing it, carve it out at the import:

```yaml
bundles:
  - name: work:work-tools
    exclude:
      - assets/skills/beagle
```

The rest keeps flowing through automatically, which is the whole point - mirroring the bundle to drop one asset brings back the drift you were escaping.

Two scopes are available, and they answer different questions. `exclude` on the import means "not from this bundle". `exclude` on the source in `tai.yaml` means "never from this source at all", covering every bundle you import from it:

```yaml
sources:
  - name: work
    path: ~/work-tai-config
    exclude:
      - assets/skills/beagle
```

A source exclusion filters imported bundles quietly, logging what it dropped. Naming an excluded asset in your *own* config is still an error - that is a contradiction you wrote, rather than one you inherited.

## Complete worked example

Here is a realistic setup for a work machine that combines personal and company configurations.

### Directory layout

```
~/personal-tai-config/          # Your personal config repo
├── tai.yaml
├── assets/
│   ├── personas/
│   │   ├── senior-swe.md
│   │   └── brainstormer.md
│   ├── delegates/
│   │   └── quick-summarizer.md
│   └── files/
│       └── personal-instructions.md
├── bundles/
│   └── core.yaml
└── profiles/
    └── personal.yaml

~/work/team-ai-config/          # Shared team asset directory
├── personas/
│   └── company-style.md
├── delegates/
│   ├── security-reviewer.md
│   └── compliance-checker.md
└── skills/
    └── incident-response/
        └── SKILL.md

~/work-tai-config/              # Your work config repo (active)
├── tai.yaml
├── assets/
│   ├── delegates/
│   │   └── project-helper.md
│   └── files/
│       └── work-instructions.md
├── bundles/
│   ├── company-standards.yaml
│   └── full-work.yaml
└── profiles/
    └── work.yaml
```

### Work config repo tai.yaml

```yaml
# ~/work-tai-config/tai.yaml
version: 1

sources:
  - name: team
    path: ~/work/team-ai-config
  - name: personal
    path: ~/personal-tai-config

default_profile: work
```

### Bundles

A bundle for company-mandated assets:

```yaml
# ~/work-tai-config/bundles/company-standards.yaml
name: company-standards
description: Required company assets for all developers

assets:
  - type: persona
    path: team:personas/company-style.md
  - type: delegate
    path: team:delegates/security-reviewer.md
  - type: delegate
    path: team:delegates/compliance-checker.md
  - type: skill
    path: team:skills/incident-response
```

A bundle that combines everything:

```yaml
# ~/work-tai-config/bundles/full-work.yaml
name: full-work
description: Full work setup with personal and company assets

bundles:
  - company-standards

assets:
  # Personal assets pulled from the personal source
  - type: persona
    path: personal:assets/personas/senior-swe.md
  - type: delegate
    path: personal:assets/delegates/quick-summarizer.md

  # Local work-specific assets
  - type: delegate
    path: assets/delegates/project-helper.md
  - type: file
    path: assets/files/work-instructions.md
    dest: ""
```

### Profile

```yaml
# ~/work-tai-config/profiles/work.yaml
name: work
description: Full work environment
target: claude

bundles:
  - full-work
```

### Install

```bash
tai config set ~/work-tai-config
tai install work
```

This produces a Claude Code installation that includes:
- Company personas and delegates from the team source
- Your personal persona and delegate from the personal source
- Work-specific delegates and files from the local source

All coming together into a single, consistent installation.

## Tips

### Keep source names stable

Bundles reference sources by name. If you rename a source in `tai.yaml`, update all bundles that reference it. Tai will fail fast if a bundle references a source that doesn't exist.

### Import bundles rather than copying them

Restating another repo's bundle in yours works until that bundle changes. Import it instead - see below.

For plain directories that just contain asset folders, use the default type (local directory) without specifying `type`.

### List available assets per source

To see what assets are available from a specific source:

```bash
tai list assets --source team
```

Or list all assets across all sources:

```bash
tai list assets
```

## Next steps

- [Composing Bundles](/tai/guides/composing-bundles/) - bundle composition and override behavior
- [Target-Aware Assets](/tai/guides/target-aware-assets/) - write assets that adapt to different targets
- [tai.yaml reference](/tai/reference/tai-yaml/) - complete source configuration specification
