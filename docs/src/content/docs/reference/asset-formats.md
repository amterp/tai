---
title: Asset Formats
description: File structure and format reference for all tai asset types
---

Assets are the atomic units of configuration in tai. This page covers the file structure and format requirements for each asset type. For frontmatter field details, see [Frontmatter](/tai/reference/frontmatter/). For config assets, see [Config Assets](/tai/reference/config-assets/).

## Two shapes

Assets come in two shapes based on how they are installed:

| Shape | Asset types | How defined | How installed |
|-------|-------------|-------------|---------------|
| **File assets** | persona, delegate, skill, file | Files or directories in `assets/` | Copied to target paths |
| **Config assets** | statusline | YAML files in `assets/` | Merged into target JSON config files |

## Personas

Behavioral styles or roles for the AI agent. In Claude Code, personas map to output-styles.

### Single-file persona

A Markdown file with tai [frontmatter](/tai/reference/frontmatter/):

```
assets/personas/senior-swe.md
```

```markdown
---
name: Senior SWE
description: Experienced software engineer co-owner role.
---

You are a senior software engineer...
```

### Directory persona

A directory containing a `PERSONA.md` main file plus any number of supporting files:

```
assets/personas/senior-swe/
  PERSONA.md              # Main file (required, transformed during install)
  references/
    coding-standards.md   # Supporting file (copied as-is)
```

During installation:
- `PERSONA.md` is processed: tai frontmatter is stripped and target-specific frontmatter is rendered
- Supporting files are copied as-is, preserving directory structure

Installed result (Claude target):

```
~/.claude/output-styles/senior-swe/
  senior-swe.md           # Main file renamed to match directory name
  references/
    coding-standards.md   # Copied unchanged
```

## Delegates

Specialized helpers for specific tasks. In Claude Code, delegates map to agents (sub-agents).

### Single-file delegate

```
assets/delegates/code-reviewer.md
```

```markdown
---
name: Code Reviewer
description: Reviews code for quality, correctness, and style.
tools: Read, Grep, Glob
model: sonnet
---

Review the provided code changes...
```

### Directory delegate

A directory containing a `DELEGATE.md` main file plus supporting files:

```
assets/delegates/code-reviewer/
  DELEGATE.md             # Main file (required, transformed during install)
  checklists/
    security.md           # Supporting file (copied as-is)
```

The installation behavior is the same as directory personas: `DELEGATE.md` is transformed, everything else is copied as-is.

## Skills

Procedural capabilities that follow the [agentskills.io](https://agentskills.io) standard. Skills are always directories containing a `SKILL.md` main file.

```
assets/skills/council-participant/
  SKILL.md                # Main file (required, transformed during install)
  helpers/
    voting.md             # Supporting file (copied as-is)
```

```markdown
---
name: Council Participant
description: Participate in a multi-agent review council.
tools: Bash, Read, Write
---

# Council Participant

Follow these steps when participating in a council review...
```

### Skill includes

Skills can declare external file dependencies via a `tai.yaml` manifest in the skill directory. This file is a build-time artifact and is not installed. See [Skill Includes](/tai/reference/skill-includes/) for the complete reference.

```
assets/skills/my-skill/
  SKILL.md
  tai.yaml                # Build manifest (not installed)
  references/
    syntax.md             # May come from an include
```

### Skill installation

During installation:
1. The entire directory is copied to the target (excluding `tai.yaml`)
2. Skill includes are resolved (files copied from sources or fetched from URLs)
3. `SKILL.md` is transformed in place: tai frontmatter is stripped and target-specific frontmatter is rendered

This ordering means skill includes can replace the `SKILL.md` file itself (e.g., pulling a skill definition from an external source).

## Files

Loose files copied as-is to the target. The `file` type has no frontmatter requirements - content is preserved byte-for-byte.

Files are referenced in bundles and profiles with a required `dest` field:

```yaml
assets:
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""              # Empty string = target root directory
  - type: file
    path: assets/files/config.json
    dest: "config"        # Goes to <target-dir>/config/config.json
```

### dest interpretation

| dest value | Interpretation | Example result (Claude) |
|------------|---------------|------------------------|
| `""` (empty) | Target root | `~/.claude/myfile.md` |
| `"subdir"` (no extension) | Directory - filename appended | `~/.claude/subdir/myfile.md` |
| `"subdir/"` (trailing slash) | Explicit directory - filename appended | `~/.claude/subdir/myfile.md` |
| `"output.md"` (has extension) | Exact file path | `~/.claude/output.md` |
| `"sub/output.md"` | Exact file path in subdirectory | `~/.claude/sub/output.md` |

The `dest` field supports [template variables](/tai/reference/template-syntax/#dest-path-templates) for target-aware file placement.

### Key differences from other file assets

- No frontmatter parsing or transformation
- Content is never modified (no stripping, no rendering)
- `dest` field is required (determines target location)
- Does not support the `depends` frontmatter field
- Templates in file content are still processed if present (variables, conditionals, etc.)

## StatusLines

Status line assets are config assets - they are merged into the target's JSON settings file rather than written as standalone files. See [Config Assets](/tai/reference/config-assets/) for the complete reference.

```
assets/statuslines/detailed.yaml
```

```yaml
level: detailed
show:
  - cost
  - model
  - tokens
```

StatusLines are singletons: only one can be active per profile. When multiple statusline assets appear, the later one overrides the earlier one.

## Summary table

| Asset type | File format | Main file (directory) | Frontmatter | dest field | Config asset |
|------------|-------------|----------------------|-------------|------------|-------------|
| persona | Markdown | `PERSONA.md` | yes | no | no |
| delegate | Markdown | `DELEGATE.md` | yes | no | no |
| skill | Directory with Markdown | `SKILL.md` | yes | no | no |
| file | Any | N/A | no | required | no |
| statusline | YAML | N/A | no | no | yes |
