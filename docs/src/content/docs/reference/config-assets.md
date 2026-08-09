---
title: Config Assets
description: Reference for config assets - status lines and hooks - that merge into a target's config files
---

Config assets are entries merged into a target's configuration files, rather than written as standalone files. StatusLines and hooks are implemented; MCP servers are planned.

## StatusLines

Status line assets control the appearance and behavior of the target tool's status bar. They are defined as YAML files in `assets/statuslines/`.

### Tai format vs native format

StatusLine configs can be written in two formats:

| Format | Detection | Description |
|--------|-----------|-------------|
| **Tai format** | Contains a `level` field | Target-agnostic; tai transforms to the target's format |
| **Native format** | No `level` field | Passed through to the target as-is |

Tai format is recommended for portability across targets. Native format is useful for target-specific options that tai does not abstract.

### Tai format

```yaml
# assets/statuslines/detailed.yaml
level: detailed
show:
  - cost
  - model
  - tokens
  - latency
```

```yaml
# assets/statuslines/custom.yaml
level: custom
command: ~/.scripts/statusline.sh
padding: 2
```

```yaml
# assets/statuslines/hidden.yaml
level: hidden
```

### Tai format fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `level` | string | yes | Status line style. See [Level values](#level-values). |
| `show` | list of strings | no | Elements to display. See [Show elements](#show-elements). |
| `command` | string | conditional | Custom status line command. Required when `level` is `custom`. |
| `padding` | integer | no | Padding value. Only valid when `level` is `custom`. |

### Level values

| Level | Description |
|-------|-------------|
| `minimal` | Minimal status information |
| `standard` | Standard set of status information |
| `detailed` | Comprehensive status information |
| `hidden` | Status line is not displayed |
| `custom` | Custom command-driven status line |

### Show elements

| Element | Description |
|---------|-------------|
| `cost` | Session or total cost |
| `model` | Current model name |
| `tokens` | Token usage |
| `latency` | Response latency |
| `context` | Context information (reserved for future use) |

Unknown show elements are silently ignored for forward compatibility.

### Validation rules

| Level | `show` | `command` | `padding` |
|-------|--------|-----------|-----------|
| `minimal` | allowed | not allowed | not allowed |
| `standard` | allowed | not allowed | not allowed |
| `detailed` | allowed | not allowed | not allowed |
| `hidden` | not allowed | not allowed | not allowed |
| `custom` | not allowed | required | allowed |

Violating these rules causes tai to exit with a validation error.

### Native format

If the YAML file does not contain a `level` field, it is treated as native format and passed through to the target without transformation. This allows using target-specific options directly.

```yaml
# Native Claude format - passed through as-is
style: detailed
showCost: true
showModel: true
showTokens: true
```

### Singleton behavior

Only one status line can be active per installation. When multiple statusline assets appear in a profile (from bundles or direct references), the later one overrides the earlier one. Tai logs an info message when such an override occurs.

### Claude adapter transformation

The Claude adapter transforms tai format fields to Claude's `statusLine` JSON format in `settings.json`:

| tai field | Claude JSON equivalent |
|-----------|------------------------|
| `level: minimal` | `"style": "minimal"` |
| `level: standard` | `"style": "standard"` |
| `level: detailed` | `"style": "detailed"` |
| `level: hidden` | `"style": "hidden"` |
| `level: custom` | `"type": "command"` |
| `show: [cost]` | `"showCost": true` |
| `show: [model]` | `"showModel": true` |
| `show: [tokens]` | `"showTokens": true` |
| `show: [latency]` | `"showLatency": true` |
| `command: ...` | `"command": "..."` |
| `padding: N` | `"padding": N` |

When `show` is specified, all four show flags are set explicitly (missing elements are set to `false`).

Installed result in `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "style": "detailed",
    "showCost": true,
    "showModel": true,
    "showTokens": true,
    "showLatency": false
  }
}
```

### Codex adapter

The Codex adapter does not support statusline assets. StatusLine assets targeting Codex are skipped with a warning.

## Hooks

A hook runs a command when something happens in the target. Hook assets are YAML files in `assets/hooks/`; the filename with its extension stripped becomes the hook's name.

```yaml
# assets/hooks/format-on-save.yaml
description: Format files after they are edited
on: post_tool_use
tools: [edit, write]
run: prettier --write "$FILE"
timeout: 30s
```

```yaml
# assets/hooks/security-check.yaml
on: pre_tool_use
tools: [shell]
run: ~/scripts/check-dangerous-commands.sh
```

### Hook fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | no | For humans reading the config repo; targets never see it |
| `on` | string | yes | Canonical tai event that triggers the hook |
| `tools` | list | no | Canonical tool names. Empty matches every tool. |
| `run` | string | yes | Command to execute |
| `timeout` | string | no | Duration such as `30s` or `2m`. Unset leaves the target's own default. |
| `match` | map | no | Raw per-target matcher, keyed by target name |
| `native` | map | no | Per-target fields merged verbatim over the generated handler |

### Events

tai names events itself rather than adopting one target's vocabulary, because the targets disagree. Claude and Codex call an event `PreToolUse`; Gemini calls the same thing `BeforeTool`, and renames every other event too.

| tai event | When it fires | Claude | Codex | Gemini |
|-----------|---------------|--------|-------|--------|
| `session_start` | A session begins | SessionStart | SessionStart | SessionStart |
| `session_end` | A session ends | SessionEnd | - | SessionEnd |
| `user_prompt_submit` | A prompt is submitted | UserPromptSubmit | UserPromptSubmit | BeforeAgent |
| `stop` | The agent finishes responding | Stop | Stop | AfterAgent |
| `pre_tool_use` | Before a tool runs | PreToolUse | PreToolUse | BeforeTool |
| `post_tool_use` | After a tool runs | PostToolUse | PostToolUse | AfterTool |
| `permission_request` | A permission is requested | PermissionRequest | PermissionRequest | - |
| `subagent_start` | A subagent starts | SubagentStart | SubagentStart | - |
| `subagent_stop` | A subagent finishes | SubagentStop | SubagentStop | - |
| `pre_compact` | Before context is compacted | PreCompact | PreCompact | PreCompress |
| `post_compact` | After context is compacted | PostCompact | PostCompact | - |
| `notification` | The target sends a notification | Notification | - | Notification |

A `-` means the target has no equivalent, and a hook using that event is skipped there with a warning. That way one hook asset can be shared by a profile installing to several targets.

### Tools

`tools` narrows a tool event using tai's canonical names, translated per target:

| tai | Claude | Codex | Gemini |
|-----|--------|-------|--------|
| `read` | Read | - | read_file |
| `write` | Write | - | write_file |
| `edit` | Edit | apply_patch | replace |
| `shell` | Bash | Bash | run_shell_command |

A tool a target has no name for is an error, not a silent drop - dropping it would widen the matcher to *every* tool, so a hook meant to guard edits would start firing on reads. Use `match` instead.

### Escape hatches

Anything the canonical vocabulary cannot express goes in `match` or `native`, keyed by target name:

```yaml
on: pre_tool_use
run: guard.sh
match:
  claude: "mcp__memory__.*"    # overrides `tools`, for this target only
native:
  claude:
    async: true                # merged into Claude's handler object
```

### Timeouts

Write `timeout` as a duration. tai converts it per target, because Claude and Codex expect seconds while Gemini expects milliseconds - a mismatch that fails silently by hand, turning a 30-second timeout into a 30-millisecond one.

### Target notes

**OpenCode** has no declarative hooks. Its plugin API is JS modules that block by throwing, with no matcher layer and no I/O contract, so hook assets are skipped with a warning rather than partially emulated.

**Codex** will not run a hook until you trust it, and trust is keyed on a hash of the hook definition - so every change re-arms the prompt. tai says so after installing; run `/hooks` in Codex to review. Codex hooks are written to a dedicated `hooks.json` rather than `config.toml`, so tai never rewrites the file holding your model settings and project trust decisions.

### Where hooks are installed

| Target | `user` | `project` | `local` |
|--------|--------|-----------|---------|
| Claude | `~/.claude/settings.json` | `.claude/settings.json` | `.claude/settings.local.json` |
| Codex | `~/.codex/hooks.json` | `.codex/hooks.json` | - |
| Gemini | `~/.gemini/settings.json` | `.gemini/settings.json` | - |

Entries land under the `hooks` key, alongside any hooks you or other tools have written:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "prettier --write \"$FILE\"",
            "timeout": 30
          }
        ]
      }
    ]
  }
}
```

## Installation scope

Scope comes from the profile's `scope` field, or from an individual asset's own `scope`, which overrides it. It defaults to `user`. See [Profile Format](/tai/reference/profile-format/#scope).

| Scope | Claude target file |
|-------|-------------------|
| `user` | `~/.claude/settings.json` |
| `project` | `.claude/settings.json` |
| `local` | `.claude/settings.local.json` |

Project and local scopes resolve against the directory you run `tai install` from.

## How config assets are merged

tai owns *entries*, not files or keys. Config files are shared: a `settings.json` holds entries written by you and by other tools alongside tai's. So installing merges entries into the parsed document and records exactly what was written; uninstalling takes back precisely those entries and leaves everything else untouched.

Two consequences worth knowing:

- **Editing an installed entry takes ownership of it.** tai notices the entry no longer matches what it wrote and leaves it alone rather than reverting or deleting your change.
- **Unchanged means untouched.** Reinstalling an asset that produces the same entries rewrites nothing, so tai does not churn a file the target may also be writing.

---

## Planned config assets

:::caution
MCP server support is not yet implemented. The design below is provisional and may change.
:::

### MCPs (planned)

MCP (Model Context Protocol) server definitions will be defined as YAML files in `assets/mcps/`. The filename (without extension) becomes the server name unless overridden with a `name` field.

#### Planned MCP format

```yaml
# assets/mcps/github.yaml
type: stdio
command: npx
args:
  - "-y"
  - "@modelcontextprotocol/server-github"
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

```yaml
# assets/mcps/internal-api.yaml
type: http
url: https://api.internal.company.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
```

#### Planned MCP fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | no | Override filename as server name |
| `type` | string | yes | `stdio` or `http` |
| `command` | string | stdio only | Executable path or command |
| `args` | list of strings | no | Command arguments |
| `env` | map | no | Environment variables (supports `${VAR}` expansion) |
| `url` | string | http only | Server endpoint URL |
| `headers` | map | http only | HTTP request headers |

#### Planned Claude MCP installation

MCPs would be installed to different files depending on scope:

| Scope | Target file |
|-------|-------------|
| `project` | `.mcp.json` (project root) |
| `user` | `~/.claude.json` |
| `local` | `.claude.json` (project root) |

Installed as entries under the `mcpServers` key:

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

