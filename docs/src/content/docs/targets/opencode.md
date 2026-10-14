---
title: OpenCode
description: How tai's OpenCode adapter maps assets, handles agent unification, and installs configuration
---

OpenCode is tai's third supported target. The OpenCode adapter translates tai's vocabulary into OpenCode's formats. OpenCode has strong concept overlap with tai - particularly for agents (which unify tai's personas and delegates) and skills.

## Target directory

Assets are installed to `$XDG_CONFIG_HOME/opencode` if that environment variable is set. Otherwise, tai defaults to `~/.config/opencode/`.

## Asset mapping

The OpenCode adapter maps tai asset types to OpenCode's directory structure:

| Tai asset type | OpenCode directory | Frontmatter rendered | Notes |
|----------------|-------------------|---------------------|-------|
| `persona` | `agents/` | `description` | No `name` (OpenCode uses filename) |
| `delegate` | `agents/` | `description`, `mode`, `model` | `mode: subagent` distinguishes from primary agents |
| `skill` | `skills/` | `name`, `description` | Near-identical to tai's format |
| `file` | Location specified by `dest` | None (copied as-is) | |
| `statusline` | N/A | N/A | Unsupported (skipped with warning) |
| `settings` | N/A | N/A | Unsupported (skipped with warning when the asset has a `native.opencode` block, silently otherwise) |
| `approval` | N/A | N/A | Unsupported (skipped with warning when an entry applies to opencode, silently otherwise) |

## Unified agent directory

OpenCode uses a single `agents/` directory for both primary agents and subagents. Tai's personas become primary agents and delegates become subagents, distinguished by the `mode` field in frontmatter.

This means a profile with both `personas/foo.md` and `delegates/foo.md` would produce a path conflict on `agents/foo.md`. Tai's existing conflict detection handles this - the later asset in bundle resolution order wins.

## Content transformation

During installation, the adapter re-renders frontmatter for each asset type:

- **Personas** get `description` only. The `name` field is omitted because OpenCode names agents by filename. If the tai asset has no `description`, the adapter falls back to the `name` value.
- **Delegates** get `description` and `mode: subagent`. If `model` is specified, it is included. Like personas, `name` is omitted and `description` falls back to `name` if missing. The `tools` field is omitted because OpenCode uses a structurally incompatible format (an object with boolean flags like `{ write: false }` vs tai's comma-separated string).
- **Skills** get `name` and `description` fields.
- **Files** receive no transformation. Content is copied as-is.

## Template variables

The OpenCode adapter provides these variables for use in [templates](/tai/guides/templating/):

| Variable | Value |
|----------|-------|
| `target_name` | `opencode` |
| `target_name_display` | `OpenCode` |
| `target_agents_file` | `AGENTS.md` |

These enable target-aware assets that adapt their content depending on which target they are installed to. User-provided variables (via `--var`) take precedence over adapter-provided ones.

## Running OpenCode

You can launch OpenCode through tai using:

```bash
tai run opencode [args...]
```

Any arguments after `opencode` are passed through to the `opencode` CLI command.

## Related pages

- [Assets](/tai/concepts/assets/) - asset types and frontmatter fields
- [Targets](/tai/concepts/targets/) - overview of the target concept
- [Claude Code](/tai/targets/claude-code/) - the Claude Code target adapter
- [Codex](/tai/targets/codex/) - the Codex target adapter
- [Templating](/tai/guides/templating/) - using template variables in assets
