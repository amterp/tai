---
title: Templating
description: How to use variables, conditionals, comments, and asset inlining in tai assets
---

Tai uses Jinja2-style templates (powered by [pongo2](https://github.com/flosch/pongo2)) for processing assets during installation. This lets you inject variables, add conditional sections, inline other assets, and wrap existing file content. Templates are evaluated at install time - the installed files contain the resolved output.

## Variable substitution

Use double curly braces to insert a variable:

```markdown
# Project: {{ project_name }}

Welcome to the {{ project_name }} codebase. This project uses {{ language }}.
```

Spaces inside the braces are optional - `{{project_name}}` works too.

### Passing variables via CLI

Pass variables when installing with `--var`:

```bash
tai install personal --var project_name=MyApp
```

For multiple variables, use comma-separated key=value pairs:

```bash
tai install personal --var project_name=MyApp,language=Go
```

### Variable priority

Variables come from multiple sources, with later sources winning on conflict:

| Source | Priority |
|--------|----------|
| Adapter-provided (`target_name`, `target_name_display`, `target_agents_file`) | Lowest |
| CLI `--var` | Highest |

If you pass `--var target_name=custom`, it overrides the adapter-provided value. In most cases you won't want to do this, but the option exists.

### Undefined variables

Undefined variables render as **empty string**, not as literal `{{ var_name }}`. If an asset uses `{{ project_name }}` and you don't pass that variable, the output will have a blank where the variable was.

## Conditionals

Use `{% if %}`, `{% elif %}`, `{% else %}`, and `{% endif %}` for conditional content:

```markdown
# Build Instructions

{% if language == "Go" %}
Run `go build ./cmd/myapp` to compile.
{% elif language == "Rust" %}
Run `cargo build --release` to compile.
{% else %}
See the project README for build instructions.
{% endif %}
```

Conditionals are most commonly used with [adapter-provided variables](/tai/guides/target-aware-assets/) to create target-specific content:

```markdown
{% if target_name == "claude" %}
## Claude-Specific Setup
Use `/output-style` to switch personas.
{% elif target_name == "codex" %}
## Codex-Specific Setup
Reference personas from the personas/ directory.
{% endif %}
```

You can also use boolean expressions:

```markdown
{% if enable_strict == "true" %}
## Strict Mode
All code must pass linting before review.
{% endif %}
```

## Comments

Use `{# #}` for comments that are stripped from the output:

```markdown
{# This comment won't appear in the installed file #}
# Visible Heading

{# TODO: Add section about error handling #}
Content here is visible.
```

Comments are useful for leaving notes in your asset source files without cluttering the installed output.

## Verbatim blocks

If your asset content needs literal `{{ }}` or `{% %}` syntax in the output (e.g., you're writing documentation about template engines), use `{% verbatim %}`:

```markdown
## Template Syntax Guide

{% verbatim %}
Use {{ variable_name }} to insert a variable.
Use {% if condition %} for conditionals.
{% endverbatim %}
```

The content between `{% verbatim %}` and `{% endverbatim %}` is passed through without any template processing.

Note: pongo2 does not support `{% raw %}`. Always use `{% verbatim %}` instead.

## Asset inlining with include_asset

The `{% include_asset %}` tag inlines another asset's body content at install time. The included asset's frontmatter is stripped - only the body is inserted:

```markdown
# Project Instructions

## Coding Standards

{% include_asset "personas/senior-swe.md" %}

## Review Process

{% include_asset "delegates/code-reviewer.md" %}
```

If `personas/senior-swe.md` contains:

```markdown
---
name: Senior SWE
description: Experienced software engineer.
---

Write clear, idiomatic code. Favor readability over cleverness.
Always consider edge cases and error handling.
```

Then the installed output includes only the body:

```markdown
# Project Instructions

## Coding Standards

Write clear, idiomatic code. Favor readability over cleverness.
Always consider edge cases and error handling.

## Review Process
...
```

### Source-qualified paths

Use the `source:path` format to include assets from other sources:

```markdown
{% include_asset "work-shared:personas/company-style.md" %}
```

Without a source prefix, assets are resolved from the `local` source.

### Recursive processing

Included content is template-processed with the same variables. If an included asset itself uses `{% include_asset %}`, that is resolved too. Tai detects circular includes (A includes B includes A) and fails with a clear error.

### Restrictions

- Only file-shaped assets (personas, delegates, skills, files) can be included. Attempting to include a config asset (statusline, mcp, hook) produces an error.
- The built-in `{% include %}` tag from pongo2/Jinja2 is **disabled** for security. Use `{% include_asset %}` for all inclusion needs - it resolves through tai's source system.

### Combining with conditionals

The most powerful pattern is combining `{% include_asset %}` with conditionals for target-specific composition:

```markdown
# Project Instructions

## Core Guidelines

Always write tests. Always handle errors.

{% if target_name == "codex" %}
## Codex Persona

{% include_asset "personas/company-style.md" %}
{% endif %}

{% if target_name == "claude" %}
## Claude-Specific Notes

Use slash commands for common operations.
{% endif %}
```

This installs the persona content only when targeting Codex, while Claude gets different content. See the [Target-Aware Assets guide](/tai/guides/target-aware-assets/) for more on this pattern.

## The existing variable

The special `{{ existing }}` variable injects the current contents of the target file. This is useful for wrapping existing configuration files without losing their content.

### Basic usage

Create a file asset that wraps existing content:

```markdown
# Tai-Managed Header

These instructions are managed by tai. Do not edit above the line.

---

{{ existing }}

---

# Tai-Managed Footer

Additional instructions added by tai.
```

When installed to a file that already has content (e.g., an existing `CLAUDE.md`), the original content appears in place of `{{ existing }}`.

### Backup and restore

When tai encounters `{{ existing }}`, it:

1. Reads the current content of the target file
2. Creates a backup (e.g., `CLAUDE.md.tai-backup`)
3. Records the backup path in the manifest
4. Installs the templated result with the original content inlined

When you uninstall, tai restores the original file from the backup. Your pre-tai content is preserved.

### Practical example

A common use case is adding project-specific instructions to an existing `CLAUDE.md`:

Create `assets/files/project-wrapper.md`:

```markdown
# Project Standards (managed by tai)

- Always run tests before committing
- Follow the style guide in docs/STYLE.md
- Use conventional commit messages

{{ existing }}
```

Reference it in a bundle:

```yaml
assets:
  - type: file
    path: assets/files/project-wrapper.md
    dest: "CLAUDE.md"
```

If the project already has a `CLAUDE.md` with custom content, that content is preserved inside the wrapper. Uninstalling restores the original file.

### When existing is empty

If the target file doesn't exist yet, `{{ existing }}` resolves to an empty string. The asset installs normally, and uninstalling removes the file entirely (since there was nothing to restore).

## Template variables in dest paths

The `dest` field on file assets also supports template variables:

```yaml
assets:
  - type: file
    path: assets/files/agents-file.md
    dest: "{{ target_agents_file }}"
```

This resolves to `CLAUDE.md` when installing for Claude and `AGENTS.md` for Codex. See [Target-Aware Assets](/tai/guides/target-aware-assets/) for a full walkthrough.

## Practical examples

### A configurable persona

```markdown
---
name: Team Developer
description: Follows {{ team_name }} team conventions.
---

You are a developer on the {{ team_name }} team.

{% if language == "Go" %}
Follow Go conventions: gofmt, standard library where possible, table-driven tests.
{% elif language == "Python" %}
Follow PEP 8. Use type hints. Prefer pytest over unittest.
{% endif %}

{% if strict_mode == "true" %}
All code must pass CI before merging. No exceptions.
{% endif %}
```

Install with:

```bash
tai install work --var team_name=Platform,language=Go,strict_mode=true
```

### Composing a project instructions file

```markdown
{# Assemble project instructions from multiple assets #}
# Project Instructions

## Coding Style

{% include_asset "personas/senior-swe.md" %}

## Review Guidelines

{% include_asset "delegates/code-reviewer.md" %}

## Deployment

{% include_asset "skills/deploy-checklist/SKILL.md" %}
```

This produces a single file that combines content from multiple assets, with all frontmatter stripped.

## Next steps

- [Target-Aware Assets](/tai/guides/target-aware-assets/) - use adapter variables for cross-target assets
- [Template Syntax reference](/tai/reference/template-syntax/) - complete syntax reference
- [Creating Personas, Delegates & Files](/tai/guides/creating-assets/) - asset creation basics
