---
title: Targets
description: Targets are the AI tools tai installs to - each with an adapter that translates tai concepts
---

Targets are the AI tools tai installs configuration to. Each target has an adapter that translates tai's terminology and asset formats into the target's native format.

## Supported targets

| Target | Adapter name | Config directory | Status |
|--------|-------------|------------------|--------|
| Antigravity | `antigravity` | `~/.gemini/` | Supported |
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

| Tai type | Claude Code | Codex | OpenCode | Antigravity |
|----------|------------|-------|----------| ------------- |
| `persona` | `output-styles/` | `skills/` (mapped to skills) | `agents/` | Unsupported |
| `delegate` | `agents/` | `agents/` (custom agent TOML) | `agents/` | `config/agents/` (Markdown) |
| `skill` | `skills/` | `skills/` | `skills/` | `config/skills/` |
| `file` | Location from `dest` | Location from `dest` | Location from `dest` | Location from `dest` |
| `statusline` | `settings.json` (merged) | Not supported (skipped) | Not supported (skipped) | `antigravity-cli/settings.json` (merged) |

Codex has no native persona concept, so personas are installed as skills (single-file personas are wrapped as `skills/<name>/SKILL.md`), with a warning per asset. Delegates become Codex custom agents: TOML files in `agents/`, with the delegate's body as the agent's instructions.

OpenCode maps both personas and delegates to `agents/`, using a `mode: subagent` field to distinguish delegates from primary agents.

## Template variables

Adapters provide target-specific variables with the `target_` prefix. These can be used in asset content and `dest` paths to make assets target-aware.

| Variable | Claude | Codex | OpenCode | Antigravity |
|----------|--------|-------|----------| ------------- |
| `target_name` | `claude` | `codex` | `opencode` | `antigravity` |
| `target_name_display` | `Claude` | `Codex` | `OpenCode` | `Antigravity` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` | `AGENTS.md` |

For example, a single file asset can install to the right filename for each target:

```yaml
assets:
  - type: file
    path: assets/files/agents.md
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

**Antigravity** renders `name` and `description` for skills and delegates. It also renders
supported delegate models and merges fields from `native.antigravity`.

## Config asset support

Not all targets support all config asset types:

| Config asset | Claude Code | Codex | OpenCode | Antigravity |
|-------------|------------|-------|----------| ------------- |
| `statusline` | Merged into `settings.json` | Not supported (skipped) | Not supported (skipped) | `antigravity-cli/settings.json` |
| `hook` | Merged into `settings.json` | Merged into `hooks.json` | Not supported (skipped) | `config/hooks.json` or `.agents/hooks.json` |
| `mcp` | Merged into `.mcp.json` or `~/.claude.json` | Merged into `config.toml` (asks first) | Merged into `opencode.json` | `config/mcp_config.json` or `.agents/mcp_config.json` |

Claude Code, Codex, and Antigravity support declarative hooks. tai translates the same hook
asset into each target's schema. Hook commands receive target-specific input and return
target-specific output. OpenCode uses a JS plugin API, so tai skips hook assets there.

An event a target does not have (Codex has no notification event, for instance) skips just that hook, with a warning.

MCP servers install on every target. Codex is the one that asks first: it has no dedicated MCP file, so servers go in `config.toml` beside settings tai does not own, and writing it loses comments and key order. tai explains that and remembers your answer.

## Detailed target pages

For target-specific details including directory layout, config file paths, and scope handling, see:

- [Claude Code](/tai/targets/claude-code/)
- [Codex](/tai/targets/codex/)
- [OpenCode](/tai/targets/opencode/)
- [Antigravity](/tai/targets/antigravity/)
