---
title: Codex
description: How tai's Codex adapter maps assets, handles unsupported types, and installs configuration
---

Codex is tai's second supported target. The Codex adapter translates tai's vocabulary into Codex's formats, though Codex does not natively consume all tai asset types. Where native support is missing, tai installs assets as passive reference files and warns you about each one.

## Target directory

Assets are installed to `$CODEX_HOME` if that environment variable is set. Otherwise, tai defaults to `~/.codex/`.

## Asset mapping

The Codex adapter maps tai asset types to Codex's directory structure:

| Tai asset type | Codex directory | Frontmatter rendered | Notes |
|----------------|-----------------|---------------------|-------|
| `persona` | `personas/` | `name`, `description` | Passive reference file (warning per asset) |
| `delegate` | `delegates/` | `name`, `description`, `tools`, `model` | Passive reference file (warning per asset) |
| `skill` | `skills/` | `name`, `description` | First-class support |
| `file` | Location specified by `dest` | None (copied as-is) | |
| `statusline` | N/A | N/A | Unsupported (skipped with warning) |

## Partial support and warnings

Codex has first-class support for `AGENTS.md` and skills. However, it does not natively consume persona or delegate assets the way Claude Code does.

When tai installs a persona or delegate to Codex, it:

1. Copies the file to the mapped directory (`personas/` or `delegates/`)
2. Renders frontmatter with the available metadata
3. Prints a warning for each asset explaining that Codex does not natively consume it

For example:

```
WARN: Codex does not natively consume persona assets - installed as passive reference file: personas/senior-swe.md
WARN: Codex does not natively consume delegate assets - installed as passive reference file: delegates/code-reviewer.md
```

These files are still installed so they are available as reference material, but Codex will not automatically discover or use them the way Claude Code uses output-styles and agents.

**StatusLine assets are unsupported entirely.** If a profile includes a statusline asset and targets Codex, the asset is skipped and tai logs a warning. No file is written.

## Content transformation

During installation, the adapter re-renders frontmatter for each asset type:

- **Personas** get `name` and `description` fields.
- **Delegates** get `name`, `description`, `tools`, and `model` fields.
- **Skills** get `name` and `description` fields. Note that unlike Claude Code, Codex skills do not include `tools` or `model` in their frontmatter.
- **Files** receive no transformation. Content is copied as-is.

## Template variables

The Codex adapter provides these variables for use in [templates](/tai/guides/templating/):

| Variable | Value |
|----------|-------|
| `target_name` | `codex` |
| `target_name_display` | `Codex` |
| `target_agents_file` | `AGENTS.md` |

These enable target-aware assets that adapt their content depending on which target they are installed to. User-provided variables (via `--var`) take precedence over adapter-provided ones.

## Running Codex

You can launch Codex through tai using:

```bash
tai run codex [args...]
```

Any arguments after `codex` are passed through to the `codex` CLI command.

## Future targets

Tai's adapter architecture is designed to support additional targets. Cursor and Windsurf are candidates for future adapters. Each new target gets its own adapter that implements the same interface, translating tai's vocabulary into the target's native format.

## Related pages

- [Assets](/tai/concepts/assets/) - asset types and frontmatter fields
- [Targets](/tai/concepts/targets/) - overview of the target concept
- [Claude Code](/tai/targets/claude-code/) - the Claude Code target adapter
- [Templating](/tai/guides/templating/) - using template variables in assets
