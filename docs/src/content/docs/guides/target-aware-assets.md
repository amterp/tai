---
title: Target-Aware Assets
description: How to write assets that adapt their content and installation paths based on the target
---

Tai's templating system includes adapter-provided variables that let you write assets once and have them behave differently depending on the target (Claude Code, Codex, etc.). This guide shows how to use these variables for target-specific content, conditional sections, and dynamic file destinations.

## Adapter-provided variables

Each target adapter provides a set of variables with the `target_` prefix. These are automatically available in all templates during installation:

| Variable | Claude | Codex | OpenCode |
|----------|--------|-------|----------|
| `target_name` | `claude` | `codex` | `opencode` |
| `target_name_display` | `Claude` | `Codex` | `OpenCode` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` |

You don't need to pass these with `--var` - they are injected automatically based on the profile's `target` field.

## Dynamic file destinations

The most common use of target variables is in the `dest` field of file assets. This lets a single asset definition install to the right location for each target.

### Example: a universal agents file

You maintain a single project instructions file that should install as `CLAUDE.md` for Claude Code and `AGENTS.md` for Codex:

Create `assets/files/agents-file.md`:

```markdown
# Project Instructions

This project uses Go 1.24+ with standard library conventions.

## Build

Run `go build ./cmd/myapp` to compile.

## Test

Run `go test ./...` for all tests.
Always write table-driven tests for new functions.
```

Reference it in a bundle with a templated `dest`:

```yaml
# bundles/core.yaml
assets:
  - type: file
    path: assets/files/agents-file.md
    dest: "{{ target_agents_file }}"
```

When installed to Claude Code, this file lands at `~/.claude/CLAUDE.md`. When installed to Codex, it lands at `~/.codex/AGENTS.md`. Same source file, correct destination per target.

### How dest resolution works with templates

The `dest` field is template-processed before the path is interpreted:

1. Template variables are resolved (e.g., `{{ target_agents_file }}` becomes `CLAUDE.md`)
2. The resolved path is interpreted using standard dest rules:
   - Has extension (e.g., `CLAUDE.md`) - treated as full file path
   - No extension (e.g., `config`) - treated as directory, original filename appended
   - Ends with `/` - explicitly a directory. A dot-named directory needs it: `.claude-plugin` is read as a file name, like `.gitignore`, and `.claude-plugin/` is the directory

You can combine target variables with static paths:

```yaml
assets:
  - type: file
    path: assets/files/notes.md
    dest: "{{ target_name }}-config"    # Resolves to "claude-config" or "codex-config" (directory)
```

## Conditional content

Use `{% if %}` with `target_name` to include or exclude content based on the target:

```markdown
# Project Instructions

## Common Rules

- Always write tests for new features
- Follow the project's existing code style
- Handle errors explicitly - no silent failures

{% if target_name == "claude" %}
## Claude-Specific

Use `/output-style` to switch between personas.
Use `/skill` to invoke skills.
Prefer the Edit tool over Write for modifying existing files.
{% endif %}

{% if target_name == "codex" %}
## Codex-Specific

Codex runs headless - keep responses concise.
Reference persona files in the personas/ directory for behavioral guidance.
Always complete the full task in a single response.
{% endif %}
```

When installed to Claude Code, the Codex section is omitted entirely. When installed to Codex, the Claude section is omitted. The "Common Rules" section appears in both.

### Using elif for multiple targets

For assets that need to handle several targets:

```markdown
{% if target_name == "claude" %}
## Setup for {{ target_name_display }}
Configure output styles in the {{ target_name_display }} settings panel.
{% elif target_name == "codex" %}
## Setup for {{ target_name_display }}
Codex reads configuration from {{ target_agents_file }}.
{% else %}
## Setup
See your AI tool's documentation for configuration details.
{% endif %}
```

The `{% else %}` branch handles any future target that doesn't have a specific section yet.

## Target-specific composition with include_asset

Combining `{% include_asset %}` with target conditionals lets you compose entirely different documents from shared building blocks.

### Example: target-specific project file

Some targets need different information. Claude Code benefits from detailed tool usage instructions. Codex needs a self-contained persona inlined directly because it doesn't natively support output-styles.

Create `assets/files/project-instructions.md`:

```markdown
# Project Instructions

## Core Guidelines

{% include_asset "files/common-guidelines.md" %}

{% if target_name == "codex" %}
## Persona

{% include_asset "personas/senior-swe.md" %}
{% endif %}

{% if target_name == "claude" %}
## Tool Usage

{% include_asset "files/claude-tool-tips.md" %}
{% endif %}
```

Create the shared building blocks:

`assets/files/common-guidelines.md`:

```markdown
- Write clean, well-tested code
- Handle errors explicitly
- Follow existing patterns in the codebase
```

`assets/files/claude-tool-tips.md`:

```markdown
- Prefer Edit over Write for existing files
- Use Glob to find files by pattern before reading
- Use Grep to search content before making changes
```

When installed to **Claude Code**, the output includes common guidelines and tool tips, but not the inlined persona (Claude has output-styles for that).

When installed to **Codex**, the output includes common guidelines and the persona body inlined directly (since Codex doesn't consume personas as first-class config).

### Bundle configuration

```yaml
# bundles/core.yaml
assets:
  - type: file
    path: assets/files/project-instructions.md
    dest: "{{ target_agents_file }}"
  - type: persona
    path: assets/personas/senior-swe.md
  - type: file
    path: assets/files/common-guidelines.md
    dest: ""
  - type: file
    path: assets/files/claude-tool-tips.md
    dest: ""
```

Note that the persona is still included as a standalone asset too - it installs as an output-style on Claude, giving users both the standalone persona and the integrated instructions.

## A complete multi-target example

Here is a full setup that works across Claude Code and Codex from the same config repo.

### Profiles

```yaml
# profiles/claude-dev.yaml
name: claude-dev
target: claude
bundles:
  - core
```

```yaml
# profiles/codex-dev.yaml
name: codex-dev
target: codex
bundles:
  - core
```

### The bundle

```yaml
# bundles/core.yaml
name: core
description: Development assets for all targets

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: file
    path: assets/files/project-instructions.md
    dest: "{{ target_agents_file }}"
```

### The project instructions file

```markdown
{# This file adapts itself based on the target #}
# {{ target_name_display }} Project Configuration

Welcome to the project. You are working with {{ target_name_display }}.

## Guidelines

- Write idiomatic code following existing patterns
- Always handle errors explicitly
- Write tests for new functionality

{% if target_name == "codex" %}
## Behavioral Guide

The following persona applies to all interactions:

{% include_asset "personas/senior-swe.md" %}
{% endif %}

{% if target_name == "claude" %}
## Working with Claude

- Use the Senior SWE output style for code tasks
- Use the Code Reviewer agent for review requests
- Prefer editing existing files over creating new ones
{% endif %}
```

### Installing

```bash
# Install for Claude Code
tai install claude-dev

# Or install for Codex
tai install codex-dev
```

The same bundle, same assets, same config repo - but each target gets appropriately tailored output.

## Next steps

- [Templating](/tai/guides/templating/) - full templating guide including variables, verbatim blocks, and existing
- [Switching & Managing Profiles](/tai/guides/managing-profiles/) - manage multiple profiles for different targets
- [Claude Code target](/tai/targets/claude-code/) - Claude-specific adapter details
- [Codex target](/tai/targets/codex/) - Codex-specific adapter details
- [OpenCode target](/tai/targets/opencode/) - OpenCode-specific adapter details
