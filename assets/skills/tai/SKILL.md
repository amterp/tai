---
name: tai
description: >
  Manage AI agent configurations with the tai CLI. Use this skill when asked to:
  manage tai config repos, sources, bundles, or profiles; install or uninstall
  configurations to targets like Claude Code, Codex, Gemini CLI, or OpenCode; create, edit, or
  organize personas, delegates, skills, or files; or create new skills for
  tai-managed configurations.
---

# Tai Configuration Management

Tai organizes AI agent configurations (personas, delegates, skills, files) into composable, version-controlled assets that install to targets like Claude Code, Codex, Gemini CLI, and OpenCode.

## Core Concepts

| Term | Purpose |
|------|---------|
| **Asset** | Atomic config unit: persona, delegate, skill, or file |
| **Bundle** | Named collection of assets (a "shopping list") |
| **Profile** | Bundle(s) bound to a specific target |
| **Source** | Location providing assets (local dir or tai repo) |
| **Target** | AI tool to install to (Claude Code, Codex, Gemini CLI, OpenCode) |

**Data flow:** Sources → Assets → Bundles → Profiles → Target

## Asset Types

Tai uses target-agnostic terminology. Here's how asset types map to current targets:

| Tai Type | Claude | Codex | Gemini | OpenCode | Purpose |
|----------|--------|-------|--------|----------|---------|
| `persona` | `output-styles/` | `skills/` (mapped) | `skills/` (mapped) | `agents/` | Behavioral styles and roles |
| `delegate` | `agents/` | `delegates/` (passive) | unsupported | `agents/` | Specialized helper agents |
| `skill` | `skills/` | `skills/` | `skills/` | `skills/` | Procedural capabilities |
| `file` | (per `dest`) | (per `dest`) | (per `dest`) | (per `dest`) | Loose files copied as-is |

Target-specific notes:
- **Codex**: Personas are mapped to skills since Codex has no persona concept (single-file personas wrap as `skills/<name>/SKILL.md`). Delegates are installed as passive reference files (warns per asset).
- **Gemini**: Personas are mapped to skills (Gemini has no persona concept). Delegates are skipped entirely (no sub-agent system).
- **OpenCode**: Both personas and delegates map to `agents/`, distinguished by `mode: subagent` on delegates.

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
tai install <profile> --diff       # Show diffs for conflicting files
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
│   └── files/            # Loose files (copied as-is to dest)
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

# Each asset requires explicit type and path
assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
  - type: file
    path: files/CLAUDE.md
    dest: ""                       # Required for file type; empty = target root
  - type: skill
    path: skills/my-skill          # Reference skill by directory name

# Include other bundles
bundles:
  - base-setup

# Reference assets from other sources via path prefix
# assets:
#   - type: persona
#     path: work-shared:personas/company-style.md
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
  - type: persona
    path: personas/brainstormer.md
```

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

| Variable | Claude | Codex | Gemini | OpenCode |
|----------|--------|-------|--------|----------|
| `target_name` | `claude` | `codex` | `gemini` | `opencode` |
| `target_name_display` | `Claude` | `Codex` | `Gemini` | `OpenCode` |
| `target_agents_file` | `CLAUDE.md` | `AGENTS.md` | `GEMINI.md` | `AGENTS.md` |

Use these for target-aware file destinations:

```yaml
assets:
  - type: file
    path: files/AGENTS.md
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

Unknown fields are silently ignored. Schema version is tracked at the repo level (`version: 1` in `tai.yaml`), not per-file.

**Note:** The `file` type has no frontmatter requirements—content is copied exactly as-is.

### Asset Dependencies

Assets can declare dependencies on other assets using the `depends` field. When an asset is included in a bundle or profile, its dependencies are automatically pulled in.

```yaml
---
name: Code Review
description: Reviews code with specialized reviewers
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - skills/council-participant
---
```

**Syntax:** Each dependency is a path in `type-dir/name` format:

```yaml
depends:
  - delegates/bug-hunter.md              # from local source
  - skills/council-participant           # directory-based skill
  - other-source:delegates/helper.md     # from named source
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

Files are copied exactly as-is—no frontmatter parsing, no transformation. The `dest` field is required and specifies where the file lands:

```yaml
# In a bundle or profile
assets:
  - type: file
    path: files/CLAUDE.md
    dest: ""              # Empty = target root (~/.claude/CLAUDE.md)

  - type: file
    path: files/config.json
    dest: "config"        # Goes to ~/.claude/config/config.json
```

**Key points:**
- `dest` is required for file type
- Empty `dest` (`""`) means target root directory
- Content is preserved byte-for-byte

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
6. **Reference** in a bundle: `- type: skill` / `path: skills/my-skill`
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
    dest: scripts/

  # Include with field remapping (for SKILL.md from external sources)
  - from: external-repo
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title            # Source's "title" → tai's "name"
      description: summary   # Source's "summary" → tai's "description"
```

### Destination Rules

- `dest: dir/` (trailing slash) → preserves original filename
- `dest: dir/file.md` (no trailing slash) → exact target path

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

## Style Guidelines

- Use **imperative language** ("Run X", "Create Y")
- Write for an agent, not a human reader
- Omit obvious information the agent already knows
- Test instructions before including them
- Structure for scanning (tables, bullet points, code blocks)

## Migration: Per-file tai-schema-version Removed (2026-01-24)

`tai-schema-version` is no longer recognized as a special field. If present in existing files, it's treated as an unknown field and silently ignored. Schema version is tracked at the repo level (`version: 1` in `tai.yaml`).

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
    path: personas/senior-swe.md
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
- Content is copied exactly as-is (no frontmatter processing)

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
    path: files/CLAUDE.md
    dest: ""              # Required: empty = target root
```

If you use `type: doc`, tai will show an error with migration instructions.
