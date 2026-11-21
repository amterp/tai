---
name: tai
description: >
  Manage AI agent configurations with the tai CLI. Use this skill when asked to:
  manage tai config repos, sources, bundles, or profiles; install or uninstall
  configurations to Antigravity, Claude Code, Codex, or OpenCode; create, edit,
  or organize personas, delegates, skills, or files; or create tai-managed skills.
---

# Tai Configuration Management

Tai composes version-controlled assets and installs them to Antigravity, Claude Code, Codex, and
OpenCode.

## Core Concepts

| Term | Purpose |
|------|---------|
| **Asset** | Atomic config unit: persona, delegate, skill, or file |
| **Bundle** | Named collection of assets (a "shopping list") |
| **Profile** | Bundle(s) bound to a specific target |
| **Source** | Location providing assets (local dir or tai repo) |
| **Target** | AI tool to install to (Antigravity, Claude Code, Codex, OpenCode) |

**Data flow:** Sources → Assets → Bundles → Profiles → Target

## Asset Types

Tai uses target-agnostic terminology. Here's how asset types map to current targets:

| Tai Type | Claude | Codex | OpenCode | Antigravity | Purpose |
|----------|--------|-------|----------|-------------|---------|
| `persona` | `output-styles/` | `skills/` (mapped) | `agents/` | unsupported | Behavioral styles and roles |
| `delegate` | `agents/` | `agents/<name>.toml` | `agents/` | `config/agents/` (Markdown) | Specialized helper agents |
| `skill` | `skills/` | `skills/` | `skills/` | `config/skills/` | Procedural capabilities |
| `file` | (per `dest`) | (per `dest`) | (per `dest`) | (per `dest`) | Loose files copied as-is |
| `statusline` | `settings.json` | unsupported | unsupported | `antigravity-cli/settings.json` | Status bar configuration |
| `hook` | `settings.json` | `hooks.json` | unsupported | `config/hooks.json` | Command run on a target event |
| `mcp` | `.mcp.json` / `~/.claude.json` | `config.toml` | `opencode.json` | `config/mcp_config.json` | MCP server |
| `settings` | `settings.json` | unsupported | unsupported | `antigravity-cli/settings.json` | Fields in the target's settings file |
| `approval` | `settings.json` (`autoMode`) | unsupported | unsupported | unsupported | Auto-approval policy |

The last five are **config assets**: they merge as entries into the target's config files instead of being written as files. See "Config Assets" below.

Target-specific notes:
- **Codex**: Personas are mapped to skills since Codex has no persona concept (single-file personas wrap as `skills/<name>/SKILL.md`). Delegates are installed as custom agents (`agents/<name>.toml`, body as `developer_instructions`). A Claude `model` (`opus`, `claude-*`) and `tools` are dropped with a warning; put Codex fields (`model`, `model_reasoning_effort`, `features: {shell_tool: false}`) under `native.codex`, which also silences the matching warning. Directory delegates stay passive reference files under `delegates/`.
- **OpenCode**: Both personas and delegates map to `agents/`, distinguished by `mode: subagent` on delegates.
- **Antigravity**: Use `target: antigravity`. File assets install under `~/.gemini/`, including
  `AGENTS.md` for instructions. A directory delegate's `DELEGATE.md` installs as `agent.md`. Put
  Antigravity tool restrictions in `native.antigravity.tools` as a YAML list. The accepted
  delegate models are `inherit`, `flash`, and `pro`. `tai run antigravity` launches `agy`.

## CLI Commands

### Configuration

```bash
tai config set <path>     # Set active config repo
tai config show           # Show current config and paths
```

`config set` loads the new repo first and refuses one that does not load (the
active config stays as it was). It never loads the *current* repo, so it can
always replace a broken one; `config show`, `status`, `uninstall` and `run` skip
the repo too.

To inspect another config repo without switching, use the global
`--config <repo>` flag or `TAI_CONFIG=<repo>` (flag wins). It never touches
`~/.config/tai/config.yaml`, so prefer it over `config set` for probing. It is
read-only: `list`, `show`, `locate`, `patch list/diff/workspace` and `refresh`
work; `install`, `reinstall`, `uninstall`, `config set`, `patch bless` and
`patch regenerate` refuse, because install manifests are shared by every repo.
`tai run` rejects a `--config` before the target; one after the target is
passed to the target. `config show` reports an override in effect.

```bash
tai list profiles --config ~/other-tai-config
TAI_CONFIG=~/other-tai-config tai show profile work
```

### Discovery

```bash
tai list sources          # List configured sources
tai list bundles          # List available bundles
tai list profiles         # List available profiles
tai list assets           # List all assets
tai list assets --source <name>  # Filter by source
```

### Inspection

```bash
tai show profile <name>   # Show resolved assets for a profile
tai show bundle <name>    # Show assets in a bundle
tai locate <asset-path>   # Print full filesystem path to asset
```

### Installation

```bash
tai install <profile>              # Install profile to target
tai install <profile> --dry-run    # Preview changes (no modifications)
tai install <profile> --diff       # Show diffs for conflicting files
tai install <profile> --force      # Overwrite conflicting files
tai install <profile> --skip-conflicts  # Leave conflicts as-is
tai install <profile> --var key=val     # Pass template variables

tai uninstall <target>             # Remove tai-managed files
tai status [target]                # Show what's installed
tai refresh [source]               # Fetch remote sources without installing
```

### Patches

```bash
tai patch list                # Status of every patch
tai patch diff <name>         # Blessed snapshot vs current upstream
tai patch bless <name>|--all  # Record current upstream as reviewed
tai patch workspace <source>  # Editable copy for authoring/repairing
tai patch regenerate <name>   # Rewrite a patch from workspace edits
```

Patch names are unique per source; use `source:name` when ambiguous.

### Running

```bash
tai run <target> [args...]  # Run target with tai config applied
tai run claude --resume     # Everything after the target goes to it, flags included
```

## Config Repo Structure

```
my-tai-config/
├── tai.yaml              # Config manifest
├── assets/
│   ├── personas/         # Behavioral styles (→ output-styles)
│   ├── delegates/        # Specialized helpers (→ agents)
│   ├── skills/           # Procedural capabilities (directories)
│   ├── files/            # Loose files (copied as-is to dest)
│   ├── statuslines/      # Config asset: status bar
│   ├── hooks/            # Config asset: commands run on target events
│   ├── mcps/             # Config asset: MCP servers
│   ├── settings/         # Config asset: fields in a target's settings file
│   └── approvals/        # Config asset: auto-approval policy (Markdown)
├── bundles/              # Bundle definitions (YAML)
├── patches/              # Local deltas, one dir per patched source
└── profiles/             # Profile definitions (YAML)
```

## Config Assets

Config assets merge as entries into a target's config files rather than being
written as files. tai owns the *entries* it wrote, not the whole key, so its
hooks sit alongside yours and other tools' under the same event, and uninstall
removes only tai's.

Editing an installed entry by hand takes ownership of it: tai sees it no longer
matches what it wrote and leaves it alone.

### Scope

Config assets install at a scope, defaulting to `user`. Set it per profile, or
per asset to override:

```yaml
# profiles/default.yaml
scope: user
assets:
  - type: hook
    path: assets/hooks/format.yaml
  - type: hook
    path: assets/hooks/repo-lint.yaml
    scope: project    # this one goes in ./.claude/settings.json
```

`user` is the target's home config, `project` and `local` resolve against the
working directory. Antigravity hooks and MCP support `project`, using config files in `.agents/`.
Antigravity settings and status line use `user` scope. Antigravity and Codex have no `local` scope.

### Statuslines

One per profile; a second overrides the first. A `level` field means tai format
(translated per target); without one the YAML is passed through natively.

```yaml
# assets/statuslines/bar.yaml
level: detailed        # minimal | standard | detailed | hidden | custom
show: [cost, model]
```

Antigravity accepts `level: custom` with a command or a native JSON object.

### Hooks

A hook runs a command when something happens in the target. The filename with
its extension stripped is the hook's name.

```yaml
# assets/hooks/format-on-save.yaml
description: Format files after they are edited   # optional
on: post_tool_use      # required, canonical tai event
tools: [edit, write]   # optional; empty matches every tool
run: prettier --write "$FILE"
timeout: 30s           # optional duration, converted per target
```

**Events** are named by tai and translated per target, because targets differ
in which events they have and what they call them:

`session_start`, `session_end`, `user_prompt_submit`, `stop`, `pre_tool_use`,
`post_tool_use`, `permission_request`, `subagent_start`, `subagent_stop`,
`pre_compact`, `post_compact`, `notification`, `pre_invocation`, `post_invocation`

A target lacking an event skips that hook with a warning. Codex has no
notification or session-end event.

**Tools** are `read`, `write`, `edit`, `shell`. A tool a target has no name for
is an error, not a silent drop - dropping it would widen the matcher to every
tool. Use `match` instead.

**Escape hatches**, both keyed by target name:

```yaml
match:
  claude: "mcp__memory__.*"   # raw matcher; overrides `tools` for that target
native:
  claude: { async: true }     # merged verbatim into the handler object
```

Write `timeout` as a duration; tai converts it to the unit each target
expects.

**OpenCode has no hooks** (only a JS plugin API), so hook assets are skipped
there. **Codex** will not run a hook until it is trusted, and every change
re-arms that: run `/hooks` in Codex after installing.

**Antigravity** appends each hook to an event array inside a top-level group in `hooks.json`. The
group defaults to the asset's filename without its extension. Set `native.antigravity.group` on
each asset to share a group. Removing a hook also removes the event and group if they become empty.
Match tools only on `pre_tool_use` and `post_tool_use`. A hook runs from the directory containing
`hooks.json`. Read the workspace from the payload's `cwd`. A `PreToolUse` command must print a
decision. Missing stdout or `{}` denies the tool call. Return `{"decision":"allow"}` to defer to
normal permission checks.

### MCP servers

The filename with its extension stripped becomes the server name.

```yaml
# assets/mcps/github.yaml
transport: stdio         # stdio | http - the two every target supports
command: npx
args: ["-y", "@modelcontextprotocol/server-github"]
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

```yaml
# assets/mcps/internal.yaml
transport: http
url: https://api.internal.example.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
```

Other fields: `cwd` (dropped with a warning on Claude, which has none),
`enabled: false` (not installed at all), `description`, and `native` for
per-target passthrough.

Write env references as `${VAR}`; tai rewrites them per target (OpenCode wants
`{env:VAR}`) and never reads the value, so secrets stay in the environment.

Targets keep the same protocol in different shapes, and tai writes each: Claude
keys servers under `mcpServers`, OpenCode uses `mcp` with the command
as one argv array and env spelled `environment`.

**Codex asks before installing MCP servers**: it keeps them in `config.toml`
beside settings tai does not own, and rewriting that file loses comments and key
order. The answer is remembered per machine in
`~/.config/tai/preferences.yaml`; `--allow-config-rewrite` skips the prompt for
scripts. Codex also does no interpolation - it takes env var *names*, so
`TOKEN: "${TOKEN}"` becomes `env_vars`, `Authorization: "Bearer ${KEY}"` becomes
`bearer_token_env_var`, and anything it cannot express is an error rather than a
literal `${VAR}` it would never expand.

At `user` scope, Claude MCP servers go in `~/.claude.json`, which also holds
account and session state. tai merges into it rather than rewriting it, but
`scope: project` writes the dedicated `.mcp.json` if you would rather keep tai
out of that file. `scope: local` is the private counterpart to `project`: it
also lands in `~/.claude.json`, under that project's own entry, since Claude
reads no `.claude.json` from the project directory.

Antigravity uses `mcpServers` with `serverUrl` for remote servers. tai warns and leaves `${VAR}`
references literal because Antigravity documents interpolation only for plugins.

### Settings

```yaml
# assets/settings/afk.yaml - reference as `type: settings`
native:
  claude:                       # the only top-level key is native.<target>
    env:
      CLAUDE_AFK_TIMEOUT_MS: "600000"
    enabledPlugins: { "foo@bar": true }
    defaultMode: auto
    permissions:
      allow: ["Bash(ls:*)"]
```

Use a settings asset instead of a `post_install` script that edits
`settings.json`. Claude and Antigravity support this asset type. tai owns each value
inside an object separately (`env.FOO`, not all of `env`) and appends each array
element, so values from you, other tools and other assets coexist, and uninstall
takes out only tai's. Two settings assets setting the same field fail the
install; `statusLine`, `hooks`, `mcpServers` and the `autoMode` arrays approval
assets own are rejected (use those asset types). An asset with no block for a target is
silently skipped there.

### Approval policies

```markdown
<!-- assets/approvals/personal.md - reference as `type: approval` -->
Notes before the first heading are not installed.

## context: Push policy

An ordinary push to the working repo's existing remote is fine.

## allow claude: Directing agents in Muster

Briefing the agents in my Muster panes is authorized.
```

Each `## <kind> [target...]: <title>` heading is one entry; kinds are
`context`, `allow`, `deny`. On Claude they append to `autoMode.environment`,
`autoMode.allow` and `autoMode.soft_deny` as `<title>: <body>`, user scope only
(other scopes are an error, because the classifier ignores them). An array tai
creates starts with `"$defaults"` so Claude keeps its built-in rules. Use this
instead of a `post_install` script that rewrites `autoMode`. Codex and OpenCode
are not supported yet. Name each repo's file distinctly (e.g. after the repo):
two same-named approval files from different sources are one asset, and the
later replaces the earlier.

## Configuration Files

### tai.yaml

```yaml
version: 1

sources:
  # "local" source is implicit (points to this repo's root)
  # A source's path is a ROOT: nothing is appended, and every reference is a
  # literal path from it. So a config repo's assets read as <src>:assets/...
  - name: work-shared
    path: ~/work/ai-common          # personas/, skills/ ... at its top level
  - name: personal
    path: ~/personal-tai-config     # a config repo: personal:assets/...
  # Command source: fetches its whole asset tree by running a command.
  # Use for private repos behind SSH/token auth. Argv runs with NO shell;
  # wrap in ["sh","-c",...] for shell features. Script must write the asset
  # tree (personas/, skills/, ...) into $TAI_DEST_DIR.
  - name: company
    type: command
    command:
      - sh
      - -c
      - git clone --depth=1 git@github.com:company/ai-standards.git "$TAI_DEST_DIR"
    timeout: 2m           # Optional, default 30s. 'path' is rejected.
  # URL source: downloads ONE file over http(s). Reach for this over a
  # command source wrapping curl, especially when you want to patch the file.
  - name: upstream-guide
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    filename: GUIDE.md    # Optional; defaults to the URL basename
    patches: patches/upstream-guide/

default_profile: personal
```

**Source priority:** Later sources override earlier ones on path conflicts.

**Source types:** `local` (default), `command`, or `url`. `type: tai-repo` was
removed - it appended `/assets` for you; write the `assets/` prefix in the
reference instead.

**Bundles from a source:** anything in `<root>/bundles/` is importable as
`source:bundle-name`. See "Importing bundles" below.

**Command sources:** materialize into `~/.cache/tai/sources/`. `tai install`
and `tai reinstall` re-fetch them automatically; `tai refresh [source]`
fetches without installing (needed on a fresh machine before read-only
commands like `tai list assets` can see the source). Dry-run never fetches.
A failed fetch fails the install and leaves the previous cache intact.
Script env: `TAI_DEST_DIR`, `TAI_SOURCE`, `TAI_CONFIG_REPO`, `TAI_VERSION`,
plus the inherited user env - keep credentials in env vars, not argv.

**URL sources:** same fetch lifecycle as command sources, but they download a
single file and run no code. The file lands in the source's cache directory
under its filename, so the source is a directory holding one file and carries
no assets - reach the file with an include:

```yaml
# assets/skills/mine/tai.yaml
include:
  - from: upstream-guide
    path: GUIDE.md
    dest: refs/guide.md
```

The include receives the source's *patched* content, which is the point:
declare `patches:` on the url source and the skill gets the patched file.
Set `filename:` when the URL has no usable basename (`https://host/`, or a
path that is all query string) - tai rejects the config otherwise.

### Bundle Definition

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

# Each asset requires explicit type and path.
# Paths are relative to the SOURCE ROOT - for your own repo that is the repo
# itself, hence the assets/ prefix.
assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""                              # Required for file type; empty = target root
  - type: skill
    path: assets/skills/my-skill          # Reference skill by directory name

# Include other bundles
bundles:
  - base-setup

# Reference assets from other sources via path prefix
# assets:
#   - type: persona
#     path: work-shared:personas/company-style.md

# Optional: steps run after any install that includes this bundle, so profiles
# don't each repeat them. Same fields as a profile's steps; names required (no
# argv shorthand). Each runs once per install, before the profile's own steps,
# in this repo's root (also TAI_SOURCE_DIR), with its own timeout or 30s.
post_install:
  - name: render
    command: [./scripts/render-approval.sh]  # Relative to this repo's root
```

Use a bundle step when the bundle does nothing until a command runs (a policy
a script renders, a tool that registers itself). The script path stays valid
whichever repo imports the bundle, because the step runs in the root of the
repo holding the bundle's `bundles/` dir - for an imported source, its content
root. A bundle reached through several imports runs each step once; output
names the bundle, e.g. `post_install step "render" from bundle "work:approval"`.
`tai show profile` lists every step that will run. A step from a patched source
runs in the patched view, a copy rebuilt each time tai loads the source: no
`.git`, no excluded files, and nothing the step writes there survives the next
tai command, so write output under `TAI_TARGET_DIR`.

To skip an imported bundle's step on one machine, add `skip_post_install` to
the `bundles:` entry (see the example below). It lists plain step names, reaches
steps of bundles the import includes, and covers that entry only: another entry
reaching the same bundle still runs the step. A name matching no step warns
rather than failing. `tai install` prints a `Skipped:` line for each skip.

### Importing bundles

A `bundles:` entry can name a source, pulling in that source's bundle instead of
restating its contents. An entry is a bare name or a mapping; the mapping form
exists for `exclude` and `skip_post_install`.

```yaml
bundles:
  - core                        # this repo's bundles/core.yaml
  - work:work-tools             # the work source's bundles/work-tools.yaml
  - name: work:extras
    exclude:
      - assets/skills/beagle    # this machine cannot use it
  - name: work:approval
    skip_post_install:
      - render                  # don't run this bundle's "render" step here
```

Imports are read at install time, so assets added upstream arrive on the next
install with nothing to change locally. Copying a bundle instead makes a
snapshot that goes stale without ever erroring.

Inside an imported bundle, unprefixed asset paths and unprefixed included bundle
names resolve against **that bundle's own repo** - upstream's
`assets/skills/dredge` keeps meaning upstream's file even if you hold one at the
same path. A `source:`-prefixed reference resolves in **your** `tai.yaml`, so
you must declare any third source an imported bundle reaches into.

Exclusions apply to the import's fully resolved contents, reaching assets that
came via a nested include. A pattern matching nothing warns rather than failing.
For "never take this from that source at all", use the source's own `exclude:`
in `tai.yaml`; it filters imported bundles quietly, while naming an excluded
asset in your own config stays a hard error.

### Profile Definition

```yaml
# profiles/work.yaml
name: work
description: Full work setup
target: claude

bundles:
  - core
  - work-extras

# Individual assets (in addition to bundles)
assets:
  - type: persona
    path: assets/personas/brainstormer.md

# Optional: named steps run after a successful install - notifications,
# follow-up scripts, or post-processing installed files. Env: TAI_TARGET,
# TAI_TARGET_DIR, TAI_PROFILE, TAI_MANIFEST_DIR, TAI_SOURCE_DIR (config repo
# root, a path - not to be confused with a command source's TAI_SOURCE, a
# source name), TAI_VERSION. Run after any bundles' steps. Every step runs even if an
# earlier one failed. Failure does NOT undo the install; tai prints each
# step's error, lists the failed steps, and exits nonzero.
post_install:
  - name: notify                           # Required, unique in the profile
    command: [./scripts/after-install.sh]  # argv, no shell
  - name: memri
    command: [memri, setup, claude]
    timeout: 2m                            # Optional; overrides post_install_timeout
post_install_timeout: 1m  # Optional, default 30s per step; requires post_install
```

A plain argv list (`post_install: [./scripts/after-install.sh]`) is shorthand
for one unnamed step - the original form, still accepted. Don't mix the two
forms in one list, and don't chain commands with `sh -c 'a; b'` when they can
be separate steps: separate steps fail independently. Unknown step fields are
an error.

A step may modify files tai installed (use `TAI_TARGET_DIR` to find them).
After each step that succeeds, tai re-hashes what the install wrote and
updates the manifest baseline for the files that step changed, so its edits
aren't reported as conflicts on the next install - it prints which managed
files changed. Two exceptions: a *failed* step's edits aren't rebaselined (a
partial edit is a conflict worth seeing, and the file stays held back even if
a later step edits it again), and files skipped via `--skip-conflicts` keep
their stale checksum so they keep conflicting until resolved.

Make such steps idempotent - tai overwrites the file from source on every
install, so the step re-applies its change each run.

## Templating

tai uses Jinja2-style templates (pongo2). Supports variables, conditionals, comments, and asset inlining.

{% verbatim %}### Variables

```markdown
# Project: {{ project_name }}
Welcome to {{ project_name }}.
```

Variables come from multiple sources (later wins): adapter < tai.yaml `vars` < profile `vars` < CLI `--var`.

```yaml
# tai.yaml - project-wide defaults
vars:
  project_name: my-project

# profiles/work.yaml - profile-specific overrides
vars:
  env: production
```

CLI override: `tai install work --var project_name=MyApp`

Undefined variables cause an error. For optional behavior, define with empty string (`vars: { opt: "" }`). Use verbatim blocks for literal template syntax.

A source written for a target rather than for tai (third-party skills, say)
can contain `{#` or `{{` that is not template syntax - a Markdown heading
anchor like `## Usage {#usage}` fails the whole install with "Newline not
permitted in a single-line comment". Set `template: false` on that source in
`tai.yaml` rather than patching each file. It covers the source's own assets,
files a skill includes from it with `from:`, and `include_asset` of it.

### Environment Variables

Access env vars with the `env` filter: `{{ "HOME"|env }}`, `{{ "USER"|env }}`. Missing env vars render as empty string.

### Conditionals

```markdown
{% if target_name == "codex" %}
## Codex-Only Section
{% elif target_name == "claude" %}
## Claude-Only Section
{% else %}
## Default Section
{% endif %}
```

### Asset Inlining

`{% include_asset %}` inlines another asset's body (frontmatter stripped):

```markdown
{% include_asset "personas/senior-swe.md" %}
{% include_asset "work-shared:personas/company-style.md" %}
```

Only file-shaped assets (personas, delegates, skills, files) can be included. Config assets produce an error. Circular includes are detected and fail.

Combine with conditionals for target-specific composition:

```markdown
# Instructions
{% if target_name == "codex" %}
{% include_asset "personas/company-style.md" %}
{% endif %}
```

### Adapter-Provided Variables

Adapters provide target-specific variables with the `target_` prefix:

| Variable | Claude | Codex | OpenCode | Antigravity |
|----------|--------|-------|----------| ------------- |
| `target_name` | `claude` | `codex` | `opencode` | `antigravity` |
| `target_name_display` | `Claude` | `Codex` | `OpenCode` | `Antigravity` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` | `AGENTS.md` |

Use these for target-aware file destinations:

```yaml
assets:
  - type: file
    path: assets/files/AGENTS.md
    dest: "{{ target_agents_file }}"  # -> CLAUDE.md for Claude, AGENTS.md for Codex
```

Variable priority (later wins): adapter < tai.yaml `vars` < profile `vars` < CLI `--var`.

### The `{{ existing }}` Variable

Inject current target file contents (useful for wrapping existing config):

```markdown
# Tai additions
{{ existing }}
# End Tai additions
```

Tai backs up the original and restores it on uninstall.
{% endverbatim %}

## Common Workflows

### Adding a New Asset

1. Create the asset file in `assets/<type>/`
2. Add it to a bundle in `bundles/`
3. Re-install: `tai install <profile>`

### Switching Profiles

```bash
tai install work    # Removes old assets, adds new ones
tai status          # Verify current installation
```

### Editing an Existing Asset

```bash
tai locate personas/senior-swe.md  # Find the file
# Edit the file
tai install <profile>              # Re-install to apply
```

### Editing an Asset You Don't Own

Use a patch (see "Patching Imported Assets" below). Do **not** copy the asset
into the local source - a copy silently stops receiving upstream updates.

---

# Creating Personas & Delegates

Personas and delegates are markdown files with required tai frontmatter.

## Persona/Delegate Format

```markdown
---
name: Senior SWE
description: Experienced software engineer co-owner role.
---

# Senior SWE

You are a senior software engineer...
```

## Frontmatter Fields

| Field | Personas | Delegates | Skills |
|-------|----------|-----------|--------|
| `name` | required | required | required |
| `description` | optional | optional | optional |
| `tools` | — | optional | optional |
| `model` | — | optional | optional |
| `depends` | optional | optional | optional |
| `native` | optional | optional | optional |

Any other field is dropped, with a warning naming it. Schema version is tracked at the repo level (`version: 1` in `tai.yaml`), not per-file.

**Note:** The `file` type has no frontmatter requirements - nothing is parsed or stripped from the top of the file.

### Per-Target Frontmatter

Targets read fields tai has no word for - Claude alone reads
`disable-model-invocation`, `user-invocable`, `allowed-tools`, `effort`,
`argument-hint`, `paths` and `hooks`. Put those under `native`, keyed by
target name; tai merges them verbatim over the frontmatter it generated:

```yaml
---
name: parallel-work
description: Use when several sessions build features in sibling worktrees
native:
  claude:
    disable-model-invocation: true
    allowed-tools: [Read, Bash]
---
```

Installing to Claude writes all four fields. Installing to any other target
writes `name` and `description` only, since `native.claude` is not addressed
to them. It is the same escape hatch hooks and MCP servers use.

For a source written for one target that you do not own (a Claude Code skill
pack, say), set `native_frontmatter: claude` on the source in `tai.yaml`
instead of patching every file. Its assets' unknown fields are then treated as
if written under `native.claude`: Claude gets them, other targets ignore them,
and the "not a tai field" warnings stop. It also applies to a SKILL.md
included from that source with `from:`.

- A field only one target understands goes here. A concept several targets
  share can become a tai field instead - written once, translated per target,
  which is what `tools` and `model` already are.
- A native field beats the tai field of the same name, keeping its position.
- A misspelled target name (`native.cluade`) warns and names the real targets.
- Lists, maps and booleans render as YAML, so nested fields keep their shape.

### Asset Dependencies

Assets can declare dependencies on other assets using the `depends` field. When an asset is included in a bundle or profile, its dependencies are automatically pulled in.

```yaml
---
name: Code Review
description: Reviews code with specialized reviewers
depends:
  - assets/delegates/bug-hunter.md
  - assets/delegates/code-reviewer.md
  - assets/skills/council-participant
---
```

**Syntax:** A dependency is a path from its source root, ending in
`<type-dir>/<name>` - the directory holding the asset is what names its type:

```yaml
depends:
  - assets/delegates/bug-hunter.md         # from local source
  - assets/skills/council-participant      # directory-based skill
  - other-source:delegates/helper.md       # from named source
```

**Resolution rules:**
- Dependencies are resolved depth-first (transitive deps appear before assets that need them)
- Duplicate assets are deduplicated ("later wins")
- Explicit bundle/profile assets override auto-included dependencies
- Circular dependencies cause installation to fail with a clear error
- The `depends` field is stripped on install (never written to target)

### YAML Escaping for Descriptions

If your description contains colons followed by spaces (e.g., `user: "hello"`), YAML will misparse it as a nested mapping. Use a block scalar (`>-`) to avoid this:

```yaml
---
name: my-delegate
description: >-
  Use this agent when... Examples: <example>user: "Do something"
  assistant: "I'll help with that."</example>
---
```

The `>-` indicator means "folded block scalar, strip trailing newlines" — the text on subsequent lines is treated as a single string.

## Files Format

Files get no frontmatter handling - nothing is parsed or stripped, so a leading `---` block passes straight through. Templating still applies, exactly as for other asset types - see **Templating** above. The `dest` field is required and specifies where the file lands:

```yaml
# In a bundle or profile
assets:
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""              # Empty = target root (~/.claude/CLAUDE.md)

  - type: file
    path: assets/files/config.json
    dest: "config"        # Goes to ~/.claude/config/config.json
```

**Key points:**
- `dest` is required for file type
- Empty `dest` (`""`) means target root directory
- A dot in the last segment makes `dest` a file path, a leading dot included:
  `.gitignore` and `.claude-plugin` are file names. For a dot-named directory,
  end with a slash: `dest: "skills/x/.claude-plugin/"` installs
  `skills/x/.claude-plugin/plugin.json`. Without the slash tai installs a file
  named `.claude-plugin` and warns
- Content passes through unchanged except for template rendering

---

# Creating Skills

Skills are the most complex asset type—directories containing a `SKILL.md` with YAML frontmatter plus optional supporting files.

## Skill Structure

```
assets/skills/my-skill/
├── SKILL.md              # Required: frontmatter + instructions
├── scripts/              # Optional: executable code
├── references/           # Optional: documentation loaded as needed
└── assets/               # Optional: templates, output-ready files
```

## SKILL.md Format

```yaml
---
name: my-skill
description: >
  Concise description of what this skill does and when to use it.
  This description triggers skill activation—write it comprehensively.
tools: Read, Glob, Grep
---

# Skill Title

Instructions for the agent...
```

### Frontmatter Fields

| Field | Required | Notes |
|-------|----------|-------|
| `name` | Yes | Human-readable display name |
| `description` | No | Under 1024 chars; triggers skill activation |
| `tools` | No | Comma-separated list of tools the skill may use |
| `model` | No | Model to run this skill with |
| `native` | No | Per-target fields; see **Per-Target Frontmatter** above |

## Core Principles

### 1. Concise is Key

The context window is a shared resource. Only include information the agent doesn't already know.

- Keep SKILL.md under 500 lines
- Move complex content to `references/` for on-demand loading
- Avoid redundant documentation (README, CHANGELOG, etc.)

### 2. Set Appropriate Degrees of Freedom

Match specificity to task fragility:

| Freedom | Format | Use When |
|---------|--------|----------|
| High | Text instructions | Flexible approaches acceptable |
| Medium | Pseudocode | Preferred patterns exist |
| Low | Specific scripts | Fragile/critical operations |

### 3. Progressive Disclosure

Information is loaded in stages:

1. **Metadata** (~100 words) - Always available
2. **SKILL.md body** (<5k words) - Loaded when skill triggers
3. **Bundled resources** - Loaded as needed during execution

### 4. Scripts for Fragile Operations

Put deterministic or fragile operations in `scripts/`:

```
scripts/
├── validate.sh       # Validation logic
└── transform.py      # Data transformation
```

Reference from SKILL.md: "Run `scripts/validate.sh` to verify the configuration."

### 5. References for Large Content

For documentation >10k words, use `references/`:

```
references/
├── api-guide.md
└── examples.md
```

Include grep patterns for large files so agents can search efficiently.

## Creating a New Skill

1. **Understand** the task through concrete examples
2. **Plan** reusable components (scripts, references, assets)
3. **Create** the directory structure under `assets/skills/`
4. **Write** SKILL.md starting with a comprehensive description
5. **Add** supporting files (scripts, references) as needed
6. **Reference** in a bundle: `- type: skill` / `path: assets/skills/my-skill`
7. **Install** and test: `tai install <profile>`

## Description Guidelines

The description determines when your skill activates. Write it to:

- Enumerate the tasks the skill handles
- Use action words (manage, create, configure, etc.)
- Include key terms users might mention
- Be comprehensive without being verbose

**Good:**
```yaml
description: >
  Manage AI agent configurations with the tai CLI. Use when asked to:
  manage tai config repos, sources, bundles, or profiles; install or
  uninstall configurations to targets; create or edit personas,
  delegates, skills, or files.
```

**Bad:**
```yaml
description: A skill for tai.
```

## Skill Includes

Skills can declare external dependencies via a `tai.yaml` manifest. Includes are resolved at install time.

### tai.yaml Format

```yaml
# assets/skills/my-skill/tai.yaml
include:
  # From a named source
  - from: source-name        # Source defined in config's tai.yaml
    path: file.md            # Path within source
    dest: references/        # Trailing "/" preserves filename

  # From a URL
  - url: https://example.com/file.md
    dest: docs/example.md    # Explicit path

  # Copy a directory
  - from: scripts-repo
    path: helpers/
    dest: scripts            # No trailing "/": contents land in scripts/

  # Include with field remapping (for SKILL.md from external sources)
  - from: external-repo
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title            # Source's "title" → tai's "name"
      description: summary   # Source's "summary" → tai's "description"

  # From a command - escape hatch for auth, private APIs, custom protocols.
  # tai runs the argv DIRECTLY with no shell. Use ["sh","-c","..."] for
  # redirection, pipes, env-var expansion, etc.
  # The script MUST write files into $TAI_DEST_DIR; tai treats that dir's
  # contents as the include's output and atomically swaps them into `dest`.
  - command:
      - sh
      - -c
      - "glab api 'projects/123/repository/files/foo%2Fbar/raw?ref=main' > $TAI_DEST_DIR/foo.md"
    dest: refs/              # final location inside the installed skill
    timeout: 60s             # Optional, default 30s
```

### Destination Rules

- `dest: dir/` (trailing slash) → preserves original filename. A directory
  is nested too: `path: helpers/` + `dest: scripts/` → `scripts/helpers/`
- `dest: dir/file.md` (no trailing slash) → exact target path. For a
  directory, `dest: scripts` puts its contents at `scripts/`

### Command Includes

Use `command:` when no built-in mode fits - the script is responsible for
authentication, dependencies, and whatever protocol it speaks. The user's env
is inherited, so secrets in env vars (`$GITLAB_TOKEN`, etc.) are available.

The script must write files into `$TAI_DEST_DIR`. tai treats the directory's
contents as the include's output and atomically swaps them into `dest` on
success. Empty output is an error; non-zero exit or timeout is an error and
preserves the staging dir for debugging.

With `dest: ./` (the skill root) tai merges instead: each top-level file or
directory the script wrote replaces the same path in the skill and the rest
stays. A script writing `$TAI_DEST_DIR/SKILL.md` therefore generates the
skill's main file, which tai then templates and transforms as usual. Symlinks
are rejected. A skill dir with no SKILL.md of its own installs fine when a
bundle names it, but `tai list assets` does not show it and `depends:` in the
generated frontmatter is not read; a stub SKILL.md in the source fixes the
listing.

`--dry-run` runs command includes as well, building the skill in a scratch
directory it then discards, so a script should have no effects beyond writing
to `$TAI_DEST_DIR`.

Env vars injected for the script (all always set, possibly to empty string,
so `set -u` is safe):

| Variable | Meaning |
|----------|---------|
| `TAI_DEST_DIR` | Absolute path where the script must write files |
| `TAI_ASSET_TYPE` | The asset type (`skill`) |
| `TAI_ASSET_NAME` | The asset's name |
| `TAI_TARGET` | The install target (`claude`, `codex`, ...) |
| `TAI_MANIFEST_DIR` | Absolute path to the source skill directory |
| `TAI_VERSION` | tai's version string |

Mutually exclusive with `from`+`path` and `url`. `timeout` is only valid with
`command`. `fields` (frontmatter remap) is rejected on `command:` includes.

**Secrets:** keep credentials in env vars (e.g. `$GITLAB_TOKEN`), not in
argv - tai echoes the full argv into error messages on failure. The user's
env is inherited so env-var-based secrets work out of the box.

### Field Mapping

When including a SKILL.md from an external source that uses different field names, use `fields` to remap them to tai's vocabulary. The mapping format is `tai_field: source_field`.

- Only applies to the include that targets SKILL.md at the skill root
- If the source file already has the tai field name set, the explicit value wins
- Unknown source fields referenced by the mapping are silently ignored

### Defining Sources

Sources are defined in the config repo's `tai.yaml`, allowing machine-specific paths:

```yaml
# my-tai-config/tai.yaml
sources:
  - name: rad
    path: ~/src/rad          # Different per machine
```

This keeps skills portable—the `tai.yaml` in the skill references source names, not absolute paths.

---

# Patching Imported Assets

Use a patch to change content from a source you don't control. Copying the asset
into your own source works but silently stops tracking upstream, so prefer a
patch whenever the change is small.

## Declaring

Patches and exclusions go on the source in `tai.yaml`:

```yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    patches: patches/shared-skills/   # dir of patch files in the config repo
    exclude:                          # files not carried at all
      - "skills/legacy-*"
```

Use `exclude:` when the goal is "don't take this file", and a patch when the
goal is "change this file, and tell me when upstream touches it". Exclusions
have no coupling to upstream; patches deliberately do.

## Writing a Patch

**Prefer `tai patch workspace` + `tai patch regenerate` over hand-writing.**
Editing a file is easier and less error-prone than describing an edit:

```bash
tai patch workspace shared-skills   # editable copy, existing patches applied
# edit files there
tai patch regenerate <name>         # tai derives the find/replace pairs
tai patch bless <name>              # record the upstream you reviewed
```

To create a new patch, write the file with an empty-ish edit first, then edit
the workspace and regenerate.

Hand-written format:

```yaml
# patches/shared-skills/use-pnpm.yaml
version: 1
file: skills/rad/SKILL.md     # source-relative; the dir is bound to a source
description: Prefer pnpm
edits:
  - find: |-
      Always install with npm.
    replace: |-
      Always install with pnpm.
```

Rules:

- One patch file targets **one** file. Patch name = path within the patch dir,
  minus extension. Subdirectories are walked.
- Each `find` must match **exactly once**. Zero or 2+ matches fails the install.
- `replace: ""` deletes the found text.
- Edits apply in order, each seeing the previous edit's result.
- **Use `find: |-`, not `find: |`.** The `|` form appends a trailing newline
  that is usually not in the source text.

## Blessing

`tai patch bless <name>` writes a `.blessed` snapshot beside the patch. Commit
it.

A patch that still applies is not necessarily still correct - upstream may have
changed something the patch has an opinion about, somewhere the patch does not
touch. Blessing is what lets tai warn about that. Without a snapshot, only
outright apply failures are reported.

## Diagnosing a Failure

An install that aborts with "could not be applied" needs the patch fixed;
nothing was written.

| Failure | Cause | Fix |
|---|---|---|
| `find text not found` | Upstream changed the text | Repair in the workspace, then `tai patch regenerate <name>` |
| `ambiguous - matched N locations` | `find` is too short | Regenerate (context is widened automatically), or add surrounding lines |
| `Hint: ...use find: \|-` | YAML appended a trailing newline | Change `find: \|` to `find: \|-` |
| `Hint: ...CRLF vs LF` | Line-ending mismatch | Match the source file's line endings |
| `does not carry` | File renamed/removed upstream, or excluded | Update `file:`, or drop the patch |
| `excluded by source` | A bundle references an excluded asset | Remove the exclusion or the reference |

Repair workflow - the failed install already left a workspace at the path in the
error:

```bash
tai patch diff <name>          # what moved upstream
# re-make the change by hand in the workspace path from the error
tai patch regenerate <name>
tai install <profile>
tai patch bless <name>
```

## Behavior Notes

- Patches never modify the source. tai builds a patched copy in its cache and
  installs from that, so `tai refresh` output and `tai locate` stay pristine.
- Everything downstream sees patched content: templating, frontmatter (including
  `depends:`), statuslines, skill includes, and `include_asset`.
- `tai install --dry-run` builds the view, so it validates patches without
  touching the target.
- Rebuilds happen on every install; there is no staleness to flush.

---

## Style Guidelines

- Use **imperative language** ("Run X", "Create Y")
- Write for an agent, not a human reader
- Omit obvious information the agent already knows
- Test instructions before including them
- Structure for scanning (tables, bullet points, code blocks)

## Migration: Hooks, MCPs, and config-asset scope (2026-08-09)

`hook` and `mcp` are new config asset types, in `assets/hooks/` and
`assets/mcps/`. Config assets now honour a `scope` field on profiles and on
individual asset entries (defaulting to `user`).

{% verbatim %}Config assets are also templated now, like every other asset - a
`{{ var }}` in a statusline or hook is substituted rather than installed
literally. A config asset containing literal `{{` or `{%` that is not meant as
a template needs a `{% verbatim %}` block around it.{% endverbatim %}

---

## Migration: Per-file tai-schema-version Removed (2026-01-24)

`tai-schema-version` is no longer recognized as a special field. If present in existing files, tai drops it and warns that it no longer does anything. Schema version is tracked at the repo level (`version: 1` in `tai.yaml`).

The only hard requirement is `name` (for personas, delegates, and skills). Files have no frontmatter requirements—content is copied as-is.

---

## Migration: Structured Asset Entries (2026-01-23)

Asset references in bundles and profiles changed from plain strings to structured entries with explicit `type` and `path` fields. This enables external sources with arbitrary directory layouts.

### Before
```yaml
assets:
  - personas/senior-swe.md
  - work-shared:skills/rad
```

### After
```yaml
assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: skill
    path: work-shared:skills/rad
```

### Valid types
`persona`, `delegate`, `skill`, `file`

### Mechanical conversion
For each asset string in `assets:`:
1. Determine the type from the first path segment (`personas/` → `persona`, `delegates/` → `delegate`, `skills/` → `skill`, `files/` → `file`)
2. Replace the string with a mapping: `type: <singular>` and `path: <original string>`
3. For `file` type, add `dest: ""` (or appropriate destination)

---

## Migration: doc type replaced with file type (2026-01-25)

The `doc` type has been replaced with `file`. The key difference:
- `file` requires explicit `dest` field (where the file lands)
- Content gets no frontmatter processing (templating still applies)

### Before
```yaml
assets:
  - type: doc
    path: docs/CLAUDE.md
```

### After
```yaml
assets:
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""              # Required: empty = target root
```

If you use `type: doc`, tai will show an error with migration instructions.
