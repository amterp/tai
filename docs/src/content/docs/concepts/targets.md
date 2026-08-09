---
title: Targets
description: Targets are the AI tools tai installs to - each with an adapter that translates tai concepts
---

Targets are the AI tools tai installs configuration to. Each target has an adapter that translates tai's terminology and asset formats into the target's native format.

## Supported targets

| Target | Adapter name | Config directory | Status |
|--------|-------------|------------------|--------|
| Claude Code | `claude` | `~/.claude/` | Fully supported |
| Codex | `codex` | `~/.codex/` (or `$CODEX_HOME`) | Supported with caveats |
| OpenCode | `opencode` | `~/.config/opencode/` (or `$XDG_CONFIG_HOME/opencode`) | Fully supported |

## How adapters work

When you install a [profile](/tai/concepts/profiles/), the target adapter handles three key translations:

1. **Path mapping** - tai asset type directories are mapped to target-specific directories (e.g., `personas/` becomes `output-styles/` for Claude)
2. **Content transformation** - tai frontmatter is stripped and replaced with target-specific frontmatter
3. **Template variables** - the adapter provides target-specific variables for use in templates

## Asset mapping

Each adapter maps tai's asset types to the target's directory structure:

| Tai type | Claude Code | Codex | OpenCode |
|----------|------------|-------|----------|
| `persona` | `output-styles/` | `skills/` (mapped to skills) | `agents/` |
| `delegate` | `agents/` | `delegates/` (passive reference) | `agents/` |
| `skill` | `skills/` | `skills/` | `skills/` |
| `file` | Location from `dest` | Location from `dest` | Location from `dest` |
| `statusline` | `settings.json` (merged) | Not supported (skipped) | Not supported (skipped) |

Codex has no native persona concept, so personas are installed as skills (single-file personas are wrapped as `skills/<name>/SKILL.md`). Codex does not natively consume delegates either, so they are installed as passive reference files that you can point Codex to manually. Tai warns per asset in both cases.

OpenCode maps both personas and delegates to `agents/`, using a `mode: subagent` field to distinguish delegates from primary agents.

## Template variables

Adapters provide target-specific variables with the `target_` prefix. These can be used in asset content and `dest` paths to make assets target-aware.

| Variable | Claude | Codex | OpenCode |
|----------|--------|-------|----------|
| `target_name` | `claude` | `codex` | `opencode` |
| `target_name_display` | `Claude` | `Codex` | `OpenCode` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` |

For example, a single file asset can install to the right filename for each target:

```yaml
assets:
  - type: file
    path: files/agents.md
    dest: "{{ target_agents_file }}"
    # Resolves to CLAUDE.md for Claude, AGENTS.md for Codex
```

## Frontmatter transformation

The adapter determines what frontmatter appears in installed files:

**Claude Code** renders:
- Personas: `name`, `description`
- Delegates: `name`, `description`, `tools`, `model`
- Skills: `name`, `description`, `tools`, `model`
- Files: no transformation (byte-for-byte copy)

**Codex** renders:
- Personas: `name`, `description`
- Delegates: `name`, `description`, `tools`, `model`
- Skills: `name`, `description`
- Files: no transformation (byte-for-byte copy)

**OpenCode** renders:
- Personas: `description` only (no `name` - OpenCode uses filename)
- Delegates: `description`, `mode: subagent`, `model` (no `name`, no `tools`)
- Skills: `name`, `description`
- Files: no transformation (byte-for-byte copy)

## Config asset support

Not all targets support all config asset types:

| Config asset | Claude Code | Codex | Gemini CLI | OpenCode |
|-------------|------------|-------|-----------|----------|
| `statusline` | Merged into `settings.json` | Not supported (skipped) | Not supported (skipped) | Not supported (skipped) |
| `hook` | Merged into `settings.json` | Merged into `hooks.json` | Merged into `settings.json` | Not supported (skipped) |
| `mcp` | Planned | Planned | Planned | Planned |

Hooks are portable across Claude, Codex and Gemini: all three structure them the same way, though Gemini renames every event and expects timeouts in milliseconds rather than seconds. tai names events itself and each adapter translates, so one hook asset works on all three. OpenCode has no declarative hooks - only a JS plugin API - so hook assets are skipped there.

An event a target does not have (Gemini has no subagent events, Codex no notification event) skips just that hook, with a warning.

## Detailed target pages

For target-specific details including directory layout, config file paths, and scope handling, see:

- [Claude Code](/tai/targets/claude-code/)
- [Codex](/tai/targets/codex/)
- [OpenCode](/tai/targets/opencode/)
