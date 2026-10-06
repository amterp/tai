---
title: Codex
description: How tai's Codex adapter maps assets, handles unsupported types, and installs configuration
---

Codex is tai's second supported target. The Codex adapter translates tai's vocabulary into Codex's formats. Codex does not natively support every tai asset type: personas are remapped onto skills (so they remain functional) and delegates are installed as passive reference files. In both cases tai warns you per asset.

## Target directory

Assets are installed to `$CODEX_HOME` if that environment variable is set. Otherwise, tai defaults to `~/.codex/`.

## Asset mapping

The Codex adapter maps tai asset types to Codex's directory structure:

| Tai asset type | Codex directory | Frontmatter rendered | Notes |
|----------------|-----------------|---------------------|-------|
| `persona` | `skills/` | `name`, `description` | Mapped to skills since Codex has no persona concept; single-file personas are wrapped as `skills/<name>/SKILL.md` (warning per asset) |
| `delegate` | `delegates/` | `name`, `description`, `tools`, `model` | Passive reference file (warning per asset) |
| `skill` | `skills/` | `name`, `description` | First-class support |
| `file` | Location specified by `dest` | None (copied as-is) | |
| `statusline` | N/A | N/A | Unsupported (skipped with warning) |

## Partial support and warnings

Codex has first-class support for `AGENTS.md` and skills. It does not have a native persona concept, and it does not natively consume delegate assets the way Claude Code does.

For personas, each persona is installed under `skills/` as a regular skill so Codex's skill discovery actually picks it up. Single-file personas are wrapped into directory form (`skills/<name>/SKILL.md`); directory-form personas keep their layout. A per-asset warning notes the remapping.

For delegates, tai copies the file to `delegates/` as a passive reference and warns per asset.

For example:

```
WARN: Codex does not support personas - installed as skill instead: skills/senior-swe/SKILL.md
WARN: Codex does not natively consume delegate assets - installed as passive reference file: delegates/code-reviewer.md
```

Personas-as-skills are real, discoverable Codex skills. Delegate files are still installed so they are available as reference material, but Codex will not automatically discover or use them the way Claude Code uses agents.

**StatusLine assets are unsupported entirely.** If a profile includes a statusline asset and targets Codex, the asset is skipped and tai logs a warning. No file is written.

## Content transformation

During installation, the adapter re-renders frontmatter for each asset type:

- **Personas** are transformed as skills - they get `name` and `description` fields.
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

## Related pages

- [Assets](/tai/concepts/assets/) - asset types and frontmatter fields
- [Targets](/tai/concepts/targets/) - overview of the target concept
- [Claude Code](/tai/targets/claude-code/) - the Claude Code target adapter
- [OpenCode](/tai/targets/opencode/) - the OpenCode target adapter
- [Templating](/tai/guides/templating/) - using template variables in assets
