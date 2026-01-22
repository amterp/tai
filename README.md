# tai

**tai** is a CLI tool for managing AI agent configurations across different targets. It organizes, composes, and installs target-agnostic assets (personas, delegates, skills, docs) from multiple sources while keeping your configuration version-controlled and portable.

## Why tai?

AI coding assistants like Claude Code and Cursor use configuration files to customize their behavior—output styles, custom agents, skills, and reference documentation. Managing these configurations across machines, sharing them between personal and work contexts, or keeping them version-controlled quickly becomes unwieldy.

tai solves this by:

- **Centralizing configuration** in a version-controlled "config repo" that you own
- **Composing assets** from multiple sources (personal, work, shared team configs)
- **Installing to any supported target** with a single command
- **Tracking what it installs** so you can cleanly switch profiles or uninstall

## Installation

```bash
go install github.com/amterp/tai/cmd/tai@latest
```

## Quick Start

```bash
# 1. Create or clone a config repo
mkdir ~/my-tai-config && cd ~/my-tai-config

# 2. Initialize with a minimal structure
mkdir -p assets/{personas,delegates,skills,docs} bundles profiles

# 3. Create your first persona
cat > assets/personas/senior-swe.md << 'EOF'
# Senior Software Engineer

You are a senior software engineer who values clean, maintainable code.
Focus on architecture and long-term implications of design decisions.
EOF

# 4. Create a bundle referencing your assets
cat > bundles/core.yaml << 'EOF'
name: core
description: My core assets
assets:
  - personas/senior-swe.md
EOF

# 5. Create a profile targeting Claude
cat > profiles/personal.yaml << 'EOF'
name: personal
description: Personal Claude setup
target: claude
bundles:
  - core
EOF

# 6. Create the config manifest
cat > tai.yaml << 'EOF'
version: 1
default_profile: personal
EOF

# 7. Set this as your active config repo and install
tai config set ~/my-tai-config
tai install personal

# Done! Your assets are now installed to ~/.claude/
```

## Core Concepts

| Term | What it is | Claude equivalent |
|------|------------|-------------------|
| **Persona** | Behavioral style/role | output-style |
| **Delegate** | Specialized helper agent | agents (sub-agents) |
| **Skill** | Procedural capability | skills |
| **Doc** | Reference material | CLAUDE.md, supporting docs |
| **Asset** | Any single persona, delegate, skill, or doc | — |
| **Bundle** | Named collection of assets | — |
| **Profile** | Bundle(s) bound to a target | — |
| **Source** | Location providing assets | — |
| **Target** | AI tool to install to | Claude Code, Cursor, etc. |

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
Target (Claude Code, Cursor, etc.)
```

## Config Repo Structure

```
my-tai-config/
├── tai.yaml              # Config manifest
├── assets/               # Your target-agnostic assets
│   ├── personas/         # Behavioral styles
│   ├── delegates/        # Specialized helper agents
│   ├── skills/           # Procedural capabilities (directories)
│   └── docs/             # Reference documentation
├── bundles/              # Bundle definitions (YAML)
└── profiles/             # Profile definitions (YAML)
```

## Configuration Files

### tai.yaml

The manifest for your config repo:

```yaml
version: 1

sources:
  # "local" source is implicit (points to ./assets)
  # Add additional sources as needed:
  - name: work-shared
    path: ~/work/ai-common
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config

default_profile: personal
```

### Bundle Definition

Bundles are "shopping lists" of assets:

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - personas/senior-swe.md
  - delegates/code-reviewer.md
  - docs/CLAUDE.md

# Bundles can include other bundles
bundles:
  - base-setup
```

Reference assets from other sources with `source:path` syntax:

```yaml
assets:
  - work-shared:personas/company-style.md
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
  - personas/brainstormer.md
```

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
tai install <profile> --force      # Overwrite conflicting files
tai install <profile> --skip-conflicts  # Leave conflicts as-is
tai install <profile> --var key=val     # Pass template variables

tai uninstall <target>             # Remove tai-managed files
tai status [target]                # Show what's installed
```

### Running

```bash
tai run <target> [args...]  # Run target (e.g., tai run claude)
```

## Installation Behavior

### Additive, Non-Destructive

tai only manages files it installs. It never wipes your target directory or touches files it doesn't own.

### Conflict Resolution

When a file already exists:

| Scenario | Default behavior |
|----------|------------------|
| tai-managed file, unchanged | Overwrite with new version |
| tai-managed file, user-modified | Error (use `--force` to overwrite) |
| Unmanaged file exists | Error (use `--force` or `--skip-conflicts`) |

### Re-installation

When you run `tai install` with an existing installation, tai:
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

### The `{{existing}}` Variable

Inject the current contents of a target file—useful for wrapping existing configuration:

```markdown
# tai additions

{{existing}}

# End tai additions
```

When using `{{existing}}`, tai backs up the original file and restores it on uninstall.

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
tai locate personas/senior-swe.md
# → /Users/me/my-tai-config/assets/personas/senior-swe.md

vim "$(tai locate personas/senior-swe.md)"
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
    path: ./assets
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config
  - name: company
    path: ~/work/company-ai-config
```

Assets from later sources override earlier ones when paths conflict. tai logs these overrides as info messages.

## Supported Targets

### Claude Code

- Target directory: `~/.claude/`
- Asset mapping:
  - `personas/` → `output-styles/`
  - `delegates/` → `agents/`
  - `skills/` → `skills/`
  - `docs/` → root level

### Future Targets

- Cursor
- Windsurf

## Global State

tai stores its configuration in standard locations:

```
~/.config/tai/
├── config.yaml         # Active config repo path
└── manifests/          # Installation manifests by target
    └── claude.yaml
```

## Skills

Skills follow the [agentskills.io](https://agentskills.io) standard. Unlike other assets (which are single files), skills are directories containing a `SKILL.md` with YAML frontmatter:

```
assets/skills/my-skill/
├── SKILL.md            # Required: YAML frontmatter + description
└── ...                 # Additional files as needed
```

Reference skills in bundles by directory name:

```yaml
assets:
  - skills/my-skill
```

## Design Principles

1. **Target-agnostic assets**: Define assets in tai terminology; adapters translate for each target
2. **Additive installation**: Never wipes target directories; only manages what it installed
3. **Fail-fast**: Missing files or circular references cause immediate failure, not partial installs
4. **Version-controlled source of truth**: Your config repo is canonical; target directories are derived

## Contributing

Contributions welcome! See [AGENTS.md](AGENTS.md) for architecture details and development guidance.

## License

MIT
