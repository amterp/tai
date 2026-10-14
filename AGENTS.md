# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

**tai** is a CLI tool for managing AI agent configurations across different targets (Claude Code, Codex, OpenCode). It organizes, composes, and installs target-agnostic assets (personas, delegates, skills, docs, MCPs, hooks) from multiple sources while maintaining version control and portability.

## Commands

```bash
# Build
go build ./cmd/tai

# Test
go test ./...                    # All tests
go test ./e2e -v                 # E2E tests verbose
go test -run TestBundle ./...    # Run specific test by name

# Lint
golangci-lint run ./...          # Runs errcheck, govet, staticcheck, unused, ineffassign, and checks gofmt

# Dev script (build + test + lint)
./dev -v                         # Full validation
./dev -b                         # Build only
```

## Architecture

### Core Concepts

| tai term | What it is | Example target equivalent |
|----------|-----------|---------------------------|
| **Target** | AI tool to install to | Claude Code, Codex, OpenCode |
| **Persona** | Behavioral style/role | output-style |
| **Delegate** | Specialized helper | agents (sub-agents) |
| **Skill** | Procedural capability | skills |
| **File** | Loose file copied as-is | CLAUDE.md, AGENTS.md |
| **StatusLine** | Status bar configuration | statusLine in settings.json |
| **MCP** | Model Context Protocol server | `mcpServers` entry |
| **Hook** | Event-triggered command | Hook entry in settings |
| **Settings** | Fields in a target's settings file | `env`, `enabledPlugins` in `settings.json` |
| **Approval** | Standing policy for an auto-approval reviewer, as entries of kind context/allow/deny | `autoMode` arrays in `settings.json` |
| **Asset** | Atomic configuration unit | Single persona/delegate/skill/file/statusline/mcp/hook/settings/approval |
| **Bundle** | Named collection of assets | Shopping list of related assets |
| **Profile** | Bundle(s) + target | What you actually install |
| **Source** | Asset provider | Local dir, tai repo, command (fetched into cache) |
| **Patch** | Local delta on a source you don't own | - (tai concept) |

**Asset shapes:** File assets (personas, delegates, skills, files) are copied to target paths. Config assets (statuslines, hooks, mcps, settings, approvals) are merged into the target's config files - as *entries*, never as whole keys; see design decision 9.

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
- **internal/configasset/** - Config asset types (statusline, hook, mcp, settings, approval): a `Handler` per type that parses and validates its YAML and returns declarative `Edit`s. Knows nothing about manifests or files
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
9. **tai owns config *entries*, not config keys**: a config asset records the exact edits it made (`ConfigEntry.Edits`: file, key path, merge mode, and a checksum of the entry), and uninstall reverses precisely those. Owning whole keys instead only works while a key has one owner - true for `statusLine`, false for `hooks` and `mcpServers`, where deleting the key would take out entries written by the user and by other tools. The checksum doubles as the "user edited this" signal, and both ends of the install honor it: reinstalling over an edited entry is a conflict needing `--force`, and uninstall leaves it in place. Without the install-side gate the detection is inert - the removal declines to touch the entry and the reapply overwrites it two lines later. An array element also records the index it was written at, since a checksum alone cannot tell an edited entry from a deleted one, and guessing "deleted" appends a second copy of a hook that then fires twice.
10. **A source is a directory root; references are literal paths from it**: `path` gets nothing appended and no layout inferred, so `work:assets/skills/rad` opens exactly that. The removed `type: tai-repo` appended `/assets`, which put a repo's `bundles/` outside anything a reference could address and made a path's meaning depend on a field elsewhere in the file. Two directories remain conventions - `<root>/bundles/` for importable bundles, and the asset type dirs (scanned at the root and under `assets/`) for `tai list assets` - both only for what tai *discovers*, never for how a written path *resolves*. Bundles stay addressed by name because a bundle's identity is its `name:` field, not its filename.
11. **An imported bundle resolves unprefixed references against its own repo**: upstream writes `assets/skills/dredge` meaning its repo, and that has to survive being imported, or the first unprefixed path breaks every import - and breaks it silently, by resolving to the consumer's same-named file. Source *prefixes* are the exception and resolve in the consumer's `tai.yaml`, the only one tai loads, so an imported bundle reaching a third source requires the consumer to declare it too. `dependency.resolveDepRef` already worked this way for `depends:`; it derives an asset's type from the directory holding it rather than the first path segment, so the type does not shift with how deep the source root sits.
12. **Source exclusions error on your references and filter upstream's**: naming an excluded asset in your own config is a contradiction and stays a hard error; an imported bundle naming one is a reference you never wrote and cannot edit, so it is dropped and logged. Without the split the two features cannot coexist - every source exclusion would have to be mirrored as a per-import exclusion on every bundle touching it, or installs break on upstream's edits. A per-import `exclude` applies to the fully resolved asset set (reaching through nested includes), and an unmatched pattern warns rather than failing, since upstream deleting the asset already satisfies the intent.
13. **Config files are merged, never rendered**: they are read into a document, mutated, and written back, so keys tai does not manage survive untouched. JSON is decoded with `UseNumber` - the default `float64` would silently reformat or lose precision on large integers in files like `~/.claude.json`, which holds a session's whole state next to the entries tai manages. A file whose content does not actually change is not rewritten, since every rewrite risks racing a running target.
14. **A target-specific field always goes in `native`, keyed by target name**: one spelling across hooks, MCPs and file-asset frontmatter, merged verbatim over what the adapter generated. The alternatives both fail on their own terms - passing unknown frontmatter through wholesale writes Claude-only fields into the Codex and OpenCode copies, which is the opposite of what target-agnostic assets mean, and a per-target allowlist needs a tai release every time Claude Code adds a field. Promoting a field to tai's own vocabulary is the other direction, and the test is reach rather than obscurity: a concept several targets share is worth abstracting so it is written once and translated per target, which is what `tools` and `model` are. Whatever is dropped says so - a field outside tai's vocabulary, a retired one, a `native` key naming no known target - because a dropped unknown field and a dropped typo look identical in the installed file, so silence leaves diffing against source as the only way to find out. A source can declare `native_frontmatter: <target>`, which treats its assets' unknown fields as written under `native.<target>`; that fits this rule because the user names the one target they reach. The list of target names is written down in `internal/adapter` rather than read from `DefaultRegistry`, which only holds adapters the current binary linked in and would report a real target as unknown.
15. **Templating is opted out per source, and the opt-out follows the content**: `template: false` on a source installs its content as written - its own assets, files a skill includes from it, and `include_asset` of it - because third-party markdown routinely contains `{#` heading anchors that pongo2 reads as an unclosed comment. Turning a parse error into skip-with-warning was rejected: tai has no notion of which sources are "not yours", and a skip installs something other than what was asked for, which the fail-fast rule exists to prevent. A file asset's `dest` stays templated, since it lives in the user's config rather than the source.

### Adapter Interface

Adapters translate tai concepts to target-specific formats. Key methods:
- `TransformContent()` - Renders target-specific frontmatter from parsed tai metadata
- `ProvideVariables()` - Returns target-specific template variables (e.g., `target_name`, `target_agents_file`)
- `MapAssetPath()` - Maps tai asset paths to target paths, interpreting `dest` for file assets
- `UnsupportedFileAssetTypes()` - Declares file asset types to skip entirely during install (vs `InstallWarnings` which warns but still installs)
- `ConfigFilePath(assetType, scope)` - Which config file a config asset type installs into at a given scope; empty means unsupported
- `SupportedConfigAssetTypes()` - Config asset types this target supports; others are skipped with a warning

Per-type translation lives behind *optional* interfaces in `internal/configasset` (e.g. `StatusLineTranslator`), which an adapter implements only for the types it supports. Keeping them off `adapter.Adapter` means the base interface does not grow a method every time a config asset type is added, and targets without the feature carry no dead stubs.

Every adapter merges an asset's `native.<target>` frontmatter over whatever it rendered, so the per-type lists below are what tai produces before that.

The Claude adapter maps:
- `personas/` → `output-styles/` (renders `name`, `description` frontmatter)
- `delegates/` → `agents/` (renders `name`, `description`, `tools`, `model` frontmatter)
- `skills/` → `skills/` (SKILL.md gets `name`, `description`, `tools`, `model` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → `statusLine` key in `settings.json` (tai format → Claude format)
- `hooks/` → entries under `hooks.<Event>` in `settings.json`
- `mcps/` → entries under `mcpServers` in `.mcp.json` (project) or `~/.claude.json` (user); `local` scope nests under `projects.<path>.mcpServers` in the same file, which is where Claude keeps a server private to one project
- `settings/` → the fields under `native.claude`, merged into `settings.json` with each value inside an object owned separately and each array element appended (`configasset.FieldEdits`)
- `approvals/` → each entry appended to `autoMode.environment`/`allow`/`soft_deny` in user `settings.json` only (the classifier ignores other scopes); an array tai creates is seeded with `"$defaults"` via the `SeedArray` merge mode, which is never recorded or removed

Claude's `ProvideVariables()` returns: `target_name: "claude"`, `target_name_display: "Claude"`, `target_agents_file: "CLAUDE.md"`

The Codex adapter maps:
- `personas/` → `skills/` (mapped to skills since Codex has no persona concept; renders `name`, `description` frontmatter; warns on install)
- `delegates/` → `agents/<name>.toml`, a Codex custom agent: `name`, `description`, and the body as `developer_instructions`. `model` is kept unless it names a Claude model, and `tools` is dropped (Codex has no tool allowlist), each with a warning unless `native.codex` covers it. `native.codex` merges at the top level of the TOML. A directory delegate has no single-file form and stays a passive reference under `delegates/`, with a warning
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)
- `settings/` → unsupported (skipped with warning only when the asset has a block for this target)
- `approvals/` → unsupported (skipped with warning only when an entry applies to this target)
- `hooks/` → entries under `hooks.<Event>` in `~/.codex/hooks.json` (a dedicated JSON file, so tai never rewrites `config.toml`)
- `mcps/` → entries under `[mcp_servers]` in `~/.codex/config.toml`, gated behind a one-time consent prompt because writing that file loses comments and key order

Codex `ProvideVariables()` returns: `target_name: "codex"`, `target_name_display: "Codex"`, `target_agents_file: "AGENTS.md"`

The OpenCode adapter maps:
- `personas/` → `agents/` (renders `description` only; no `name` since OpenCode uses filename)
- `delegates/` → `agents/` (renders `description`, `mode: subagent`, `model`; no `name` or `tools`)
- `skills/` → `skills/` (SKILL.md gets `name`, `description` frontmatter)
- `files/` → location specified by `dest` field (content copied as-is, no transformation)
- `statuslines/` → unsupported (skipped with warning)
- `settings/` → unsupported (skipped with warning only when the asset has a block for this target)
- `approvals/` → unsupported (skipped with warning only when an entry applies to this target)

Both personas and delegates map to `agents/` - OpenCode unifies these into a single agent concept, distinguished by the `mode` field. Tools are omitted because OpenCode uses a structurally incompatible format (boolean map vs comma-separated string). Because both land in `agents/`, a persona and a delegate of the same name collide on one target path; the installer resolves this with "later wins" (the earlier asset is shadowed, logged as an override) so reinstall stays idempotent.

OpenCode `ProvideVariables()` returns: `target_name: "opencode"`, `target_name_display: "OpenCode"`, `target_agents_file: "AGENTS.md"`

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
│   ├── settings/       # Config assets (fields in a target's settings file)
│   └── approvals/      # Config assets (auto-approval policy, Markdown)
├── bundles/            # Bundle definitions (YAML)
├── patches/            # Local deltas, one dir per patched source
└── profiles/           # Profile definitions (YAML)
```

## CLI Commands Reference

```bash
# Configuration
tai config set <path>              # Set active config repo
tai config show                    # Show current config/cache/paths
tai --config <repo> <command>      # Read another repo for one command (or TAI_CONFIG); read-only

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
- **docs/** - The user-facing docs site (Astro); pages under `docs/src/content/docs/`

### The Tai Skill

The `assets/skills/tai/` directory contains a skill for AI agents to use tai effectively. This skill documents CLI usage, config file formats, and skill creation. **Keep this skill updated when making changes** to tai's user-facing behavior—it's the primary reference AI agents use when working with tai configurations.

### Keeping Docs Updated

When making changes, update the relevant documentation:

| Change Type | Update |
|-------------|--------|
| New/changed CLI commands | SPEC.md (command reference), README.md (usage examples), tai skill, docs site (`reference/cli.md`) |
| New concepts or terminology | SPEC.md (definitions), AGENTS.md (core concepts table), tai skill, docs site (`concepts/`) |
| Architecture changes | AGENTS.md (package structure, design decisions), SPEC.md |
| New adapters or targets | AGENTS.md (adapter interface section), SPEC.md, docs site (`targets/`, `concepts/targets.md`) |
| User-facing behavior | README.md, SPEC.md, tai skill, docs site |
| Skill creation/includes | SPEC.md, tai skill, docs site (`reference/skill-includes.md`) |

The docs site lives in `docs/src/content/docs/`. It repeats much of SPEC.md for users, so a
behavior change usually has a stale copy there too: grep it for the old behavior.

Documentation should be updated in the same commit as the code change, not deferred.
