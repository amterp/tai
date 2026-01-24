---
tai-schema-version: 1
name: tai
description: >
  Manage AI agent configurations with the tai CLI. Use this skill when asked to:
  manage tai config repos, sources, bundles, or profiles; install or uninstall
  configurations to targets like Claude Code; create, edit, or organize personas,
  delegates, skills, or docs; or create new skills for tai-managed configurations.
---

# Tai Configuration Management

Tai organizes AI agent configurations (personas, delegates, skills, docs) into composable, version-controlled assets that install to targets like Claude Code.

## Core Concepts

| Term | Purpose |
|------|---------|
| **Asset** | Atomic config unit: persona, delegate, skill, or doc |
| **Bundle** | Named collection of assets (a "shopping list") |
| **Profile** | Bundle(s) bound to a specific target |
| **Source** | Location providing assets (local dir or tai repo) |
| **Target** | AI tool to install to (Claude Code, Cursor, etc.) |

**Data flow:** Sources → Assets → Bundles → Profiles → Target

## CLI Commands

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
tai install <profile> --force      # Overwrite conflicting files
tai install <profile> --skip-conflicts  # Leave conflicts as-is
tai install <profile> --var key=val     # Pass template variables

tai uninstall <target>             # Remove tai-managed files
tai status [target]                # Show what's installed
```

### Running

```bash
tai run <target> [args...]  # Run target with tai config applied
```

## Config Repo Structure

```
my-tai-config/
├── tai.yaml              # Config manifest
├── assets/
│   ├── personas/         # Behavioral styles (→ output-styles)
│   ├── delegates/        # Specialized helpers (→ agents)
│   ├── skills/           # Procedural capabilities (directories)
│   └── docs/             # Reference material
├── bundles/              # Bundle definitions (YAML)
└── profiles/             # Profile definitions (YAML)
```

## Configuration Files

### tai.yaml

```yaml
version: 1

sources:
  # "local" source is implicit (points to ./assets)
  - name: work-shared
    path: ~/work/ai-common
  - name: personal
    type: tai-repo        # References another tai config repo
    path: ~/personal-tai-config

default_profile: personal
```

**Source priority:** Later sources override earlier ones on path conflicts.

### Bundle Definition

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - personas/senior-swe.md
  - delegates/code-reviewer.md
  - docs/CLAUDE.md
  - skills/my-skill          # Reference skill by directory name

# Include other bundles
bundles:
  - base-setup

# Reference assets from other sources
# assets:
#   - work-shared:personas/company-style.md
```

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
  - personas/brainstormer.md
```

## Templating

Assets support `{{variable}}` substitution:

```markdown
# Project: {{project_name}}
Welcome to {{project_name}}.
```

Install with: `tai install work --var project_name=MyApp`

### The `{{existing}}` Variable

Inject current target file contents (useful for wrapping existing config):

```markdown
# Tai additions
{{existing}}
# End Tai additions
```

Tai backs up the original and restores it on uninstall.

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

---

# Creating Personas & Delegates

Personas and delegates are markdown files with required tai frontmatter.

## Persona/Delegate Format

```markdown
---
tai-schema-version: 1
name: Senior SWE
description: Experienced software engineer co-owner role.
---

# Senior SWE

You are a senior software engineer...
```

## Frontmatter Fields

| Field | Personas | Delegates | Skills | Docs |
|-------|----------|-----------|--------|------|
| `tai-schema-version` | required | required | required | required |
| `name` | required | required | required | optional |
| `description` | optional | optional | optional | optional |
| `tools` | — | optional | optional | — |

Only the fields listed above are allowed for each asset type. Unrecognized fields cause an error.

## Docs Format

Docs also require `tai-schema-version: 1` but `name` is optional. During installation, only the body is written (no target frontmatter).

```markdown
---
tai-schema-version: 1
---

# Project Instructions

Reference material here...
```

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
tai-schema-version: 1
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
| `tai-schema-version` | Yes | Must be `"1"`. Stripped during install. |
| `name` | Yes | Human-readable display name |
| `description` | No | Under 1024 chars; triggers skill activation |
| `tools` | No | Comma-separated list of tools the skill may use |

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
6. **Reference** in a bundle: `- skills/my-skill`
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
  delegates, skills, or docs.
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
    dest: scripts/
```

### Destination Rules

- `dest: dir/` (trailing slash) → preserves original filename
- `dest: dir/file.md` (no trailing slash) → exact target path

### Defining Sources

Sources are defined in the config repo's `tai.yaml`, allowing machine-specific paths:

```yaml
# my-tai-config/tai.yaml
sources:
  - name: rad
    path: ~/src/rad          # Different per machine
```

This keeps skills portable—the `tai.yaml` in the skill references source names, not absolute paths.

## Style Guidelines

- Use **imperative language** ("Run X", "Create Y")
- Write for an agent, not a human reader
- Omit obvious information the agent already knows
- Test instructions before including them
- Structure for scanning (tables, bullet points, code blocks)
