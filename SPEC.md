# tai Specification

**Version:** 0.1 (Draft)
**Status:** Design Phase

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

Additional terms:
- **Asset**: An atomic configuration unit—a single persona, delegate, skill, or doc.
- **Bundle**: A named collection of assets. Acts as a "shopping list" for composing related assets.
- **Profile**: A bundle bound to a specific target. Profiles are what you actually install and run.
- **Source**: A location providing assets—a local directory, another tai config repo, or (future) a git remote.
- **Config Repo**: A repository defining sources, bundles, and profiles. The entry point for tai.
- **Adapter**: Target-specific logic that knows how to build, install, and run a particular AI target.

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
│  personas/, delegates/, skills/, docs/                      │
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
│   │   └── council-participant/
│   └── docs/             # Reference material (→ CLAUDE.md etc.)
│       ├── CLAUDE.md
│       └── RFC2119.md
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
  # Local assets in this repo (always implicitly available as "local")
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
assets:
  - personas/senior-swe.md
  - delegates/code-reviewer.md
  - docs/CLAUDE.md
  - docs/RFC2119.md
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
```

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

## Installation Manifest

```yaml
# ~/.config/tai/manifests/claude.yaml
target: claude
profile: work
config_repo: /Users/amterp/work-tai-config
installed_at: 2024-01-21T14:30:00Z

files:
  - path: output-styles/senior-swe.md
    source: local
    source_path: /Users/amterp/my-tai-config/assets/personas/senior-swe.md
    checksum: sha256:abc123...

  - path: output-styles/company-style.md
    source: work-shared
    source_path: /Users/amterp/work/ai-common/personas/company-style.md
    checksum: sha256:def456...
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

### Remote Source Caching (Future)

When using git remote sources:
- Sources are cached in `~/.cache/tai/sources/`
- `tai sync` fetches/updates cached sources
- `tai install` does NOT require explicit sync first
- `tai install` warns if cache is stale (configurable threshold, default 24h)
- Cache miss during install triggers automatic fetch

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
- [x] Bundle and profile resolution
- [x] Claude adapter with file-copy installation and terminology mapping
- [x] Installation manifest tracking
- [x] Basic CLI: config, list, install, uninstall, status, locate
- [x] `--dry-run` flag for install preview
- [x] `tai run` pass-through

### Deferred to Future Versions

- [ ] Git remote sources
- [ ] Templating/compilation of assets (variable substitution)
- [ ] `--add-dir` runtime injection (vs. installing to ~/.claude)
- [ ] Cursor adapter
- [ ] Merge strategies (append/prepend/template)
- [ ] Asset validation (schema checking)
- [ ] Version compatibility checking
- [ ] `tai lint` command
- [ ] `tai install --strict` mode

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
**What:** Skills have explicit manifest files (`skill.yaml`) declaring metadata, entry points, dependencies.
**Why deferred:** Directory-as-atomic-unit is sufficient for MVP. Skills aren't complex enough yet to need manifests.
**Future scope:** Add when skills need version requirements, build steps, or complex dependencies.

## Open Questions

1. ~~**Conflict resolution strategy**~~: **RESOLVED.** Later source/bundle in list wins. tai logs info message when override occurs.

2. **Checksum on re-install**: Should we warn if source file changed but installed file was also modified externally? (Both diverged from manifest.)

3. **Skills structure**: Skills are directories. How do bundles reference them? Proposal: `skills/my-skill/` (trailing slash indicates directory).

4. **CLAUDE.md handling**: This file is special—it's the root config. Should tai own it entirely, or merge with existing? Proposal: tai owns it via docs/, but supports `includes:` directive to reference other docs.

5. **Profile inheritance**: Should profiles be able to extend other profiles? Adds complexity but might be useful.

6. **Templating syntax**: When we add templating, what syntax? Go templates, Jinja-style, or custom?

---

*This spec is a living document. Feedback welcome.*
