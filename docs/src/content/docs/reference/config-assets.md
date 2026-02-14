---
title: Config Assets (StatusLines)
description: Reference for config assets that merge into target JSON settings files
---

Config assets are entries merged into target JSON configuration files, rather than written as standalone files. Currently, StatusLines are the only implemented config asset type. MCPs and hooks are planned for v0.2.

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

### Config asset installation scope

StatusLine installation scope is controlled by the profile's `scope` field. See [Profile Format](/tai/reference/profile-format/#scope) for details.

| Scope | Claude target file |
|-------|-------------------|
| `user` | `~/.claude/settings.json` |
| `project` | `.claude/settings.json` |

---

## Planned config assets (v0.2)

The following config asset types are planned but not yet implemented. The formats below reflect the current design and may change before implementation.

:::caution
MCPs and hooks are not yet implemented. The information below describes the planned design for v0.2.
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

### Hooks (planned)

Hook definitions will be YAML files in `assets/hooks/` that define event-triggered shell commands.

#### Planned hook format

```yaml
# assets/hooks/format-on-save.yaml
event: PostToolUse
matcher: Edit|Write
command: prettier --write "$FILE"
```

```yaml
# assets/hooks/security-check.yaml
event: PreToolUse
matcher: Bash
command: ~/scripts/check-dangerous-commands.sh
```

#### Planned hook fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | no | Override filename as hook identifier |
| `event` | string | yes | Event that triggers the hook |
| `matcher` | string | no | Tool name pattern (`Tool1\|Tool2`). Empty matches all tools. |
| `command` | string | yes | Shell command to execute |

#### Planned hook events (Claude Code)

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool executes |
| `PostToolUse` | After a tool executes |
| `UserPromptSubmit` | When the user submits a prompt |
| `Notification` | When Claude sends a notification |
| `Stop` | When Claude finishes responding |
| `SubagentStop` | When a subagent completes |
| `SessionStart` | When a session begins |
| `SessionEnd` | When a session ends |

#### Planned Claude hook installation

Hooks would be installed to different files depending on scope:

| Scope | Target file |
|-------|-------------|
| `project` | `.claude/settings.json` |
| `user` | `~/.claude/settings.json` |
| `local` | `.claude/settings.local.json` |

Installed as entries under the `hooks` key:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "prettier --write \"$FILE\""
          }
        ]
      }
    ]
  }
}
```
