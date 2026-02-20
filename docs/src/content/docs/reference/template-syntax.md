---
title: Template Syntax
description: Complete reference for tai's pongo2/Jinja2 template engine
---

Tai uses [pongo2](https://github.com/flosch/pongo2) (a Jinja2-style template engine for Go) to process asset files during installation. Templates enable variable substitution, conditional content, comments, and asset inlining.

Template processing happens early in the [install pipeline](/tai/reference/installation-behavior/) - before frontmatter parsing and adapter transformation.

## Variables

Variables use double curly braces. Spaces around the variable name are optional.

```
{{ variable_name }}
{{variable_name}}
```

Both forms are equivalent. Variables are replaced with their values during template processing.

```markdown
# Project: {{ project_name }}

Welcome to the {{ project_name }} codebase.
```

### Undefined variables

Undefined variables render as an empty string. They do not cause errors and do not leave the literal `{{ }}` syntax in the output.

```markdown
# {{ undefined_var }} is here
```

Renders as:

```markdown
#  is here
```

Use `{% verbatim %}` to preserve literal `{{ }}` syntax in the output.

## Conditionals

Conditionals use `{% if %}`, `{% elif %}`, `{% else %}`, and `{% endif %}` tags.

```markdown
{% if target_name == "claude" %}
## Claude-Specific Instructions
Follow Claude Code conventions.
{% elif target_name == "codex" %}
## Codex-Specific Instructions
Follow Codex conventions.
{% else %}
## General Instructions
Follow standard conventions.
{% endif %}
```

Conditions support standard comparison operators (`==`, `!=`, `>`, `<`, etc.) and boolean logic (`and`, `or`, `not`).

```markdown
{% if target_name == "claude" and project_type == "go" %}
Use Go idioms and Claude Code formatting.
{% endif %}
```

## Comments

Comments use `{# #}` and are stripped entirely from the output.

```markdown
{# This comment will not appear in the installed file #}
# Visible Heading

{# TODO: Add more instructions later #}
Content here.
```

Comments can span the remainder of a line but cannot span multiple lines.

## Verbatim blocks

The `{% verbatim %}` / `{% endverbatim %}` tags preserve literal template syntax in the output. Content inside these blocks is not processed.

```markdown
{% verbatim %}
This {{ will not }} be treated as a variable.
These {% tags %} are preserved literally.
{% endverbatim %}
```

This is useful when your asset content needs to contain literal `{{ }}` or `{% %}` syntax, for example when documenting template usage or when the target tool uses similar syntax.

:::note
pongo2 does not support `{% raw %}`. Use `{% verbatim %}` instead.
:::

## Asset inlining

The `{% include_asset %}` tag inlines another asset's body content (with frontmatter stripped) at the point of inclusion.

```markdown
# Project Instructions

{% include_asset "personas/senior-swe.md" %}

## Additional Rules
Always write tests.
```

### Source-qualified paths

Use the `source:path` format to include assets from specific sources:

```markdown
{% include_asset "work-shared:personas/company-style.md" %}
```

Without a source prefix, the `local` source is used.

### What gets included

The included asset's body content is inserted - its frontmatter is stripped. This means you get the instructional content without duplicate metadata.

### Restrictions

- **Only file-shaped assets** can be included: personas, delegates, skills, and files. Attempting to include a config asset (statusline) produces a hard error.
- **The `{% include %}` tag is disabled** for security. The built-in pongo2 `{% include %}` would allow arbitrary filesystem access. Use `{% include_asset %}` instead, which goes through tai's source resolution.
- **Absolute paths are not allowed.** Use relative asset paths like `personas/foo.md`.

### Recursive processing

Included content is template-processed with the same variables, so included assets can themselves use conditionals, variables, and further `{% include_asset %}` calls.

```markdown
{# personas/full-instructions.md #}

{% include_asset "personas/base-rules.md" %}

{% if target_name == "claude" %}
{% include_asset "personas/claude-extras.md" %}
{% endif %}
```

### Cycle detection

Circular includes are detected and produce a hard error. If asset A includes asset B which includes asset A, tai reports the cycle chain:

```
include_asset cycle detected: local:personas/a.md -> local:personas/b.md -> local:personas/a.md
```

Cycle detection normalizes references, so `personas/a.md` and `local:personas/a.md` are treated as the same asset.

### Nesting depth limit

To guard against pathological (non-circular) chains of includes, tai enforces a maximum nesting depth of 50 levels.

## Special variable: {{ existing }}

The `{{ existing }}` variable injects the current contents of the target file. This enables wrapping existing configuration without losing user content.

```markdown
# Tai Header

{{ existing }}

# Tai Footer
```

When installed to a file that already contains content, the result includes the original content in place of `{{ existing }}`.

### Backup behavior

When tai processes a file using `{{ existing }}`:

1. The original target file is backed up to `<filename>.tai-backup` (e.g., `CLAUDE.md.tai-backup`)
2. The backup path is recorded in the installation manifest
3. On uninstall, the original content is restored from the backup

This ensures users do not lose their customizations when uninstalling tai.

### Conflict handling

Files using `{{ existing }}` are not treated as conflicts even when an unmanaged file exists at the target path. This is by design - the whole point of `{{ existing }}` is to wrap existing content.

### No existing file

If the target file does not exist when `{{ existing }}` is processed, it renders as an empty string. No backup is created.

### Restriction

`{{ existing }}` cannot be used in `dest` path templates. It is only valid in file content. Attempting to use it in a `dest` field produces an error.

## Variable sources and priority

Variables come from multiple sources. When the same variable name appears in multiple sources, the higher-priority source wins.

| Source | Priority | Description |
|--------|----------|-------------|
| Adapter-provided | Lowest | Target-specific variables (e.g., `target_name`) |
| CLI `--var` | Highest | User-provided via `--var key=val` on the command line |

Future versions may add intermediate priority levels (e.g., tai.yaml globals, profile options).

### Adapter-provided variables

Each target adapter provides variables with the `target_` prefix:

| Variable | Claude | Codex | OpenCode | Description |
|----------|--------|-------|----------|-------------|
| `target_name` | `claude` | `codex` | `opencode` | Adapter name (lowercase) |
| `target_name_display` | `Claude` | `Codex` | `OpenCode` | Display name (capitalized) |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` | Global instructions filename |

These variables enable writing target-aware assets. For example:

```markdown
See the project's {{ target_agents_file }} for conventions.
```

### CLI variables

Pass variables at install time with `--var` (short: `-v`):

```bash
tai install work --var project_name=MyApp,team=backend
```

The format is `key1=val1,key2=val2`. CLI variables override adapter-provided variables with the same name.

## Dest path templates

File assets can use template variables in the `dest` field:

```yaml
assets:
  - type: file
    path: files/agents.md
    dest: "{{ target_agents_file }}"
```

This resolves to `CLAUDE.md` when targeting Claude and `AGENTS.md` when targeting Codex, enabling a single file asset definition to install to different locations per target.

### Dest interpretation after template resolution

After variables are substituted, the resolved dest is interpreted as:

- **Has extension in basename** (e.g., `CLAUDE.md`, `subdir/app.yaml`) - treated as a full file path
- **No extension** (e.g., `config`, `subdir`) - treated as a directory; the source filename is appended
- **Trailing `/`** (e.g., `output/`) - explicitly a directory

### Error handling

If a dest template resolves to an empty string (because all variables are undefined), tai exits with an error rather than silently installing to the target root. The error message includes the original template for debugging.

## Template detection

Tai only runs template processing on files that contain template syntax (`{{ }}`, `{% %}`, or `{# #}`). Files without any template syntax skip the template engine entirely, avoiding any performance overhead or accidental interpretation.
