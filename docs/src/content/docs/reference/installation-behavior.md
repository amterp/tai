---
title: Installation Behavior
description: How tai installs, updates, and removes assets, including conflict handling and manifest tracking
---

This page covers the mechanics of `tai install`, `tai reinstall`, and `tai uninstall` - how assets move from your config repo to the target directory, how conflicts are detected and resolved, and how tai tracks what it has installed.

## Design principles

**Additive, non-destructive.** Tai never wipes the target directory. It only manages files it installed, tracked via a manifest. Other files in the target directory are left untouched.

**Fail-fast.** Installation fails immediately (not partially) when it encounters missing files, circular references, invalid configs, or unresolved asset references.

## Install pipeline

When you run `tai install <profile>`, each file asset goes through this pipeline:

```
Read -> Template processing -> Frontmatter parsing -> Adapter transform -> Checksum -> Write
```

### Step by step

1. **Read** - load the source file from the resolved source path

2. **Template processing** - if the file contains template syntax (`{{ }}`, `{% %}`, or `{# #}`), process it through pongo2. This substitutes variables, evaluates conditionals, strips comments, and resolves `{% include_asset %}` tags. See [Template Syntax](/tai/reference/template-syntax/).

3. **Frontmatter parsing and validation** - for personas, delegates, and skills, parse the `---` delimited YAML frontmatter. Validate required fields (e.g., `name`). Separate metadata from body content. File-type assets skip this step entirely.

4. **Adapter transformation** - the target adapter receives the parsed metadata and body content, then renders target-specific frontmatter. For example, the Claude adapter writes `name`, `description`, and `tools` fields for delegates. File-type assets are passed through unchanged.

5. **Checksum** - compute SHA-256 of the final transformed content. This checksum is stored in the manifest and used for conflict detection on subsequent installs.

6. **Write** - copy the transformed content to the target directory. Source file permissions are preserved.

### Config asset pipeline

Config assets (statuslines) follow a different path:

1. **Read** - load the YAML config from the source path
2. **Parse** - determine if tai format (has `level` field) or native format
3. **Validate** - check tai format rules (e.g., `custom` requires `command`)
4. **Transform** - the adapter converts to target-specific JSON format
5. **Checksum** - compute checksum of the transformed JSON
6. **Merge** - read existing settings file, merge in the new entry, write back

### Directory assets

Directory assets (persona, delegate, or skill directories) have additional handling:

1. The entire directory is copied, excluding build artifacts (`tai.yaml` for skills)
2. For skills, includes are resolved (copying files from sources or fetching URLs)
3. The main file (`PERSONA.md`, `DELEGATE.md`, or `SKILL.md`) is transformed with target-specific frontmatter
4. Supporting files are copied as-is

## Two-phase installation

Installation runs in two phases to detect all conflicts upfront rather than failing partway through:

### Phase 1: Resolve and detect conflicts

All assets are resolved (read, template-processed, parsed, transformed) and checked for conflicts. No files are written during this phase. Backup paths for `{{ existing }}` files are computed but backups are not created yet.

If any conflicts are found and neither `--force` nor `--skip-conflicts` is set, tai reports all conflicts at once and exits.

### Phase 2: Execute installation

If no conflicts exist (or conflict flags are set), tai writes all files, creates backups, and removes stale files. The manifest is saved atomically at the end.

This two-phase approach means you see all problems before any changes are made.

## Re-installation behavior

When running `tai install` with an existing installation (manifest exists for the target):

1. Load the current manifest
2. Resolve the new profile's assets through the full pipeline
3. Compare the new asset list against the manifest:

| Category | Description | Action |
|----------|-------------|--------|
| **Added** | In new profile, not in manifest | Install the file |
| **Updated** | In both, checksum changed | Overwrite with new version |
| **Unchanged** | In both, checksum identical | Skip (no write needed) |
| **Removed** | In manifest, not in new profile | Delete the file |

4. Apply changes and update the manifest

For removed files that had a backup (from `{{ existing }}` templating), the original content is restored from the backup instead of deleting the file.

## Conflict handling

Conflicts occur when tai wants to write a file but something unexpected is at the target path.

### Conflict types

| Scenario | Manifest entry? | Current file matches manifest checksum? | Classification |
|----------|-----------------|----------------------------------------|----------------|
| File does not exist | no | N/A | No conflict - install normally |
| File exists, not managed by tai | no | N/A | **Unmanaged conflict** |
| File exists, managed, unchanged | yes | yes | No conflict - safe to overwrite |
| File exists, managed, user modified | yes | no | **User divergence** |
| File uses `{{ existing }}` | no | N/A | No conflict - wrapping is expected |

### Conflict resolution flags

| Flag | Effect |
|------|--------|
| `--diff` | Show unified diffs for each conflict. Does not resolve conflicts - combine with `--force` or `--skip-conflicts`. |
| `--force` / `-f` | Overwrite all conflicting files with tai's version. |
| `--skip-conflicts` | Leave conflicting files as-is. For managed files, the existing manifest entry is preserved. |
| `--dry-run` / `-n` | Preview the entire install without writing anything. Conflicts are still reported. |

When conflicts are detected without any resolution flag:

```
2 conflict(s) detected:
  output-styles/senior-swe.md: file modified since last install
  CLAUDE.md: unmanaged file exists

Use --force to overwrite, --skip-conflicts to skip, or --diff to inspect.
```

With `--diff`, each conflict shows a unified diff between the existing content and what tai would install:

```diff
--- a/output-styles/senior-swe.md
+++ b/output-styles/senior-swe.md
@@ -1,5 +1,5 @@
 ---
 name: Senior SWE
-description: My custom edit.
+description: Experienced software engineer co-owner role.
 ---
```

Directory conflicts and config asset conflicts display a placeholder instead of a diff, since diffing those is more complex.

### Combining flags

- `--dry-run --diff` is a natural preview workflow: see what would change and inspect diffs, without writing anything
- `--force --diff` shows diffs and then overwrites
- `--force` and `--skip-conflicts` are mutually exclusive in intent but can technically both be set; `--force` takes precedence when both apply to the same file

## Config asset installation

Config assets (statuslines) are merged into target JSON settings files rather than written as standalone files.

### Installation process

1. **Read target file** - load the existing JSON settings file (or start with `{}` if it does not exist)
2. **Merge entry** - set the config asset's key to the transformed value (e.g., set `statusLine` key)
3. **Write back** - save the modified JSON with pretty formatting, preserving all non-tai-managed entries
4. **Record in manifest** - track the asset type, name, target file, and checksum

### Uninstallation process

1. **Load manifest** - find config entries tai installed
2. **Read target file** - load the current JSON settings
3. **Remove entry** - delete the key tai installed (e.g., remove `statusLine`)
4. **Write back** - save the modified JSON (the file is kept even if empty)

### Config asset conflict handling

| Scenario | Action |
|----------|--------|
| Key does not exist in settings | Add normally |
| Key exists, not in manifest | **Unmanaged conflict** - use `--force` to overwrite |
| Key exists, in manifest, checksum matches | Update to new version |
| Key exists, in manifest, checksum differs | **User divergence** - use `--force` or `--skip-conflicts` |

## The {{ existing }} backup/restore cycle

The `{{ existing }}` variable has special lifecycle handling:

### On install

1. Tai detects `{{ existing }}` in the source file content
2. If the target file exists, tai copies it to `<path>.tai-backup` (e.g., `CLAUDE.md.tai-backup`)
3. The original content is injected in place of `{{ existing }}` during template processing
4. The backup path is recorded in the manifest entry

### On re-install

1. If the file is still in the profile, the existing backup is used for `{{ existing }}` content
2. If the source template changed, the file is updated with the new template wrapping the original content

### On uninstall

1. Tai reads the manifest entry and finds the backup path
2. The backup is restored (moved back to the original path)
3. The user gets their original file back, as if tai was never installed

### On removal from profile

If a file using `{{ existing }}` is removed from the profile during re-installation, the backup is restored the same way as during uninstall.

## Manifest tracking

Tai records every installation in a manifest file stored at `~/.config/tai/manifests/<target>.yaml`.

### Manifest structure

```yaml
version: 1
target: claude
profile: work
config_repo: /Users/you/my-tai-config
installed_at: 2025-01-21T14:30:00Z

files:
  - path: output-styles/senior-swe.md
    source: local
    source_path: /Users/you/my-tai-config/assets/personas/senior-swe.md
    checksum: sha256:abc123...

  - path: CLAUDE.md
    source: local
    source_path: /Users/you/my-tai-config/assets/files/CLAUDE.md
    checksum: sha256:def456...
    backup_path: /Users/you/.claude/CLAUDE.md.tai-backup
    is_dir: false

  - path: skills/council-participant
    source: local
    source_path: /Users/you/my-tai-config/assets/skills/council-participant
    checksum: sha256:789ghi...
    is_dir: true

config_entries:
  - type: statusline
    name: detailed
    source: local
    source_path: /Users/you/my-tai-config/assets/statuslines/detailed.yaml
    settings_file: /Users/you/.claude/settings.json
    checksum: sha256:status123...
```

### File entry fields

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Target-relative path (e.g., `output-styles/senior-swe.md`) |
| `source` | string | Source name the asset came from |
| `source_path` | string | Absolute filesystem path to the source file |
| `checksum` | string | SHA-256 checksum of the installed content (`sha256:hex...`) |
| `backup_path` | string | Path to backup file (only for `{{ existing }}` files) |
| `is_dir` | boolean | `true` for directory assets |

### Config entry fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Config asset type (e.g., `statusline`) |
| `name` | string | Asset name (filename without extension) |
| `source` | string | Source name |
| `source_path` | string | Absolute path to the source YAML file |
| `settings_file` | string | Path to the JSON settings file this entry was merged into |
| `checksum` | string | SHA-256 checksum of the installed JSON fragment |

### Manifest uses

The manifest enables:
- **`tai uninstall`** - remove exactly what tai installed
- **`tai install`** (re-install) - diff against previous install to add, update, and remove files
- **`tai status`** - show what is currently installed and from where
- **Conflict detection** - compare current file checksums against manifest to detect user modifications

### Atomic writes

Manifests are saved atomically: tai writes to a temporary file first, then renames it to the final path. This prevents corrupt manifests if the process is interrupted.

## Fail-fast philosophy

Tai fails immediately (with a clear error) rather than proceeding with a partial install when:

- A source path does not exist
- An asset referenced in a bundle does not exist in its source
- A bundle referenced in a profile does not exist
- Circular bundle references are detected
- Circular asset dependencies are detected
- Circular `{% include_asset %}` references are detected
- Asset frontmatter validation fails (e.g., missing `name`)
- Template processing fails
- A `dest` path resolves to an empty string
- A `dest` path contains path traversal (`..`)
- A skill include cannot be resolved (source not found, file not found, URL fetch failure)

The rationale: a typo in a bundle definition should produce a loud error, not a silently incomplete installation. Partial installs are harder to debug than outright failures.
