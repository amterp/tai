# Tai

**Tai** is a CLI tool for managing AI agent configurations across different targets. It organizes, composes, and
installs target-agnostic assets (personas, delegates, skills, docs) from multiple sources while keeping your
configuration version-controlled and portable.

## Why Tai?

AI coding assistants like Claude Code, Codex, Gemini CLI, and OpenCode use configuration files to customize their behavior - output
styles, custom agents, skills, and reference documentation. Managing these configurations across machines, sharing them between
personal and work contexts, or keeping them version-controlled quickly becomes unwieldy.

Tai solves this by:

- **Centralizing configuration** in a version-controlled "config repo" that you own
- **Composing assets** from multiple sources (personal, work, shared team configs)
- **Installing to any supported target** with a single command
- **Tracking what's installed** so you can cleanly switch profiles or uninstall

## Installation

```bash
go install github.com/amterp/tai/cmd/tai@latest
```

<!-- TODO: Write a more user-friendly quick start guide -->

## Core Concepts

| Term            | What it is                                   | Claude equivalent          |
|-----------------|----------------------------------------------|----------------------------|
| **Persona**     | Behavioral style/role                        | output-style               |
| **Delegate**    | Specialized helper agent                     | agents (sub-agents)        |
| **Skill**       | Procedural capability                        | skills                     |
| **Doc**         | Reference material                           | CLAUDE.md, supporting docs |
| **StatusLine**  | Status bar configuration                     | `statusLine` in settings   |
| **Hook**        | Command run when something happens           | hook entry in settings     |
| **MCP**         | Model Context Protocol server                | `mcpServers` entry         |
| **Asset**       | Any one of the above                         | -                          |
| **Bundle**      | Named collection of assets                   | -                          |
| **Profile**     | Bundle(s) bound to a target                  | -                          |
| **Source**      | Location providing assets                    | -                          |
| **Patch**       | Local delta on a source you don't own        | -                          |
| **Target**      | AI tool to install to                        | Claude Code, Codex, Gemini CLI, OpenCode |
| **Config Repo** | Repository containing your Tai configuration | -                          |

### How They Fit Together

```
Sources (provide assets)
    ↓
Assets (personas/, delegates/, skills/, docs/)
    ↓
Bundles (compose assets into named collections)
    ↓
Profiles (bind bundles to a target)
    ↓
Target (Claude Code, Codex, OpenCode)
```

## Config Repo Structure

```
my-tai-config/
├── tai.yaml              # Config manifest
├── assets/               # Your target-agnostic assets
│   ├── personas/         # Behavioral styles
│   ├── delegates/        # Specialized helper agents
│   ├── skills/           # Procedural capabilities (directories)
│   ├── docs/             # Reference documentation
│   ├── statuslines/      # Status bar configuration
│   ├── hooks/            # Commands run on target events
│   └── mcps/             # MCP servers
├── bundles/              # Bundle definitions (YAML)
├── patches/              # Local deltas on sources you don't own
└── profiles/             # Profile definitions (YAML)
```

## Configuration Files

### tai.yaml

The manifest for your config repo:

```yaml
version: 1

sources:
  # "local" source is implicit (points to this repo's root)
  # Add additional sources as needed:
  - name: work-shared
    path: ~/work/ai-common
  - name: personal
    path: ~/personal-tai-config
  # Command sources fetch assets by running a command - e.g. cloning a
  # private repo. tai re-fetches on install; 'tai refresh' fetches manually.
  - name: company
    type: command
    command:
      - sh
      - -c
      - git clone --depth=1 git@github.com:company/ai-standards.git "$TAI_DEST_DIR"
  # URL sources download a single file. Pair with 'patches:' to keep local
  # edits on a document you don't control.
  - name: upstream-guide
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    patches: patches/upstream-guide/

default_profile: personal
```

### Bundle Definition

Bundles are "shopping lists" of assets. Each asset entry requires an explicit `type` and `path`:

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: doc
    path: docs/CLAUDE.md

# Bundles can include other bundles
bundles:
  - base-setup
```

Reference assets from other sources with `source:path` syntax in the path field:

```yaml
assets:
  - type: persona
    path: work-shared:personas/company-style.md
```

### Profile Definition

Profiles bind bundles to a target:

```yaml
# profiles/work.yaml
name: work
description: Full work setup
target: claude

bundles:
  - core
  - work-extras

# Can also include individual assets
assets:
  - type: persona
    path: assets/personas/brainstormer.md

# Optional: run a command after a successful install (notifications,
# follow-up scripts, post-processing installed files). Failure doesn't undo
# the install, but tai exits nonzero.
post_install:
  - ./scripts/after-install.sh
```

`post_install` commands get `TAI_TARGET`, `TAI_TARGET_DIR`, `TAI_PROFILE`,
`TAI_MANIFEST_DIR`, and `TAI_VERSION` in their env. They may modify files tai
installed - afterwards tai re-baselines its checksums so their edits don't show
up as conflicts on the next install. See SPEC.md for the full contract.

Note these are tai's own install-time commands, and are unrelated to the `hook`
asset type, which installs commands the *target* runs during a session.

### Asset File Formats

Personas, delegates and skills carry tai frontmatter. Only `name` is required:

```markdown
---
name: Senior SWE
description: Experienced software engineer co-owner role.
---

# Senior SWE

You are a senior software engineer...
```

During installation tai strips its own frontmatter and the adapter renders the
target's. Delegates and skills also take `tools` and `model`. A `file` asset is
copied byte-for-byte, frontmatter and all.

A field only one target understands goes under `native`, keyed by target name,
and is merged verbatim into that target's copy:

```markdown
---
name: parallel-work
description: Coordinates parallel worktree sessions.
native:
  claude:
    disable-model-invocation: true
---
```

Any field tai does not recognize is dropped with a warning naming it. See
SPEC.md for the full list.

## CLI Reference

### Configuration

```bash
tai config set <path>     # Set active config repo
tai config show           # Show current config and paths
```

### Discovery

```bash
tai list sources          # List configured sources
tai list bundles          # List available bundles
tai list profiles         # List available profiles
tai list assets           # List all assets
tai list assets --source work-shared  # Filter by source
```

### Inspection

```bash
tai show profile <name>   # Show resolved assets for a profile
tai show bundle <name>    # Show assets in a bundle
tai locate <asset-path>   # Print full filesystem path to an asset
```

### Installation

```bash
tai install <profile>              # Install profile to target
tai install <profile> --dry-run    # Preview changes
tai install <profile> --diff       # Show diffs for conflicting files
tai install <profile> --force      # Overwrite conflicting files
tai install <profile> --skip-conflicts  # Leave conflicts as-is
tai install <profile> --var key=val     # Pass template variables

tai uninstall <target>             # Remove Tai-managed files
tai status [target]                # Show what's installed
tai refresh [source]               # Fetch remote sources without installing
```

### Patches

```bash
tai patch list                # List patches and whether they still apply
tai patch diff <name>         # Show what changed upstream since blessing
tai patch bless <name>        # Record current upstream content as reviewed
tai patch bless --all         # Bless every patch that applies cleanly
tai patch workspace <source>  # Editable copy for authoring or repairing
tai patch regenerate <name>   # Rewrite a patch from your workspace edits
```

### Running

```bash
tai run <target> [args...]  # Run target (e.g., tai run claude or tai run codex)
```

## Installation Behavior

### Additive, Non-Destructive

Tai only manages files it installs. It never wipes your target directory or touches files it doesn't own.

### Conflict Resolution

When a file already exists:

| Scenario                        | Default behavior                                                |
|---------------------------------|-----------------------------------------------------------------|
| Tai-managed file, unchanged     | Overwrite with new version                                      |
| Tai-managed file, user-modified | Error (use `--diff` to inspect, `--force` to overwrite)         |
| Unmanaged file exists           | Error (use `--diff` to inspect, `--force` or `--skip-conflicts`)|

### Re-installation

When you run `tai install` with an existing installation, Tai:

1. Compares the new profile against the current manifest
2. Adds new files
3. Updates changed files
4. Removes files no longer in the profile

## Templating

Assets support simple `{{variable}}` substitution:

```markdown
# Project: {{project_name}}

Welcome to the {{project_name}} codebase.
```

```bash
tai install work --var project_name=MyApp
```

A source written for a target rather than for tai may contain `{#` or `{{` that
was never meant as template syntax, such as a Markdown heading anchor. Mark it
`template: false` in `tai.yaml` to install its content as written:

```yaml
sources:
  - name: vendor-skills
    path: ~/src/vendor-skills
    template: false
```

### Target-Aware Files

Adapters provide target-specific variables (prefixed with `target_`), enabling files that adapt to each target:

```yaml
# bundles/core.yaml
assets:
  - type: file
    path: assets/files/AGENTS.md
    dest: "{{target_agents_file}}"  # CLAUDE.md for Claude, AGENTS.md for Codex
```

| Variable | Claude | Codex | OpenCode |
|----------|--------|-------|----------|
| `target_name` | `claude` | `codex` | `opencode` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `AGENTS.md` |

User variables (`--var`) override adapter variables on collision.

### The `{{existing}}` Variable

Inject the current contents of a target file -- useful for wrapping existing configuration:

```markdown
# Tai additions

{{existing}}

# End Tai additions
```

When using `{{existing}}`, Tai backs up the original file and restores it on uninstall.

## Patching Imported Assets

When you import from a source you don't own -- a shared team repo, say -- you'll
sometimes need a small local change. Editing the checkout drifts silently and
dies on the next pull; forking loses upstream updates. Patches let you keep
tracking upstream and declare a delta on top.

Declare it on the source:

```yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    patches: patches/shared-skills/   # dir of patch files in your config repo
    exclude:                          # files you don't carry at all
      - "skills/legacy-*"
```

Then write a patch file:

```yaml
# patches/shared-skills/use-pnpm.yaml
version: 1
file: skills/rad/SKILL.md
description: Prefer pnpm over npm
edits:
  - find: |-
      Always install with npm.
    replace: |-
      Always install with pnpm.
```

The next `tai install` applies it. Your checkout is never modified -- Tai builds
a patched copy in its cache and installs from that.

`exclude:` is separate and works on its own. A patch means "change this, and
tell me when upstream touches it"; an exclusion means "I don't carry this file
at all, upstream can do what it likes".

### Patches Fail Loudly

Every `find` must match **exactly once**. If upstream changes so the text no
longer matches -- or now matches twice -- the install stops rather than quietly
installing content you'd rejected:

```
patch 'use-pnpm' could not be applied - install aborted, nothing was changed:

  patch:  patches/shared-skills/use-pnpm.yaml
  file:   shared-skills:skills/rad/SKILL.md
  edit #1: find text not found - upstream has likely changed since this patch was written
    | Always install with npm.

  To fix:
    1. See what changed upstream:             tai patch diff use-pnpm
    2. Edit the file to match what you want:  ~/.cache/tai/workspaces/.../shared-skills
    3. Turn that edit into the patch:         tai patch regenerate use-pnpm
    4. Review the rewritten edits:            patches/shared-skills/use-pnpm.yaml
    5. Accept the new upstream:               tai patch bless use-pnpm
    6. Re-run the install
```

> **Tip:** YAML's `find: |` appends a trailing newline; use `find: |-` if you
> don't want one. Tai detects this (and CRLF/LF mismatches) and says so.

### Don't Write Patches By Hand

Editing a file is easier than describing an edit, so let Tai do the describing:

```bash
tai patch workspace shared-skills     # editable copy, patches already applied
# ...edit files normally...
tai patch regenerate use-pnpm         # Tai works out the find/replace pairs
```

This is also the repair path -- a failed install leaves its workspace at the
same location, ready to fix.

### Blessing

A patch that still applies isn't necessarily still *right*. Upstream might
change something your patch has an opinion about, somewhere your patch doesn't
touch.

`tai patch bless <name>` records the upstream content you reviewed, as a
`.blessed` file next to the patch. Commit it. Afterwards, Tai warns when that
file changes even though the patch still applies:

```
Warning: upstream file for patch 'use-pnpm' changed since it was blessed
  file: shared-skills:skills/rad/SKILL.md
  The patch still applies cleanly; installing the patched content.
  Review what changed:  tai patch diff shared-skills:use-pnpm
  Accept it:            tai patch bless shared-skills:use-pnpm
```

Run `tai patch list` to see where every patch stands.

## Example Workflows

### Fresh Machine Setup

```bash
git clone github.com/me/my-tai-config ~/my-tai-config
tai config set ~/my-tai-config
tai install personal
claude  # Ready to use
```

### Editing an Asset

```bash
tai locate assets/personas/senior-swe.md
# → /Users/me/my-tai-config/assets/personas/senior-swe.md

vim "$(tai locate assets/personas/senior-swe.md)"
tai install personal  # Re-install to apply changes
```

### Switching Profiles

```bash
tai status          # Profile: personal
tai install work    # Switches to work profile
tai status          # Profile: work
```

### Multiple Sources (Work Setup)

```yaml
# work-tai-config/tai.yaml
sources:
  - name: local
    path: .
  - name: personal
    path: ~/personal-tai-config
  - name: company
    path: ~/work/company-ai-config
```

A source's `path` is a root, and references are literal paths from it - so a config repo's assets read as `personal:assets/personas/senior-swe.md`. Assets from later sources override earlier ones when paths conflict. Tai logs these overrides as info messages.

### Importing Bundles From Another Repo

A source also offers whatever bundles live in its `bundles/` directory, so a shared bundle can be referenced instead of copied:

```yaml
# work-tai-config/profiles/work.yaml
bundles:
  - work-local
  - personal:core                 # read from ~/personal-tai-config at install time
  - name: company:standards
    exclude:
      - assets/skills/beagle      # this machine cannot use it
```

An import stays current: assets added to `personal:core` upstream arrive on your next install with nothing to update on your side. Inside an imported bundle, unprefixed paths resolve against that bundle's own repo, so upstream's references keep meaning upstream's files.

## Supported Targets

### Claude Code

- Target directory: `~/.claude/`
- Asset mapping:
    - `personas/` → `output-styles/`
    - `delegates/` → `agents/`
    - `skills/` → `skills/`
    - `docs/` → root level

### Codex

- Target directory: `$CODEX_HOME` (if set), otherwise `~/.codex/`
- Asset mapping:
    - `personas/` → `skills/` (mapped to skills since Codex has no persona concept; warns per asset)
    - `delegates/` → `delegates/` (passive reference files)
    - `skills/` → `skills/`
    - `files/` → location specified by `dest` field
- Behavior note:
    - Codex has first-class support for `AGENTS.md` and skills.
    - Personas are installed as skills (`skills/<name>/SKILL.md`) so Codex actually consumes them; tai warns for each one.
    - Delegates are installed as fallback reference files and warned about.

### Gemini CLI

- Target directory: `$GEMINI_HOME` (if set), otherwise `~/.gemini/`
- Asset mapping:
    - `personas/` -> `skills/` (mapped to skills; Gemini has no persona concept)
    - `delegates/` -> unsupported (skipped with warning)
    - `skills/` -> `skills/`
    - `files/` -> location specified by `dest` field
- Behavior note:
    - Gemini CLI has no native persona or delegate concepts.
    - Personas are installed as skills so their prompt content is still accessible.
    - Delegates are skipped entirely since Gemini has no user-defined sub-agent system.

### OpenCode

- Target directory: `$XDG_CONFIG_HOME/opencode` (if set), otherwise `~/.config/opencode/`
- Asset mapping:
    - `personas/` -> `agents/` (description only; filename is the name)
    - `delegates/` -> `agents/` (description + `mode: subagent`)
    - `skills/` -> `skills/`
    - `files/` -> location specified by `dest` field
- Behavior note:
    - Both personas and delegates install to `agents/` - OpenCode uses a single agent concept.
    - Delegates get a `mode: subagent` field to distinguish them from primary agents.
    - Tools are omitted (incompatible format between tai and OpenCode).

## Global State

Tai stores its configuration in standard locations:

```
~/.config/tai/
├── config.yaml         # Active config repo path
└── manifests/          # Installation manifests by target
    └── claude.yaml
```

## Skills

Skills follow the [agentskills.io](https://agentskills.io) standard. Unlike other assets (which are single files),
skills are directories containing a `SKILL.md` with YAML frontmatter:

```
assets/skills/my-skill/
├── SKILL.md            # Required: YAML frontmatter + description
├── tai.yaml            # Optional: declare external file dependencies
└── ...                 # Additional files as needed
```

Reference skills in bundles by directory name:

```yaml
assets:
  - type: skill
    path: assets/skills/my-skill
```

### Skill Includes

Skills can pull in external files at install time via a `tai.yaml` manifest:

```yaml
# assets/skills/my-skill/tai.yaml
include:
  - from: my-source      # Named source from config's tai.yaml
    path: docs/guide.md
    dest: references/    # Trailing "/" preserves filename

  - url: https://example.com/file.md
    dest: docs/remote.md

  # Escape hatch: run any command to fetch files. Use for authenticated
  # sources (self-hosted GitLab, private GitHub, etc.) - the script handles auth.
  # The script must write into $TAI_DEST_DIR; tai stages output there and
  # atomically swaps it into `dest` on success.
  - command:
      - sh
      - -c
      - "glab api 'projects/123/repository/files/foo%2Fbar/raw?ref=main' > $TAI_DEST_DIR/foo.md"
    dest: refs/          # final location inside the installed skill

  # A dest of ./ merges the output into the skill root instead of replacing
  # it, so a command can generate the skill's own SKILL.md.
  - command: ["sh", "-c", "mytool skill > \"$TAI_DEST_DIR/SKILL.md\""]
    dest: ./
```

> Note: tai runs `command` directly, with no shell - wrap in `["sh", "-c", "..."]`
> if you need shell features like redirection or env-var expansion. Keep
> credentials in environment variables (e.g. `$GITLAB_TOKEN`), not in argv -
> argv is echoed verbatim in error messages.

This keeps skills portable -- reference source names instead of machine-specific paths. See [SPEC.md](SPEC.md) for full details.

## Design Principles

1. **Target-agnostic assets**: Define assets in Tai terminology; adapters translate for each target
2. **Additive installation**: Never wipes target directories; only manages what Tai installed
3. **Fail-fast**: Missing files or circular references cause immediate failure, not partial installs
4. **Version-controlled source of truth**: Your config repo is canonical; target directories are derived

## Contributing

Contributions welcome! See [AGENTS.md](AGENTS.md) for architecture details and development guidance.

## License

MIT
