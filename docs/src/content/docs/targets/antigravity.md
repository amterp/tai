---
title: Antigravity
description: Install skills, custom agents, hooks, and other configuration for Antigravity CLI
---

Set `target: antigravity` in your [profile](/tai/concepts/profiles/). Run
`tai run antigravity [args...]` to launch `agy` with the supplied arguments.

## Files

| Asset | Installed at |
|-------|--------------|
| Skill | `~/.gemini/config/skills/<name>/SKILL.md` |
| Delegate | `~/.gemini/config/agents/<name>.md` |
| Directory delegate | `~/.gemini/config/agents/<name>/agent.md` |
| File with `dest: AGENTS.md` | `~/.gemini/AGENTS.md` |

Skills and delegates get `name` and `description`. When the source omits the description, tai uses
the name. Delegates accept `model: inherit`, `model: flash` and `model: pro`. To restrict a delegate's
tools, set `native.antigravity.tools` to a YAML list of Antigravity tool names. Native fields
override the generated frontmatter.

The template variables are `target_name: antigravity`, `target_name_display: Antigravity`, and
`target_agents_file: AGENTS.md`.

tai installs file assets at user scope. Create workspace skills and agents directly under
`.agents/`. Persona and approval assets are unsupported and tai skips them with a warning when
they apply to this target. [Antigravity customization discovery](https://antigravity.google/docs/customization-discovery/)

## Config assets

| Asset | User scope | Project scope |
|-------|------------|---------------|
| Hook | `~/.gemini/config/hooks.json` | `.agents/hooks.json` |
| MCP server | `~/.gemini/config/mcp_config.json` | `.agents/mcp_config.json` |
| Status line and settings | `~/.gemini/antigravity-cli/settings.json` | Unsupported |

Project paths are relative to the directory where you run `tai install`. Antigravity has no
`local` scope in tai.

### Hooks

By default, tai uses the hook asset's filename without its extension as the top-level group name in
`hooks.json`. Set `native.antigravity.group` to a nonblank string to give several assets the same
group name.

```yaml
on: session_start
run: my-tool start
native:
  antigravity:
    group: my-tool
```

tai appends each hook to its event's array in the group. Removing an asset removes its hook and
preserves other hooks in the group. When that removal leaves an event or group empty, tai removes
the empty event or group, even if it was created by hand.

Match tools with `tools` or `match.antigravity` on `pre_tool_use` and `post_tool_use`. Each
command runs from the directory containing `hooks.json`. Read the workspace from the hook
payload's `cwd` field.

**A `PreToolUse` command must print a decision.** When it has no objection, print
`{"decision":"allow"}` to pass the call to the next hook or normal permission checks. Missing
stdout or `{}` denies the call. Use `/hooks` in Antigravity to check that it loaded your hook.
[Antigravity hooks](https://antigravity.google/docs/hooks/)

### Status line and settings

A status line asset owns one key in `settings.json`. Use a command:

```yaml
level: custom
command: ~/.local/bin/my-statusline
padding: 1
```

You can also use a native JSON object for options such as `stack_with_default`. The built-in
status line levels in tai apply to Claude Code. Put other Antigravity settings under
`native.antigravity` in a `settings` asset. [Antigravity status line](https://antigravity.google/docs/cli/statusline/)

### MCP

tai installs each server under `mcpServers` and maps a remote server's `url` to `serverUrl`.
Use `native.antigravity` for additional server fields. tai writes `${VAR}` references literally
and warns because Antigravity documents interpolation for plugins only.
[Antigravity MCP](https://antigravity.google/docs/cli/mcp/)
