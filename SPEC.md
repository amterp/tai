# tai Specification

**Version:** 0.1 (MVP)
**Status:** Implemented

## Overview

tai is a CLI tool for managing AI agent configurations. It provides a framework for organizing, composing, and installing assets (personas, delegates, skills, reference docs) across different AI targets (Claude Code, Cursor, etc.) while keeping configuration version-controlled and portable.

## Core Concepts

### Terminology

| tai term | What it is | Claude Code equivalent |
|----------|------------|------------------------|
| **Target** | The AI tool we install to | Claude Code, Cursor, Windsurf, etc. |
| **Persona** | A behavioral style/role | output-style |
| **Delegate** | A specialized helper for tasks | agents (sub-agents) |
| **Skill** | A procedural capability | skills |
| **Doc** | Reference material | CLAUDE.md, supporting docs |
| **MCP** | Model Context Protocol server | MCP server entry in config |
| **Hook** | Event-triggered shell command | Hook entry in settings |

Additional terms:
- **Asset**: An atomic configuration unit—a single persona, delegate, skill, doc, MCP, or hook.
- **Bundle**: A named collection of assets. Acts as a "shopping list" for composing related assets.
- **Profile**: A bundle bound to a specific target. Profiles are what you actually install and run.
- **Source**: A location providing assets—a local directory, another tai config repo, or (future) a git remote.
- **Config Repo**: A repository defining sources, bundles, and profiles. The entry point for tai.
- **Adapter**: Target-specific logic that knows how to build, install, and run a particular AI target.

### Asset Types

Assets come in two shapes based on how they're installed:

| Shape | Asset Types | How Defined | How Installed |
|-------|-------------|-------------|---------------|
| **File assets** | personas, delegates, skills, docs | Files/directories in `assets/` | Copied to target paths |
| **Config assets** | mcps, hooks | YAML fragments in `assets/` | Merged into target JSON config files |

**File assets** are the traditional model: you define a file (or directory for skills), and tai copies it to the appropriate location in the target's config directory.

**Config assets** are entries that get merged into JSON configuration files. The target (Claude Code, Cursor, etc.) reads these JSON files at runtime. tai must read the existing JSON, merge in the new entries, and write it back—being careful to preserve entries it doesn't manage.

This distinction matters because:
1. **Installation logic differs**: copy vs. JSON merge
2. **Uninstallation differs**: delete file vs. remove JSON keys
3. **Conflict handling differs**: file exists vs. key exists
4. **Scope matters more**: config assets often have project/user/local scope options

### Design Principles

1. **Target-agnostic assets**: Assets are defined in tai's terminology (personas, delegates, etc.), not target-specific terms. Adapters handle translation during the build/install step.

2. **Additive installation**: tai never wipes target config directories. It only manages files it installed, leaving other files untouched.

3. **Version-controlled source of truth**: Your config repo is the canonical source. Target directories (like `~/.claude/`) are derived outputs that can be fully reconstructed.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        tai CLI                               │
│  (framework, adapters, core logic)                          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ references
┌─────────────────────────────────────────────────────────────┐
│                    Active Config Repo                        │
│  (tai.yaml, profiles/, bundles/, assets/)                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ declares
┌─────────────────────────────────────────────────────────────┐
│                        Sources                               │
│  - Local directories                                         │
│  - Other tai config repos (inherits assets/)                │
│  - Git remotes (future)                                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ provides
┌─────────────────────────────────────────────────────────────┐
│                         Assets                               │
│  File: personas/, delegates/, skills/, docs/                │
│  Config: mcps/, hooks/                                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ composed into
┌─────────────────────────────────────────────────────────────┐
│                        Bundles                               │
│  Named collections referencing assets from sources          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ combined into
┌─────────────────────────────────────────────────────────────┐
│                        Profiles                              │
│  Bundle(s) + target                                         │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ installed to (via adapter)
┌─────────────────────────────────────────────────────────────┐
│                    Target Config                             │
│  ~/.claude/, ~/.cursor/, etc.                               │
└─────────────────────────────────────────────────────────────┘
```

## Repository Structures

### tai CLI Repository (Public Framework)

```
tai/
├── cmd/
│   └── tai/
│       └── main.go
├── internal/
│   ├── config/           # Config repo loading/parsing
│   ├── adapter/          # Adapter interface and registry
│   │   ├── adapter.go    # Interface definition
│   │   ├── claude/       # Claude Code adapter
│   │   └── cursor/       # Future: Cursor adapter
│   ├── bundle/           # Bundle resolution
│   ├── profile/          # Profile management
│   └── install/          # Installation logic
├── go.mod
├── go.sum
└── README.md
```

### tai Config Repository (User's Personal/Work Config)

Assets are **target-agnostic**. The adapter handles translation to target-specific formats.

```
my-tai-config/
├── tai.yaml              # Config repo manifest
├── assets/               # Target-agnostic assets
│   ├── personas/         # Behavioral styles (→ output-styles in Claude)
│   │   ├── senior-swe.md
│   │   └── brainstormer.md
│   ├── delegates/        # Specialized helpers (→ agents in Claude)
│   │   ├── code-reviewer.md
│   │   └── pair-programmer.md
│   ├── skills/           # Procedural capabilities (→ skills in Claude)
│   │   └── council-participant/   # Skills follow agentskills.io standard
│   ├── docs/             # Reference material (→ CLAUDE.md etc.)
│   │   ├── CLAUDE.md
│   │   └── RFC2119.md
│   ├── mcps/             # MCP server definitions (→ .mcp.json entries)
│   │   ├── github.yaml
│   │   └── postgres.yaml
│   └── hooks/            # Hook definitions (→ settings.json entries)
│       └── format-on-save.yaml
├── bundles/
│   ├── core.yaml
│   └── work-extras.yaml
└── profiles/
    ├── personal.yaml
    └── work.yaml
```

## Configuration Files

### tai.yaml (Config Repo Manifest)

```yaml
# my-tai-config/tai.yaml
version: 1

# Sources provide assets that bundles can reference.
# PRIORITY: Later sources override earlier ones when asset paths conflict.
# This ordering is intentional—list base/default sources first, overrides last.
sources:
  # Local assets in this repo
  # NOTE: The "local" source is IMPLICIT - tai automatically adds it if not
  # explicitly defined. It points to ./assets in the config repo. You only
  # need to declare it if you want to override the path.
  - name: local
    path: ./assets

  # Another local directory
  - name: work-shared
    path: ~/work/ai-common

  # Another tai config repo (uses its assets/ directory)
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config

  # Future: Git remote
  # - name: company
  #   git: github.com/company/ai-standards
  #   ref: main

# Default profile to use when none specified
default_profile: personal
```

### Bundle Definition

Bundles are "shopping lists" of assets. They can reference individual assets or include other bundles for composition.

```yaml
# my-tai-config/bundles/core.yaml
name: core
description: Essential assets for daily work

# Assets can be referenced with source:path syntax
# If source omitted, defaults to "local"
# Both file assets and config assets use the same referencing syntax
assets:
  - personas/senior-swe.md
  - delegates/code-reviewer.md
  - docs/CLAUDE.md
  - docs/RFC2119.md
  - mcps/github              # Config asset: MCP server
  - hooks/format-on-save     # Config asset: hook
```

```yaml
# my-tai-config/bundles/work-extras.yaml
name: work-extras
description: Additional assets from work sources

assets:
  - work-shared:personas/company-style.md
  - work-shared:delegates/security-reviewer.md
```

```yaml
# my-tai-config/bundles/full-setup.yaml
name: full-setup
description: Comprehensive setup combining multiple bundles

# Bundles can include other bundles for composition
bundles:
  - core
  - work-extras

# Plus additional individual assets
assets:
  - personas/brainstormer.md
```

### Profile Definition

Profiles bind bundles to a specific target. Bundle order matters: **later bundles override earlier ones** when asset paths conflict. tai logs an info message when such overrides occur.

```yaml
# my-tai-config/profiles/work.yaml
name: work
description: Full work setup
target: claude

# Bundles to include (resolved in order, later overrides earlier on conflict)
# Example: if both 'core' and 'work-extras' have personas/senior-swe.md,
#          work-extras version wins and tai logs the override
bundles:
  - core
  - work-extras

# Can also include individual assets directly
assets:
  - personas/brainstormer.md

# Optional: adapter-specific settings (passed to adapter during install/run)
options:
  # Example Claude-specific options (future use)
  # dangerously_skip_permissions: false

# Optional: installation scope for config assets (default: project)
# Affects where MCPs and hooks are installed
scope: project  # or "user" or "local"
```

### Config Asset Definitions

Config assets (MCPs and hooks) are defined as YAML files that describe entries to be merged into target configuration files.

#### MCP Definition

```yaml
# my-tai-config/assets/mcps/github.yaml
# The filename (minus extension) becomes the server name

type: stdio                    # Required: "stdio" or "http"
command: npx                   # Required for stdio: executable path
args:                          # Optional: command arguments
  - "-y"
  - "@modelcontextprotocol/server-github"
env:                           # Optional: environment variables
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

```yaml
# my-tai-config/assets/mcps/internal-api.yaml
# HTTP-based MCP server

type: http
url: https://api.internal.company.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
```

**MCP fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | No | Override filename as server name |
| `type` | string | Yes | `stdio` or `http` |
| `command` | string | stdio only | Executable path |
| `args` | list | No | Command arguments |
| `env` | map | No | Environment variables (supports `${VAR}` expansion) |
| `url` | string | http only | Server endpoint |
| `headers` | map | http only | HTTP headers |

#### Hook Definition

```yaml
# my-tai-config/assets/hooks/format-on-save.yaml

event: PostToolUse             # Required: which event triggers this hook
matcher: Edit|Write            # Optional: tool pattern (empty = all tools)
command: prettier --write "$FILE"  # Required: shell command to run
```

```yaml
# my-tai-config/assets/hooks/security-check.yaml

event: PreToolUse
matcher: Bash
command: ~/scripts/check-dangerous-commands.sh
```

**Hook fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | No | Override filename as hook identifier |
| `event` | string | Yes | Event that triggers the hook |
| `matcher` | string | No | Tool pattern (`Tool1\|Tool2`), empty matches all |
| `command` | string | Yes | Shell command to execute |

**Available hook events (Claude Code):**

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool executes |
| `PostToolUse` | After tool executes |
| `UserPromptSubmit` | When user submits a prompt |
| `Notification` | When Claude sends a notification |
| `Stop` | When Claude finishes responding |
| `SubagentStop` | When a subagent completes |
| `SessionStart` | When a session begins |
| `SessionEnd` | When a session ends |

## CLI Interface

### Global State

tai stores its global configuration in `~/.config/tai/` and caches remote sources in `~/.cache/tai/`:

```
~/.config/tai/
├── config.yaml           # CLI settings
└── manifests/            # Installation manifests by target
    └── claude.yaml

~/.cache/tai/
└── sources/              # Cached remote sources (git repos)
    └── github.com/
        └── company/
            └── ai-standards/
                └── <ref>/
```

```yaml
# ~/.config/tai/config.yaml
active_config: /Users/amterp/work-tai-config
```

### Commands

```bash
# Configuration
tai config set <path>           # Set active config repo
tai config show                 # Show current config repo and settings

# Discovery
tai list sources                # List sources in active config
tai list bundles                # List available bundles
tai list profiles               # List available profiles
tai list assets [--source X]    # List assets, optionally filtered by source

# Inspection
tai show profile <name>         # Show resolved assets for a profile
tai show bundle <name>          # Show assets in a bundle
tai locate <asset-path>         # Print full filesystem path to an asset

# Installation
tai install <profile>           # Install profile to target config
tai install <profile> --dry-run # Preview what would be installed (no changes)
tai uninstall <target>          # Remove tai-managed files for a target
tai status [target]             # Show what's currently installed

# Running (convenience wrapper)
tai run claude [args...]        # Run claude with tai config applied
tai run cursor [args...]        # Run cursor with tai config applied

# Remote Source Management (Future)
tai sync                        # Re-fetch all remote sources
tai sync <source-name>          # Re-fetch specific remote source

# Build (Future)
tai build <profile>             # Build/compile assets without installing
```

### Example Session

```bash
# Initial setup
tai config set ~/my-tai-config
tai list profiles
# Output:
#   personal  claude  Core assets for personal use
#   work      claude  Full work setup

# Preview installation
tai install work --dry-run
# Output:
#   Would install profile 'work' for claude...
#   + personas/senior-swe.md (new)
#   + personas/company-style.md (new)
#   + delegates/code-reviewer.md (new)
#   ...
#   8 files would be added, 0 updated, 0 removed

# Install a profile
tai install work
# Output:
#   Installing profile 'work' for claude...
#   Resolving bundles: core, work-extras
#   ℹ Override: personas/senior-swe.md (core → work-extras)
#   Installing 8 assets to ~/.claude/
#   ✓ output-styles/senior-swe.md
#   ✓ output-styles/company-style.md
#   ✓ agents/code-reviewer.md
#   ...
#   Manifest written to ~/.config/tai/manifests/claude.yaml

# Check status
tai status claude
# Output:
#   Profile: work
#   Installed: 2024-01-21T14:30:00Z
#   Assets: 8 files

# Find where an asset lives
tai locate personas/senior-swe.md
# Output:
#   /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Run claude with tai config
tai run claude
# Equivalent to: claude (tai config already installed to ~/.claude/)
```

## Adapter Interface

Each supported target has an adapter implementing:

```go
type Adapter interface {
    // Name returns the adapter name (e.g., "claude", "cursor")
    Name() string

    // AssetMapping returns how tai asset types map to target paths
    // e.g., "personas" → "output-styles", "delegates" → "agents"
    AssetMapping() map[string]string

    // SupportedAssetTypes returns which asset types this adapter supports
    // Profiles containing unsupported types will fail fast at install time
    SupportedAssetTypes() []string

    // ValidateAssets checks that assets are valid for this target
    ValidateAssets(assets []Asset) error

    // TargetDir returns where this target's config lives
    TargetDir() string  // e.g., ~/.claude

    // Install copies/links assets to the target's config directory
    // Returns a manifest of what was installed
    Install(assets []ResolvedAsset, opts InstallOptions) (*Manifest, error)

    // Uninstall removes tai-managed files using the manifest
    Uninstall(manifest *Manifest) error

    // Run invokes the target (optional, for wrapper mode)
    Run(args []string) error

    // --- Config asset methods (for mcps, hooks, etc.) ---

    // ConfigAssetTypes returns which asset types are config-based (vs file-based)
    ConfigAssetTypes() []string  // e.g., ["mcps", "hooks"]

    // ConfigFilePath returns the target JSON file for a config asset type and scope
    ConfigFilePath(assetType string, scope string) string

    // MergeConfigAsset merges a config asset into existing JSON content
    MergeConfigAsset(existing []byte, asset ConfigAsset) ([]byte, error)

    // RemoveConfigAsset removes a config asset from existing JSON content
    RemoveConfigAsset(existing []byte, assetID ConfigAssetID) ([]byte, error)
}
```

### Claude Adapter Specifics

The Claude adapter:
- Targets `~/.claude/`
- Maps tai terminology to Claude terminology:
  - `personas/` → `output-styles/`
  - `delegates/` → `agents/`
  - `skills/` → `skills/`
  - `docs/` → root level (CLAUDE.md, etc.)
- Installation is file copy (not symlink for reliability)
- May use `--add-dir` for run-time config in future

**Config asset handling:**

| Asset Type | Scope | Target File |
|------------|-------|-------------|
| `mcps` | project | `.mcp.json` (project root) |
| `mcps` | user | `~/.claude.json` |
| `mcps` | local | `.claude.json` (project root) |
| `hooks` | project | `.claude/settings.json` |
| `hooks` | user | `~/.claude/settings.json` |
| `hooks` | local | `.claude/settings.local.json` |

**MCP installation:** Entries are added to the `mcpServers` key in the target JSON file:
```json
{
  "mcpServers": {
    "github": { "type": "stdio", "command": "npx", ... }
  }
}
```

**Hook installation:** Entries are added to the `hooks.{EventName}` array in the target JSON file:
```json
{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Edit|Write", "hooks": [{ "type": "command", "command": "..." }] }
    ]
  }
}
```

## Installation Manifest

```yaml
# ~/.config/tai/manifests/claude.yaml
target: claude
profile: work
config_repo: /Users/amterp/work-tai-config
installed_at: 2024-01-21T14:30:00Z

# File assets: tracked by path
files:
  - path: output-styles/senior-swe.md
    source: local
    source_path: /Users/amterp/my-tai-config/assets/personas/senior-swe.md
    checksum: sha256:abc123...

  - path: output-styles/company-style.md
    source: work-shared
    source_path: /Users/amterp/work/ai-common/personas/company-style.md
    checksum: sha256:def456...

  # Files using {{existing}} templating track their backup
  - path: CLAUDE.md
    source: local
    source_path: /Users/amterp/my-tai-config/assets/docs/CLAUDE.md
    checksum: sha256:789ghi...
    backup_path: /Users/amterp/.claude/CLAUDE.md.tai-backup
    is_templated: true

# Config assets: tracked by type, name, and target file
config_entries:
  - type: mcp
    name: github
    source: local
    source_path: /Users/amterp/my-tai-config/assets/mcps/github.yaml
    target_file: .mcp.json
    scope: project
    checksum: sha256:mcp123...   # Checksum of the installed JSON fragment

  - type: hook
    name: format-on-save
    source: local
    source_path: /Users/amterp/my-tai-config/assets/hooks/format-on-save.yaml
    target_file: .claude/settings.json
    scope: project
    event: PostToolUse           # For hooks: which event array it's in
    checksum: sha256:hook456...
```

The manifest enables:
- `tai uninstall`: Remove exactly what tai installed
- `tai install`: Diff against previous install, add new, remove stale
- `tai status`: Show what's installed and from where
- Conflict detection: Warn if file was modified externally

## Installation Behavior

### Additive, Non-Destructive

tai installation is **additive**:
- Only touches files it manages (tracked in manifest)
- Preserves files not managed by tai
- Never wipes the entire target directory

### Re-installation

When running `tai install <profile>` with an existing installation:
1. Load current manifest
2. Resolve new profile's assets
3. Compare:
   - Files to add (in new, not in old)
   - Files to remove (in old, not in new)
   - Files to update (in both, checksum changed)
4. Apply changes
5. Update manifest

### Conflict Handling

| Scenario | Manifest Entry? | File Matches Checksum? | Action |
|----------|-----------------|------------------------|--------|
| New file | No | N/A | Install safely |
| Managed update | Yes | Yes | Overwrite with new version |
| User divergence | Yes | No | **Warning**: Require `--force` |
| Unmanaged conflict | No | N/A | **Error**: File exists but tai doesn't own it |

Options:
- `--force`: Overwrite conflicting files
- `--skip-conflicts`: Leave conflicting files as-is
- `--dry-run`: Preview changes without applying

### Error Handling

tai follows a **fail-fast** philosophy. Installation MUST fail (not warn-and-continue) when:

- A source path doesn't exist (local directory missing)
- A git source fails to clone/fetch
- An asset referenced in a bundle doesn't exist in its source
- A bundle referenced in a profile doesn't exist

Rationale: Silent partial installs are worse than loud failures. A typo in a bundle definition should not result in a working-but-incomplete installation.

### Config Asset Installation

Config assets (MCPs, hooks) require different handling than file assets because they're merged into shared JSON configuration files.

#### Installation Process

1. **Read target file**: Load the existing JSON config (or `{}` if it doesn't exist)
2. **Merge entries**: Add/update the config asset entry at the appropriate key path
3. **Write back**: Save the modified JSON, preserving all non-tai-managed entries
4. **Record in manifest**: Track the asset name, target file, and checksum

#### Uninstallation Process

1. **Load manifest**: Find config entries tai installed
2. **Read target file**: Load the current JSON config
3. **Identify entries**: Match by type and name (e.g., MCP server name, hook identifier)
4. **Compare checksums**: If entry was modified since install, warn but proceed (user took ownership)
5. **Remove entries**: Delete only the entries tai installed
6. **Write back**: Save the modified JSON (or delete file if empty and tai-created)

#### Conflict Handling for Config Assets

| Scenario | Action |
|----------|--------|
| Key doesn't exist | Add normally |
| Key exists, not in manifest | **Error**: Unmanaged conflict (use `--force` to overwrite) |
| Key exists, in manifest, unchanged | Update to new version |
| Key exists, in manifest, user modified | **Warning**: Overwrite with `--force`, skip with `--skip-conflicts` |

#### Scope Resolution

Config assets can be installed at different scopes. The scope is determined by (in order of precedence):

1. Per-asset override in profile (if supported)
2. Profile-level `scope` setting
3. Default scope (typically `project`)

```yaml
# Profile with explicit scope
name: work
target: claude
scope: user  # MCPs and hooks install to user-level config files

bundles:
  - dev-tools
```

### Remote Source Caching (Future)

When using git remote sources:
- Sources are cached in `~/.cache/tai/sources/`
- `tai sync` fetches/updates cached sources
- `tai install` does NOT require explicit sync first
- `tai install` warns if cache is stale (configurable threshold, default 24h)
- Cache miss during install triggers automatic fetch

## Templating

tai supports simple variable substitution in assets during installation.

### Syntax

Variables use double curly braces: `{{variable_name}}`

```markdown
# Project: {{project_name}}

Welcome to the {{project_name}} codebase.
```

When installing, pass variables via the `--var` flag:

```bash
tai install work --var project_name=MyApp
```

### Special Variables

#### `{{existing}}`

The `{{existing}}` variable injects the current contents of the target file. This is useful for wrapping existing configuration without losing user content.

```markdown
# tai Header

{{existing}}

# tai Footer
```

When installed to a file that already contains content, the result includes the original content in place of `{{existing}}`.

**Backup behavior:** When using `{{existing}}`, tai:
1. Creates a backup of the original file (e.g., `CLAUDE.md.tai-backup`)
2. Records the backup path in the manifest
3. On uninstall, restores the original content from the backup

This ensures users don't lose their customizations when uninstalling tai.

### Merge Strategies (Future)

For special files like docs that may need merging rather than replacing:
- **Overwrite**: (Default) Replace entire file
- **Append/Prepend**: Add content to existing file
- **Template**: Inject content at marked locations

## Example Workflows

### Fresh Machine Setup

```bash
# Clone your config repo
git clone github.com/me/my-tai-config ~/my-tai-config

# Install tai CLI
go install github.com/amterp/tai/cmd/tai@latest

# Configure and install
tai config set ~/my-tai-config
tai install personal

# Ready to use
claude
```

### Editing an Asset

```bash
# Find the asset
tai locate personas/senior-swe.md
# /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Edit it
vim /Users/amterp/my-tai-config/assets/personas/senior-swe.md

# Re-install to apply changes
tai install personal

# Test
claude
```

### Adding a New Asset

```bash
# Create new persona
vim ~/my-tai-config/assets/personas/new-style.md

# Add to a bundle
vim ~/my-tai-config/bundles/core.yaml
# Add: - personas/new-style.md

# Re-install
tai install personal
```

### Switching Profiles

```bash
tai status
# Profile: personal

tai install work
# Removes personal-only assets, adds work assets

tai status
# Profile: work
```

### Work Machine Setup (Multiple Sources)

```yaml
# work-tai-config/tai.yaml
sources:
  - name: local
    path: ./assets
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config
  - name: company
    path: ~/work/company-ai-config
```

```yaml
# work-tai-config/profiles/full-work.yaml
name: full-work
target: claude
bundles:
  - personal:core        # From personal tai repo
  - company-standards    # From company source
  - local-overrides      # Local customizations
```

## MVP Scope

### Included in v0.1

- [x] Config repo structure (tai.yaml, bundles/, profiles/, assets/)
- [x] Target-agnostic asset structure (personas/, delegates/, skills/, docs/)
- [x] Source types: local path, other tai repo
- [x] Bundle and profile resolution with circular reference detection (fail-fast)
- [x] Diamond dependency deduplication
- [x] Claude adapter with file-copy installation and terminology mapping
- [x] Skills handling per agentskills.io standard (SKILL.md with YAML frontmatter)
- [x] Installation manifest tracking with backup support
- [x] Basic CLI: config, list, install, uninstall, status, locate
- [x] `--dry-run` flag for install preview
- [x] `--force` and `--skip-conflicts` flags for conflict handling
- [x] `tai run` pass-through
- [x] Simple templating with `{{variable}}` substitution
- [x] `{{existing}}` templating for wrapping existing file content with backup/restore

### Deferred to Future Versions

- [ ] **Config assets (MCPs, hooks)** - v0.2 target
  - [ ] MCP server definitions (`assets/mcps/`)
  - [ ] Hook definitions (`assets/hooks/`)
  - [ ] JSON merge installation logic
  - [ ] Scope handling (project/user/local)
  - [ ] Config entry manifest tracking
- [ ] Git remote sources
- [ ] `--add-dir` runtime injection (vs. installing to ~/.claude)
- [ ] Cursor adapter
- [ ] Merge strategies (append/prepend/template) - beyond {{existing}}
- [ ] Asset validation (schema checking)
- [ ] Version compatibility checking
- [ ] `tai lint` command
- [ ] `tai install --strict` mode
- [ ] Profile inheritance

## Future Considerations

Items considered during design but explicitly deferred. These inform future development but are NOT in MVP scope.

### `tai lint` Command
**What:** Static analysis to catch common issues before install.
**Why deferred:** Adds complexity. Documentation can cover common dependency patterns for now.
**Future scope:**
- Warn about missing common asset dependencies (skill needs doc)
- Detect unused assets in bundles
- Catch circular bundle references

### Asset Dependencies (DAG Resolution)
**What:** Assets declare required dependencies; tai resolves them automatically.
**Why deferred:** DAG resolution adds significant complexity (cycle detection, error handling). `tai lint` provides 80% of the value.
**Future scope:** Sidecar YAML or frontmatter declaring `requires: [other-asset]`.

### Profile as Context Container
**What:** Profiles hold environment variables, model constraints, and target-specific flags beyond just asset composition.
**Why deferred:** Current profiles (bundles + target + options) are sufficient for MVP. Environment context can be passed via wrapper or shell.
**Future scope:** `env:`, `constraints:`, and richer `options:` blocks in profile definition.

### Namespacing with Aliases
**What:** Full namespacing (`source::type/name`) with explicit alias promotion to global namespace.
**Why deferred:** Current `source:path` syntax handles most cases. "Later wins" with info logging is sufficient.
**Future scope:** `--strict` mode that fails on conflicts; explicit aliasing in tai.yaml.

### Hooks System
**What:** Pre/post install scripts in config repo (`hooks/post-install.sh`).
**Why deferred:** Security concern—arbitrary script execution from potentially remote sources. Requires careful sandboxing.
**Future scope:** Opt-in hooks with explicit user consent; possibly restricted to local sources only.

### Adapter as State Synchronizer
**What:** Adapters sync to "desired state" rather than copying files—supporting future targets that use DBs, APIs, etc.
**Why deferred:** All current targets (Claude Code, Cursor) are file-based. Premature abstraction.
**Future scope:** Refactor adapter interface when first non-file-based target emerges.

### Skill Manifests
**What:** Extended skill metadata beyond the agentskills.io SKILL.md standard.
**Why deferred:** The agentskills.io standard (SKILL.md with YAML frontmatter) is sufficient for MVP.
**Future scope:** Add when skills need version requirements, build steps, or complex dependencies beyond what SKILL.md supports.

### Config Assets (MCPs and Hooks)
**What:** Support for MCP servers and hooks as first-class asset types, merged into target JSON config files.
**Why deferred:** Requires different installation logic (JSON merge vs. file copy), manifest tracking changes, and scope handling. File-based assets cover the core use cases for v0.1.
**Design decisions:**
- Config assets stored as YAML files in `assets/mcps/` and `assets/hooks/` for consistency with other assets
- Same referencing syntax in bundles (`mcps/github`, `hooks/format-on-save`)
- Filename becomes identifier (MCP server name, hook name) unless overridden
- Installation scope (project/user/local) controlled at profile level with per-asset override option
- Uninstall matches by identifier; warns if entry was modified (user took ownership)
- Adapters declare which asset types they support; unsupported types in a profile cause fail-fast error
**Target version:** v0.2

## Testing Philosophy

1. **No hardcoded paths**: All filesystem paths are injected via config structs
2. **Temp directories**: All tests create isolated temp dirs via `t.TempDir()`
3. **Fixtures**: Helper functions in `internal/testutil` set up fake config repos
4. **Coverage targets**:
   - Unit tests for all parsing/resolution logic
   - Integration tests for adapters
   - E2E tests for full workflows in `e2e/` directory

## Implementation Notes

### CLI Framework

tai uses [github.com/amterp/ra](https://github.com/amterp/ra) for CLI argument parsing. ra provides a clean, declarative API for defining commands with dual-nature arguments.

### UI Libraries

For creating rich terminal output, the following libraries are approved:
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components

## Open Questions

1. ~~**Conflict resolution strategy**~~: **RESOLVED.** Later source/bundle in list wins. tai logs info message when override occurs.

2. **Checksum on re-install**: Should we warn if source file changed but installed file was also modified externally? (Both diverged from manifest.)

3. ~~**Skills structure**~~: **RESOLVED.** Skills follow the [agentskills.io](https://agentskills.io) standard (SKILL.md with YAML frontmatter). Bundles reference them as `skills/my-skill` (directory path, no trailing slash needed).

4. ~~**CLAUDE.md handling**~~: **RESOLVED.** Use `{{existing}}` templating to wrap user content. tai backs up the original and restores it on uninstall.

5. **Profile inheritance**: Should profiles be able to extend other profiles? Deferred—adds complexity but might be useful in the future.

7. **Config asset scope granularity**: Should scope be per-asset, per-asset-type, or only per-profile? Current design uses profile-level default. May need per-asset override for mixed project/user installs.

6. ~~**Templating syntax**~~: **RESOLVED.** Simple `{{variable}}` substitution. Not Go templates or Jinja—just straightforward variable replacement. Special variable `{{existing}}` for injecting current file content.

---

*This spec is a living document. Feedback welcome.*
