---
title: Frontmatter
description: Complete field reference for tai's YAML frontmatter in asset files
---

Tai uses YAML frontmatter (delimited by `---` markers) to declare metadata in persona, delegate, and skill assets. During installation, tai strips its own frontmatter and the target adapter renders target-specific frontmatter in its place.

## Field reference

| Field | Type | Personas | Delegates | Skills | Description |
|-------|------|----------|-----------|--------|-------------|
| `name` | string | required | required | required | Human-readable display name |
| `description` | string | optional | optional | optional | Short description of what this asset does |
| `tools` | string | - | optional | optional | Comma-separated list of tools the asset may use |
| `model` | string | - | optional | optional | Model to use (e.g., `haiku`, `sonnet`, `opus`) |
| `depends` | list | optional | optional | optional | Asset dependencies (see [Asset dependencies](#asset-dependencies)) |

The `file` type has no frontmatter requirements. Its content is copied as-is without any parsing or transformation.

## name

Required for personas, delegates, and skills. This is the display name shown in the target tool's UI.

```yaml
---
name: Senior SWE
---
```

If `name` is missing or empty, tai exits with an error.

## description

Optional. A short description of the asset's purpose. How this is displayed depends on the target adapter.

```yaml
---
name: Code Reviewer
description: Reviews code for quality, correctness, and potential issues.
---
```

If your description contains colons, use a YAML block scalar to avoid parsing issues:

```yaml
---
name: Code Reviewer
description: >-
  Reviews code for quality: checks correctness, style, and potential issues.
---
```

## tools

Optional. Available for delegates and skills only. A comma-separated list of tools the asset is allowed to use. Not valid for personas.

```yaml
---
name: Code Reviewer
description: Reviews code changes.
tools: Read, Grep, Glob
---
```

## model

Optional. Available for delegates and skills only. Specifies which model to use for this asset.

```yaml
---
name: Quick Helper
description: Fast helper for simple tasks.
tools: Read, Bash
model: haiku
---
```

## depends

Optional. Available for personas, delegates, and skills. Declares dependencies on other assets. See [Asset dependencies](#asset-dependencies) below.

## How frontmatter is processed

The install pipeline processes frontmatter in these steps:

1. **Parse** - tai reads the `---` delimited YAML block and separates metadata from body content
2. **Validate** - tai checks required fields (e.g., `name` must be present for personas/delegates/skills)
3. **Strip** - tai removes its own frontmatter from the content
4. **Transform** - the target adapter renders target-specific frontmatter using the parsed metadata

This means the frontmatter you write in your config repo is tai's format, and what gets installed is the target's format. The body content below the frontmatter is preserved unchanged (after template processing).

### Example transformation

Source file (`assets/personas/senior-swe.md`):

```markdown
---
name: Senior SWE
description: Experienced software engineer.
depends:
  - assets/delegates/code-reviewer.md
---

You are a senior software engineer...
```

Installed file for Claude (`~/.claude/output-styles/senior-swe.md`):

```markdown
---
name: Senior SWE
description: Experienced software engineer.
---

You are a senior software engineer...
```

Note that `depends` is consumed during resolution and not written to the target. The Claude adapter renders only `name` and `description` for personas.

### Adapter transformation by type

| Target | Personas | Delegates | Skills |
|--------|----------|-----------|--------|
| Claude | `name`, `description` | `name`, `description`, `tools`, `model` | `name`, `description`, `tools`, `model` |
| Codex | `name`, `description` | `name`, `description`, `tools`, `model` | `name`, `description` |

## Unknown fields

A field outside tai's vocabulary is not installed, and tai warns about it, naming the field and
suggesting `native.<target>` if you meant it for one target. Assets from external sources often
carry such fields, so the warning is information, not an error. Nested values (a `metadata:` map,
a list) are dropped the same way.

```yaml
---
name: My Persona
description: A custom persona.
custom_field: some value   # Not installed; tai warns
metadata:                  # Same, nested or not
  author: Someone
---
```

`name`, `description`, `tools` and `model` each take a single value. A list or map under one of
them is not installed, with a warning: write `tools` as a comma-separated string
(`tools: Read, Grep`), not a YAML list. A `fields:` include mapping is the exception, and joins a
list it maps with commas, so Agent Skills' `allowed-tools: [Read, Grep]` can map onto `tools`.

## Schema versioning

Frontmatter does not carry its own schema version. The schema is tracked at the repo level via `version: 1` in [tai.yaml](/tai/reference/tai-yaml/). If tai introduces new frontmatter fields in the future, the `tai.yaml` version number will indicate which fields are supported.

## Asset dependencies

Assets can declare dependencies on other assets using the `depends` field. When an asset with dependencies is included in a bundle or profile, its dependencies are automatically pulled in.

### Syntax

A dependency is a path from its source root, optionally source-qualified - the same form a bundle entry uses:

```yaml
---
name: Code Review
description: Request a code review with specialized reviewers.
depends:
  - assets/delegates/bug-hunter.md
  - assets/delegates/code-reviewer.md
  - assets/delegates/performance-reviewer.md
  - assets/skills/council-participant
  - other-source:delegates/helper.md
---
```

The directory holding the asset names its type, so the same directory name means the same type wherever the source root sits:

| Directory | Asset type |
|-----------|-----------|
| `personas/` | persona |
| `delegates/` | delegate |
| `skills/` | skill |
| `files/` | file |
| `statuslines/` | statusline |

### Resolution rules

1. **Depth-first ordering** - dependencies are resolved before the assets that need them, ensuring transitive dependencies appear first
2. **Deduplication** - if the same asset appears multiple times (directly or as a dependency), it is included once using "later wins" semantics
3. **Explicit wins** - assets explicitly listed in a bundle or profile override auto-included dependencies
4. **Cycle detection** - circular dependencies (A depends on B, B depends on A) are detected and cause installation to fail with a clear error
5. **Stripped on install** - the `depends` field is tai metadata consumed during resolution and never written to the target

### Use case

A skill like `code-review` that orchestrates multiple specialized delegates does not require users to manually include each delegate. Include the skill, and its delegates are pulled in automatically:

```yaml
# bundles/core.yaml
assets:
  - type: skill
    path: assets/skills/code-review   # Automatically pulls in its depends
```

### Limitations

- The `file` type does not support `depends` because its frontmatter is not parsed
- Dependencies must be resolvable in the active config repo's sources
- Dependency paths cannot reference bundles, only individual assets
