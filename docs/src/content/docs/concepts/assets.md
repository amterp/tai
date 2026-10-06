---
title: Assets
description: The atomic units of configuration in tai - personas, delegates, skills, files, and config entries
---

Assets are the atomic units of configuration in tai. Each asset represents a single piece of AI agent configuration - a behavioral style, a specialized helper, a skill, a loose file, or a config entry.

## Two shapes

Assets come in two shapes based on how they are installed:

| Shape | Asset types | How installed |
|-------|-------------|---------------|
| **File assets** | persona, delegate, skill, file | Copied to target paths |
| **Config assets** | statusline, hook, mcp | Merged into the target's config files |

## File assets

### Personas

Behavioral styles or roles for the AI agent. In Claude Code, these map to output-styles.

```markdown
---
name: Senior SWE
description: Experienced software engineer co-owner role.
---

You are a senior software engineer...
```

### Delegates

Specialized helpers for specific tasks. In Claude Code, these map to agents (sub-agents).

```markdown
---
name: Code Reviewer
description: Reviews code for quality, correctness, and style.
tools: Read, Grep, Glob
---

Review the provided code...
```

### Skills

Procedural capabilities. Skills are typically directories containing a `SKILL.md` main file plus supporting files.

```markdown
---
name: Database Migration
description: Manages database schema migrations.
tools: Bash, Read, Write
---

# Database Migration

Follow these steps...
```

### Files

Loose files copied as-is to the target. Unlike other file assets, the `file` type has **no frontmatter requirements** - content is preserved byte-for-byte. The `dest` field is required and determines where the file is placed.

```yaml
# In a bundle or profile
assets:
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""              # Empty string = target root directory
  - type: file
    path: assets/files/config.json
    dest: "config"        # Goes to <target-dir>/config/config.json
```

## Config assets

Config assets are entries merged into target JSON configuration files rather than written as standalone files.

### StatusLines

Status line assets control the target's status bar appearance. They can be defined in tai format (portable) or native format (target-specific passthrough).

```yaml
# Tai format (recommended)
level: detailed
show:
  - cost
  - model
  - tokens
```

StatusLines are singleton - only one can be active. When multiple statusline assets appear in a profile, later ones override earlier ones.

## Frontmatter

All file assets except `file` type require YAML frontmatter with at least a `name` field. Available fields vary by type:

| Field | Personas | Delegates | Skills |
|-------|----------|-----------|--------|
| `name` | required | required | required |
| `description` | optional | optional | optional |
| `tools` | - | optional | optional |
| `model` | - | optional | optional |
| `depends` | optional | optional | optional |

Fields outside this table are not installed, and tai warns about each one. See [Frontmatter](/tai/reference/frontmatter/#unknown-fields).

The `file` type is different - it has no frontmatter processing at all. Content is copied exactly as written.

## Directory assets

Personas, delegates, and skills can be directories rather than single files. Each directory has a conventional main file plus any number of supporting files.

| Asset type | Main file | Example path |
|------------|-----------|--------------|
| Persona | `PERSONA.md` | `personas/senior-swe/PERSONA.md` |
| Delegate | `DELEGATE.md` | `delegates/code-reviewer/DELEGATE.md` |
| Skill | `SKILL.md` | `skills/db-migration/SKILL.md` |

During installation:
- The main file is transformed with target-specific frontmatter
- Supporting files are copied as-is, preserving the directory structure

```
# Source
assets/skills/council-participant/
  SKILL.md              # Transformed
  helpers/
    voting.md           # Copied as-is

# Installed (Claude)
~/.claude/skills/council-participant/
  council-participant.md  # Main file renamed to match directory
  helpers/
    voting.md
```

## Asset dependencies

Assets can declare dependencies on other assets using the `depends` frontmatter field. When an asset is included in a [bundle](/tai/concepts/bundles/) or [profile](/tai/concepts/profiles/), its dependencies are automatically pulled in.

```markdown
---
name: Code Review
description: Request a code review with specialized reviewers
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - skills/council-participant
---
```

Each dependency is a path in the format `type-dir/name` or `source:type-dir/name`:

```yaml
depends:
  - delegates/bug-hunter.md            # From local source
  - skills/council-participant         # Directory-based skill
  - other-source:delegates/helper.md   # From named source
```

Resolution rules:

- **Depth-first ordering** - dependencies are resolved before the assets that need them
- **Deduplication** - same asset included multiple times resolves once via "later wins"
- **Explicit wins** - assets explicitly listed in a bundle/profile override auto-included dependencies
- **Cycle detection** - circular dependencies cause installation to fail with a clear error
- **Stripped on install** - the `depends` field is consumed during resolution and never written to the target

The `file` type does not support dependencies since its frontmatter is not parsed.
