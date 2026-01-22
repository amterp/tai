# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

**tai** is a CLI tool for managing AI agent configurations across different targets (Claude Code, Cursor, etc.). It organizes, composes, and installs target-agnostic assets (personas, delegates, skills, docs) from multiple sources while maintaining version control and portability.

## Commands

```bash
# Build
go build ./cmd/tai

# Test
go test ./...                    # All tests
go test ./e2e -v                 # E2E tests verbose
go test -run TestBundle ./...    # Run specific test by name
```

## Architecture

### Core Concepts

| tai term | What it is | Example target equivalent |
|----------|-----------|---------------------------|
| **Target** | AI tool to install to | Claude Code, Cursor, etc. |
| **Persona** | Behavioral style/role | output-style |
| **Delegate** | Specialized helper | agents (sub-agents) |
| **Skill** | Procedural capability | skills |
| **Doc** | Reference material | CLAUDE.md, .cursorrules |
| **Asset** | Atomic configuration unit | Single persona/delegate/skill/doc |
| **Bundle** | Named collection of assets | Shopping list of related assets |
| **Profile** | Bundle(s) + target | What you actually install |
| **Source** | Asset provider | Local dir, tai repo |

### Data Flow

```
Config Repo (tai.yaml + bundles/ + profiles/ + assets/)
  ↓
Sources → Assets → Bundles → Profiles → Target Config
```

### Package Structure

- **cmd/tai/main.go** - CLI entry point, command routing & handlers
- **internal/state/** - Global state management; loads config repo, creates resolvers
- **internal/config/** - Core types (TaiConfig, Source, Bundle, Profile, Asset, Manifest) and YAML parsing
- **internal/source/** - Discovers and loads assets from filesystem sources
- **internal/bundle/** - Recursive bundle composition with cycle detection
- **internal/profile/** - Composes bundles into profiles, handles asset deduplication
- **internal/install/** - Installation logic with manifest tracking, conflict detection, backup/restore
- **internal/adapter/** - Target-specific implementations (currently Claude)
- **internal/template/** - `{{variable}}` and `{{existing}}` substitution

### Key Design Decisions

1. **Target-agnostic assets**: Assets defined in tai terminology; adapters translate to target format
2. **Additive installation**: Never wipes target directories; only manages tai-installed files via manifest
3. **Fail-fast**: Missing files, circular refs cause immediate failure, not partial install
4. **Diamond deduplication**: Later sources/bundles override earlier ones on conflict (logged as info)
5. **All paths injected**: No hardcoded paths—enables thorough testing with temp directories

### Adapter Interface

Adapters translate tai concepts to target-specific formats. The Claude adapter maps:
- `personas/` → `output-styles/`
- `delegates/` → `agents/`
- `skills/` → `skills/`
- `docs/` → root level

## User Config Repo Structure

```
my-tai-config/
├── tai.yaml            # Config manifest (version, sources, default_profile)
├── assets/             # Target-agnostic assets
│   ├── personas/
│   ├── delegates/
│   ├── skills/
│   └── docs/
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
- **SPEC.md** - Complete specification (735 lines) covering all features and design rationale

### Keeping Docs Updated

When making changes, update the relevant documentation:

| Change Type | Update |
|-------------|--------|
| New/changed CLI commands | SPEC.md (command reference), README.md (usage examples) |
| New concepts or terminology | SPEC.md (definitions), AGENTS.md (core concepts table) |
| Architecture changes | AGENTS.md (package structure, design decisions), SPEC.md |
| New adapters or targets | AGENTS.md (adapter interface section), SPEC.md |
| User-facing behavior | README.md, SPEC.md |

Documentation should be updated in the same commit as the code change, not deferred.
