# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

**tai** is a CLI tool for managing AI agent configurations across different targets (Claude Code, Cursor, etc.). It organizes, composes, and installs target-agnostic assets (personas, delegates, skills, docs, MCPs, hooks) from multiple sources while maintaining version control and portability.

## Commands

```bash
# Build
go build ./cmd/tai

# Test
go test ./...                    # All tests
go test ./e2e -v                 # E2E tests verbose
go test -run TestBundle ./...    # Run specific test by name

# Lint
golangci-lint run ./...          # Runs errcheck, govet, staticcheck, unused, ineffassign

# Dev script (build + test + lint)
./dev -v                         # Full validation
./dev -b                         # Build only
```

## Architecture

### Core Concepts

| tai term | What it is | Example target equivalent |
|----------|-----------|---------------------------|
| **Target** | AI tool to install to | Claude Code, Cursor, etc. |
| **Persona** | Behavioral style/role | output-style |
| **Delegate** | Specialized helper | agents (sub-agents) |
| **Skill** | Procedural capability | skills |
| **File** | Loose file copied as-is | CLAUDE.md, .cursorrules |
| **StatusLine** | Status bar configuration | statusLine in settings.json |
| **MCP** | Model Context Protocol server | MCP server entry (v0.2) |
| **Hook** | Event-triggered command | Hook in settings (v0.2) |
| **Asset** | Atomic configuration unit | Single persona/delegate/skill/file/statusline/mcp/hook |
| **Bundle** | Named collection of assets | Shopping list of related assets |
| **Profile** | Bundle(s) + target | What you actually install |
| **Source** | Asset provider | Local dir, tai repo |

**Asset shapes:** File assets (personas, delegates, skills, files) are copied to target paths. Config assets (statuslines, mcps, hooks) are merged into target JSON config files.

### Data Flow

```
Config Repo (tai.yaml + bundles/ + profiles/ + assets/)
  ↓
Sources → Assets → Bundles → Profiles → Install Pipeline → Target Config
```

**Install pipeline per file asset:** Read -> Template processing (pongo2: variables, conditionals, include_asset) -> Parse & validate tai frontmatter -> Adapter `TransformContent` -> Checksum -> Write

### Package Structure

- **cmd/tai/main.go** - CLI entry point, command routing & handlers
- **internal/state/** - Global state management; loads config repo, creates resolvers
- **internal/config/** - Core types (TaiConfig, Source, Bundle, Profile, Asset, Manifest) and YAML parsing
- **internal/source/** - Discovers and loads assets from filesystem sources
- **internal/bundle/** - Recursive bundle composition with cycle detection
- **internal/profile/** - Composes bundles into profiles, handles asset deduplication
- **internal/install/** - Installation logic with manifest tracking, conflict detection, backup/restore
- **internal/adapter/** - Target-specific implementations (Claude and Codex)
- **internal/frontmatter/** - YAML frontmatter parsing/rendering (target-agnostic)
- **internal/template/** - Jinja2-style template processing (pongo2): variables, conditionals, `{% include_asset %}`

### Key Design Decisions

1. **Target-agnostic assets**: Assets defined in tai terminology; adapters translate to target format
2. **Additive installation**: Never wipes target directories; only manages tai-installed files via manifest
3. **Fail-fast**: Missing files, circular refs cause immediate failure, not partial install
4. **Diamond deduplication**: Later sources/bundles override earlier ones on conflict (logged as info)
5. **All paths injected**: No hardcoded paths—enables thorough testing with temp directories

### Adapter Interface

Adapters translate tai concepts to target-specific formats. Key methods:
- `TransformContent()` - Renders target-specific frontmatter from parsed tai metadata
- `ProvideVariables()` - Returns target-specific template variables (e.g., `target_name`, `target_agents_file`)
- `MapAssetPath()` - Maps tai asset paths to target paths, interpreting `dest` for file assets

The Claude adapter maps:
- `personas/` → `output-styles/` (renders `name`, `description` frontmatter)
- `delegates/` → `agents/` (renders `name`, `description`, `tools` frontmatter)
- `skills/` → `skills/` (SKILL.md gets `name`, `description`, `tools` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → `statusLine` key in `settings.json` (tai format → Claude format)
- `mcps/` → entries in `.mcp.json` or `~/.claude.json` (v0.2)
- `hooks/` → entries in `settings.json` (v0.2)

Claude's `ProvideVariables()` returns: `target_name: "claude"`, `target_name_display: "Claude"`, `target_agents_file: "CLAUDE.md"`

The Codex adapter maps:
- `personas/` → `personas/` (passive reference files; tai warns per asset)
- `delegates/` → `delegates/` (passive reference files; tai warns per asset)
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)

Codex `ProvideVariables()` returns: `target_name: "codex"`, `target_name_display: "Codex"`, `target_agents_file: "AGENTS.md"`

## User Config Repo Structure

```
my-tai-config/
├── tai.yaml            # Config manifest (version, sources, default_profile)
├── assets/             # Target-agnostic assets
│   ├── personas/       # File assets (can be files or directories)
│   ├── delegates/
│   ├── skills/
│   ├── files/          # Loose files (copied as-is to dest)
│   ├── statuslines/    # Config assets (settings.json)
│   ├── mcps/           # Config assets (v0.2)
│   └── hooks/
├── bundles/            # Bundle definitions (YAML)
└── profiles/           # Profile definitions (YAML)
```

## CLI Commands Reference

```bash
# Configuration
tai config set <path>              # Set active config repo
tai config show                    # Show current config/cache/paths

# Discovery
tai list sources|bundles|profiles|assets [--source X]

# Inspection
tai show profile|bundle <name>
tai locate <asset-path>            # Print full filesystem path

# Installation
tai install <profile> [--dry-run] [--force] [--skip-conflicts]
tai uninstall <target>
tai status [target]

# Convenience
tai run <target> [args...]         # Run target with tai config applied
```

## Project Management

This repo uses `kan` for task tracking (file-based kanban). Leave `.kan/` changes for the user to commit separately.

## Documentation

### Key Documents

- **AGENTS.md** (this file) - Architecture and development guidance for AI agents
- **README.md** - User-facing documentation, installation, and usage
- **SPEC.md** - Complete specification covering all features and design rationale

### The Tai Skill

The `assets/skills/tai/` directory contains a skill for AI agents to use tai effectively. This skill documents CLI usage, config file formats, and skill creation. **Keep this skill updated when making changes** to tai's user-facing behavior—it's the primary reference AI agents use when working with tai configurations.

### Keeping Docs Updated

When making changes, update the relevant documentation:

| Change Type | Update |
|-------------|--------|
| New/changed CLI commands | SPEC.md (command reference), README.md (usage examples), tai skill |
| New concepts or terminology | SPEC.md (definitions), AGENTS.md (core concepts table), tai skill |
| Architecture changes | AGENTS.md (package structure, design decisions), SPEC.md |
| New adapters or targets | AGENTS.md (adapter interface section), SPEC.md |
| User-facing behavior | README.md, SPEC.md, tai skill |
| Skill creation/includes | SPEC.md, tai skill |

Documentation should be updated in the same commit as the code change, not deferred.
