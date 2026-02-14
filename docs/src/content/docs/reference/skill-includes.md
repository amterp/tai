---
title: Skill Includes
description: Complete reference for external file dependencies in skill directories
---

Skills can declare external file dependencies via a `tai.yaml` manifest in the skill directory. At install time, tai resolves these includes and assembles them into the installed skill. The `tai.yaml` file itself is a build-time artifact and is never installed to the target.

## Use cases

- **Reference documentation** - include syntax guides, API docs from external projects
- **Shared scripts** - pull helper scripts from a common repository
- **Examples** - fetch example files from upstream projects
- **Machine-specific paths** - define sources in the config repo's `tai.yaml` so paths can vary per machine
- **External SKILL.md** - pull the main skill definition from another source

## Manifest format

The `tai.yaml` file in a skill directory has a single top-level key, `include`, containing a list of include entries.

### Complete example

```yaml
# assets/skills/my-skill/tai.yaml
include:
  # Copy a file from a named source
  - from: rad
    path: SYNTAX.md
    dest: references/          # Trailing "/" preserves original filename

  # Fetch a file from a URL
  - url: https://raw.githubusercontent.com/example/repo/main/guide.md
    dest: examples/guide.md    # Explicit destination path

  # Copy an entire directory
  - from: shared-scripts
    path: helpers/
    dest: scripts/

  # Pull an external SKILL.md with field remapping
  - from: external
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title              # Source's "title" field becomes tai's "name"
      description: summary     # Source's "summary" field becomes tai's "description"
```

## Include fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | yes* | Named source (defined in config repo's `tai.yaml`). Mutually exclusive with `url`. |
| `path` | string | yes* | Path within the source. Required when `from` is used. |
| `url` | string | yes* | URL to fetch. Mutually exclusive with `from`/`path`. |
| `dest` | string | yes | Destination within the skill directory. |
| `fields` | map | no | Frontmatter field remapping (see [Field mapping](#field-mapping)). |

*Either `from` + `path` or `url` must be specified, not both.

## Source-based includes

Source-based includes copy files or directories from a named source.

```yaml
include:
  - from: rad
    path: SYNTAX.md
    dest: references/
```

### Source resolution

The `from` field references a source defined in your config repo's [tai.yaml](/tai/reference/tai-yaml/):

```yaml
# my-tai-config/tai.yaml
sources:
  - name: rad
    path: ~/src/rad
  - name: shared-scripts
    path: ~/work/common-scripts
```

This keeps skills portable - the same skill definition works on any machine because each machine defines its sources with local paths in `tai.yaml`.

If the named source does not exist in `tai.yaml`, installation fails with a clear error.

### Directory includes

When `path` points to a directory, the entire directory is copied recursively:

```yaml
include:
  - from: shared-scripts
    path: helpers/
    dest: scripts/
```

## URL-based includes

URL-based includes fetch a single file from a URL.

```yaml
include:
  - url: https://raw.githubusercontent.com/example/repo/main/file.md
    dest: examples/example.md
```

Only `http://` and `https://` URLs are allowed. Other schemes are rejected.

## Destination semantics

The `dest` field determines where the included file or directory is placed within the skill directory.

| dest format | Behavior | Example |
|-------------|----------|---------|
| Trailing `/` (e.g., `references/`) | Preserve original filename or directory name | `path: SYNTAX.md` + `dest: references/` results in `references/SYNTAX.md` |
| No trailing `/` (e.g., `docs/syntax.md`) | Use as exact target path | `path: SYNTAX.md` + `dest: docs/syntax.md` results in `docs/syntax.md` |

The trailing-slash convention lets you copy files into a directory without renaming them, while the exact-path form lets you rename on copy.

## Field mapping

When including a `SKILL.md` from an external source whose frontmatter uses different field names, the `fields` mapping renames them to tai's vocabulary before validation and transformation.

```yaml
include:
  - from: external
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title          # Source file's "title" becomes tai's "name"
      description: summary # Source file's "summary" becomes tai's "description"
```

The map keys are tai field names and the values are source field names.

### Precedence

If the source file already has the tai field name set (e.g., it has both `title` and `name`), the explicit tai field value wins over the mapping. The mapped source field is removed from the metadata after remapping.

### When field mapping applies

Field mapping only takes effect when the include targets the root-level `SKILL.md` (i.e., `dest: SKILL.md` or `dest: ./` with a source file named `SKILL.md`). Includes targeting other destinations are not subject to field mapping.

## Security measures

Skill includes implement several security protections:

| Measure | Description |
|---------|-------------|
| **Path traversal prevention** | Source and destination paths cannot escape their respective directories. Paths like `../../../etc/passwd` are rejected. |
| **Symlink rejection** | Symlinks in source directories are skipped to prevent escaping the source boundary. |
| **URL scheme validation** | Only `http://` and `https://` URLs are allowed. Other schemes (e.g., `file://`, `ftp://`) are rejected. |
| **Size limits** | URL downloads are limited to 10 MB per file. Downloads exceeding this limit are aborted and the partial file is cleaned up. |
| **HTTP timeout** | URL fetches time out after 30 seconds. |
| **Redirect validation** | HTTP redirects to non-HTTP schemes are blocked. A maximum of 10 redirects is allowed. |

## Error handling

Skill includes follow tai's fail-fast philosophy. Any error causes installation to fail immediately:

| Error | Result |
|-------|--------|
| Named source not found | Installation fails |
| File not found in source | Installation fails |
| URL fetch failure (HTTP error, timeout) | Installation fails |
| Path traversal attempt | Installation fails |
| File exceeds size limit | Installation fails |
| Invalid URL scheme | Installation fails |
| Malformed `tai.yaml` manifest | Installation fails |
| Missing required fields (`from`/`path` or `url`, `dest`) | Installation fails |

## Validation rules

The `tai.yaml` manifest is validated on load:

- Each include must have either `from` + `path` or `url`, not both, not neither
- `from` requires `path` and vice versa
- `dest` is required for all includes
- `url` must use `http://` or `https://` scheme
