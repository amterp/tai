# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

**tai** is a CLI tool for managing AI agent configurations across different targets (Claude Code, Codex, Gemini CLI, OpenCode). It organizes, composes, and installs target-agnostic assets (personas, delegates, skills, docs, MCPs, hooks) from multiple sources while maintaining version control and portability.

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
| **Target** | AI tool to install to | Claude Code, Codex, Gemini CLI, OpenCode |
| **Persona** | Behavioral style/role | output-style |
| **Delegate** | Specialized helper | agents (sub-agents) |
| **Skill** | Procedural capability | skills |
| **File** | Loose file copied as-is | CLAUDE.md, AGENTS.md |
| **StatusLine** | Status bar configuration | statusLine in settings.json |
| **MCP** | Model Context Protocol server | `mcpServers` entry |
| **Hook** | Event-triggered command | Hook entry in settings |
| **Asset** | Atomic configuration unit | Single persona/delegate/skill/file/statusline/mcp/hook |
| **Bundle** | Named collection of assets | Shopping list of related assets |
| **Profile** | Bundle(s) + target | What you actually install |
| **Source** | Asset provider | Local dir, tai repo, command (fetched into cache) |
| **Patch** | Local delta on a source you don't own | - (tai concept) |

**Asset shapes:** File assets (personas, delegates, skills, files) are copied to target paths. Config assets (statuslines, hooks, mcps) are merged into the target's config files - as *entries*, never as whole keys; see design decision 9.

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
- **internal/source/** - Discovers and loads assets from filesystem sources; materializes command sources into `~/.cache/tai/sources/` and patched views into `~/.cache/tai/views/` (`view.go`)
- **internal/cmdexec/** - Shared runner for user-declared commands (skill includes, command sources, post-install hooks): staging dir + TAI_* env, timeouts with process-group cleanup, capped output, atomic swap
- **internal/patch/** - Search/replace patches: parse, validate, apply exactly-once, and derive edits from a diff. Pure; no install/source deps
- **internal/bundle/** - Recursive bundle composition with cycle detection
- **internal/profile/** - Composes bundles into profiles, handles asset deduplication
- **internal/install/** - Installation logic with manifest tracking, conflict detection, backup/restore. `configfile.go` holds the config-file merge engine: batched read/modify/write, plus `applyEdit`/`removeEdit`
- **internal/configasset/** - Config asset types (statusline, hook, mcp): a `Handler` per type that parses and validates its YAML and returns declarative `Edit`s. Knows nothing about manifests or files
- **internal/adapter/** - Target-specific implementations (Claude, Codex, and OpenCode)
- **internal/frontmatter/** - YAML frontmatter parsing/rendering (target-agnostic)
- **internal/template/** - Jinja2-style template processing (pongo2): variables, conditionals, `{% include_asset %}`

### Key Design Decisions

1. **Target-agnostic assets**: Assets defined in tai terminology; adapters translate to target format
2. **Additive installation**: Never wipes target directories; only manages tai-installed files via manifest
3. **Fail-fast**: Missing files, circular refs cause immediate failure, not partial install
4. **Diamond deduplication**: Later sources/bundles override earlier ones on conflict (logged as info)
5. **All paths injected**: No hardcoded paths—enables thorough testing with temp directories
6. **Patched sources are materialized, not patched at read time**: a source declaring `patches:` gets a patched view built in the cache, and `LoadedSource.ContentPath()` redirects all content lookups to it. The install pipeline is unaware patches exist. Patching at each read site instead would make correctness depend on every present and future `os.ReadFile(asset.SourcePath)` remembering to patch—and the failure mode is silent wrong content (this bit us twice: dependency resolution and statusline parsing).
7. **The pristine source is never written to**: views are built from it and swapped in atomically. That is what lets `.blessed` snapshots detect real upstream drift rather than tai's own edits, and keeps `tai locate` pointing at an editable checkout.
8. **Patches fail loudly**: each `find` must match exactly once; 0 or 2+ aborts before anything is written. A patch that silently stops applying would reintroduce content the user explicitly rejected.
9. **tai owns config *entries*, not config keys**: a config asset records the exact edits it made (`ConfigEntry.Edits`: file, key path, merge mode, and a checksum of the entry), and uninstall reverses precisely those. Owning whole keys instead only works while a key has one owner - true for `statusLine`, false for `hooks` and `mcpServers`, where deleting the key would take out entries written by the user and by other tools. The checksum doubles as the "user edited this" signal: a mismatch means they took ownership, so tai leaves it alone.
10. **Config files are merged, never rendered**: they are read into a document, mutated, and written back, so keys tai does not manage survive untouched. JSON is decoded with `UseNumber` - the default `float64` would silently reformat or lose precision on large integers in files like `~/.claude.json`, which holds a session's whole state next to the entries tai manages. A file whose content does not actually change is not rewritten, since every rewrite risks racing a running target.

### Adapter Interface

Adapters translate tai concepts to target-specific formats. Key methods:
- `TransformContent()` - Renders target-specific frontmatter from parsed tai metadata
- `ProvideVariables()` - Returns target-specific template variables (e.g., `target_name`, `target_agents_file`)
- `MapAssetPath()` - Maps tai asset paths to target paths, interpreting `dest` for file assets
- `UnsupportedFileAssetTypes()` - Declares file asset types to skip entirely during install (vs `InstallWarnings` which warns but still installs)
- `ConfigFilePath(assetType, scope)` - Which config file a config asset type installs into at a given scope; empty means unsupported
- `SupportedConfigAssetTypes()` - Config asset types this target supports; others are skipped with a warning

Per-type translation lives behind *optional* interfaces in `internal/configasset` (e.g. `StatusLineTranslator`), which an adapter implements only for the types it supports. Keeping them off `adapter.Adapter` means the base interface does not grow a method every time a config asset type is added, and targets without the feature carry no dead stubs.

The Claude adapter maps:
- `personas/` → `output-styles/` (renders `name`, `description` frontmatter)
- `delegates/` → `agents/` (renders `name`, `description`, `tools` frontmatter)
- `skills/` → `skills/` (SKILL.md gets `name`, `description`, `tools` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → `statusLine` key in `settings.json` (tai format → Claude format)
- `hooks/` → entries under `hooks.<Event>` in `settings.json`
- `mcps/` → entries under `mcpServers` in `.mcp.json` (project) or `~/.claude.json` (user)

Claude's `ProvideVariables()` returns: `target_name: "claude"`, `target_name_display: "Claude"`, `target_agents_file: "CLAUDE.md"`

The Codex adapter maps:
- `personas/` → `skills/` (mapped to skills since Codex has no persona concept; renders `name`, `description` frontmatter; warns on install)
- `delegates/` → `delegates/` (passive reference files; tai warns per asset)
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)
- `hooks/` → entries under `hooks.<Event>` in `~/.codex/hooks.json` (a dedicated JSON file, so tai never rewrites `config.toml`)
- `mcps/` → unsupported for now: Codex keeps servers in `config.toml` beside model settings and project trust levels, and tai has no TOML writer

Codex `ProvideVariables()` returns: `target_name: "codex"`, `target_name_display: "Codex"`, `target_agents_file: "AGENTS.md"`

The OpenCode adapter maps:
- `personas/` → `agents/` (renders `description` only; no `name` since OpenCode uses filename)
- `delegates/` → `agents/` (renders `description`, `mode: subagent`, `model`; no `name` or `tools`)
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)

Both personas and delegates map to `agents/` - OpenCode unifies these into a single agent concept, distinguished by the `mode` field. Tools are omitted because OpenCode uses a structurally incompatible format (boolean map vs comma-separated string). Because both land in `agents/`, a persona and a delegate of the same name collide on one target path; the installer resolves this with "later wins" (the earlier asset is shadowed, logged as an override) so reinstall stays idempotent.

OpenCode `ProvideVariables()` returns: `target_name: "opencode"`, `target_name_display: "OpenCode"`, `target_agents_file: "AGENTS.md"`

The Gemini adapter maps:
- `personas/` → `skills/` (mapped to skills since Gemini has no persona concept; renders `name`, `description` frontmatter; warns on install)
- `delegates/` → unsupported (skipped with warning; Gemini has no user-defined sub-agents)
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)
- `hooks/` → entries under `hooks.<Event>` in `settings.json`, with every event renamed (`pre_tool_use` → `BeforeTool`, `stop` → `AfterAgent`) and timeouts in milliseconds
- `mcps/` → entries under `mcpServers` in `settings.json`

Gemini `ProvideVariables()` returns: `target_name: "gemini"`, `target_name_display: "Gemini"`, `target_agents_file: "GEMINI.md"`

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
│   ├── hooks/          # Config assets (commands run on target events)
│   ├── mcps/           # Config assets (MCP servers)
│   └── hooks/
├── bundles/            # Bundle definitions (YAML)
├── patches/            # Local deltas, one dir per patched source
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
tai refresh [source]               # Fetch command sources without installing

# Patches
tai patch list                     # Status of every patch
tai patch diff <name>              # Blessed snapshot vs current upstream
tai patch bless <name>|--all       # Record current upstream as reviewed
tai patch workspace <source>       # Editable copy for authoring/repairing
tai patch regenerate <name>        # Rewrite a patch from workspace edits

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
