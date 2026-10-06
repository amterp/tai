---
title: Config Assets
description: Reference for config assets - status lines and hooks - that merge into a target's config files
---

Config assets are entries merged into a target's configuration files, rather than written as standalone files. Status lines, hooks, and MCP servers are all config assets.

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

tai names events itself rather than adopting one target's vocabulary, because targets differ in which events they have and are free to name them differently.

| tai event | When it fires | Claude | Codex |
|-----------|---------------|--------|-------|
| `session_start` | A session begins | SessionStart | SessionStart |
| `session_end` | A session ends | SessionEnd | - |
| `user_prompt_submit` | A prompt is submitted | UserPromptSubmit | UserPromptSubmit |
| `stop` | The agent finishes responding | Stop | Stop |
| `pre_tool_use` | Before a tool runs | PreToolUse | PreToolUse |
| `post_tool_use` | After a tool runs | PostToolUse | PostToolUse |
| `permission_request` | A permission is requested | PermissionRequest | PermissionRequest |
| `subagent_start` | A subagent starts | SubagentStart | SubagentStart |
| `subagent_stop` | A subagent finishes | SubagentStop | SubagentStop |
| `pre_compact` | Before context is compacted | PreCompact | PreCompact |
| `post_compact` | After context is compacted | PostCompact | PostCompact |
| `notification` | The target sends a notification | Notification | - |

A `-` means the target has no equivalent, and a hook using that event is skipped there with a warning. That way one hook asset can be shared by a profile installing to several targets.

### Tools

`tools` narrows a tool event using tai's canonical names, translated per target:

| tai | Claude | Codex |
|-----|--------|-------|
| `read` | Read | - |
| `write` | Write | - |
| `edit` | Edit | apply_patch |
| `shell` | Bash | Bash |

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

Write `timeout` as a duration. tai converts it to the unit each target expects. Claude and Codex both take seconds, but the unit is the adapter's to declare because targets differ, and a mismatch fails silently by hand, turning a 30-second timeout into a 30-millisecond one.

### Target notes

**OpenCode** has no declarative hooks. Its plugin API is JS modules that block by throwing, with no matcher layer and no I/O contract, so hook assets are skipped with a warning rather than partially emulated.

**Codex** will not run a hook until you trust it, and trust is keyed on a hash of the hook definition - so every change re-arms the prompt. tai says so after installing; run `/hooks` in Codex to review. Codex hooks are written to a dedicated `hooks.json` rather than `config.toml`, so tai never rewrites the file holding your model settings and project trust decisions.

### Where hooks are installed

| Target | `user` | `project` | `local` |
|--------|--------|-----------|---------|
| Claude | `~/.claude/settings.json` | `.claude/settings.json` | `.claude/settings.local.json` |
| Codex | `~/.codex/hooks.json` | `.codex/hooks.json` | - |

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

## MCP servers

MCP (Model Context Protocol) server definitions are YAML files in `assets/mcps/`. The filename with its extension stripped becomes the server name.

```yaml
# assets/mcps/github.yaml
description: GitHub MCP server
transport: stdio
command: npx
args:
  - "-y"
  - "@modelcontextprotocol/server-github"
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

```yaml
# assets/mcps/internal-api.yaml
transport: http
url: https://api.internal.example.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
```

### MCP fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `transport` | string | yes | `stdio` (a local command) or `http` (a remote server) |
| `command` | string | stdio | Executable to run |
| `args` | list | no | Arguments to the command |
| `env` | map | no | Environment variables for the server |
| `cwd` | string | no | Working directory. Claude has no equivalent - dropped there with a warning. |
| `url` | string | http | Server endpoint |
| `headers` | map | no | HTTP headers |
| `enabled` | bool | no | Defaults to true. A disabled server is not installed at all. |
| `description` | string | no | For humans; installed only where the target has somewhere to put it |
| `native` | map | no | Per-target fields merged verbatim over the generated entry |

Only `stdio` and `http` are offered, because they are the two every target supports. SSE and WebSocket exist on some and not others, so tai does not pretend to abstract them - use `native` if you need one on a target that has it.

### Environment references

Write environment references as `${VAR}`. tai rewrites them into whatever syntax the target uses - OpenCode, for instance, wants `{env:VAR}`.

tai does not read the variable itself, so the secret stays in your environment rather than being baked into an installed file.

### Per-target shapes

The MCP protocol is shared, but the config shapes are not:

| tai | Claude | OpenCode |
|-----|--------|----------|
| key | `mcpServers` | `mcp` |
| `command` + `args` | separate fields | one `command` array |
| `env` | `env` | `environment` |
| `transport` | `type: stdio\|http` | `type: local\|remote`, required |
| `cwd` | not supported | `cwd` |

tai writes each target's own shape; you write the tai one.

### Where MCP servers are installed

| Target | `user` | `project` | `local` |
|--------|--------|-----------|---------|
| Claude | `~/.claude.json` | `.mcp.json` | `~/.claude.json`, under `projects.<project path>` |
| OpenCode | `~/.config/opencode/opencode.json` | `opencode.json` | `opencode.json` |

:::note
Claude's `project` scope writes `.mcp.json`, meant to be committed and shared with the repo. `local` scope is the private counterpart, so Claude keeps it under the project's own entry in `~/.claude.json` rather than in a file in the repo.

That file also holds your account identity, per-project trust decisions, and a good deal of session state. tai merges into the parsed document, touches only its own entries, and leaves numbers exactly as it found them - but it is worth knowing what else lives there. Use `project` scope if you would rather keep tai away from it.
:::

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

Three consequences worth knowing:

- **Editing an installed entry takes ownership of it.** tai notices the entry no longer matches what it wrote. Reinstalling reports a conflict instead of quietly restoring its own version - pass `--force` to take the entry back, or `--skip-conflicts` to keep your edit and carry on. Uninstalling leaves an edited entry in place entirely.
- **Unchanged means untouched.** Reinstalling an asset that produces the same entries rewrites nothing, so tai does not churn a file the target may also be writing. An entry sharing an array with other tools keeps its position, so the order things run in does not drift between installs.
- **Removing an entry by hand is not an error.** tai simply puts it back on the next install.

### Codex

Codex has no dedicated MCP config file: servers live in `~/.codex/config.toml`, alongside your model settings, `notify` command, and per-project trust levels. Adding one server means reparsing and re-rendering that whole file, and no Go TOML library preserves comments or key order - so the file comes back correct but reformatted.

Because that is a real cost to a file tai does not own, tai asks first:

```
Codex has no separate MCP config file, so servers go in ~/.codex/config.toml,
alongside your model settings, notify command, and project trust levels.

tai reads that file, adds its servers, and writes it back. Your settings
are preserved, but comments and key order are not - the file comes back
reformatted. tai only ever removes the servers it added.

Let tai manage MCP servers in Codex's config.toml?
> Allow, and remember for this machine
  Allow just this once
  Decline
```

Answering "and remember" (or declining) records the decision in `~/.config/tai/preferences.yaml`, so you are asked once per machine. Delete the entry there to be asked again.

Outside a terminal - CI, a script, a pipe - there is nobody to ask, so tai declines and says so. Pass `--allow-config-rewrite` to proceed without the prompt.

`--dry-run` never asks either: a preview must not leave a real answer behind for a later install to reuse. It reports what it would have asked about instead.

Declining is an answer about writing the file, not a request to undo anything. Servers installed under an earlier "yes" stay exactly where they are.

Codex also does no string interpolation at all. Instead it takes environment variable *names* in dedicated fields, which can express a whole-value reference and nothing else:

| tai | Codex |
|-----|-------|
| `env: {GITHUB_TOKEN: "${GITHUB_TOKEN}"}` | `env_vars = ['GITHUB_TOKEN']` |
| `headers: {Authorization: "Bearer ${API_KEY}"}` | `bearer_token_env_var = 'API_KEY'` |
| `headers: {X-Key: "${KEY}"}` | `env_http_headers = {X-Key = 'KEY'}` |

Anything Codex cannot express - an environment key that differs from the variable it reads, or a reference embedded in a longer string like `"https://${HOST}/x"` - is an error rather than a literal `${VAR}` written into the config, which Codex would never expand. Use `native.codex` for those.

---

## Planned config assets

Every config asset type is now implemented. Status lines remain Claude-only, and hooks remain unsupported on OpenCode, for the reasons given above.
