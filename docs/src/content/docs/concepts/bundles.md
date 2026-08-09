---
title: Bundles
description: Named collections of assets that serve as composable building blocks for profiles
---

Bundles are named collections of [assets](/tai/concepts/assets/). They act as "shopping lists" - each entry references an asset by type and path, and bundles can include other bundles for composition.

## Structure

A bundle is a YAML file in your config repo's `bundles/` directory. Each asset entry requires an explicit `type` and `path`.

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
  - type: skill
    path: skills/council-participant
  - type: file
    path: files/CLAUDE.md
    dest: ""
  - type: statusline
    path: statuslines/detailed.yaml
```

## Valid asset types

| Type | Description |
|------|-------------|
| `persona` | Behavioral style or role |
| `delegate` | Specialized helper |
| `skill` | Procedural capability (often a directory) |
| `file` | Loose file copied as-is (requires `dest`) |
| `statusline` | Status bar configuration |
| `hook` | Command run when something happens in the target |

The `mcp` type is planned.

## Bundle composition

Bundles can include other bundles via the `bundles` field, plus add their own assets on top:

```yaml
# bundles/full-setup.yaml
name: full-setup
description: Comprehensive setup combining multiple bundles

bundles:
  - core
  - work-extras

assets:
  - type: persona
    path: personas/brainstormer.md
```

When `full-setup` is resolved, it includes everything from `core` and `work-extras`, plus the additional `brainstormer` persona.

## Cross-source references

Assets can reference files from any [source](/tai/concepts/sources/) using the `source:path` syntax. If no source is specified, it defaults to the implicit `local` source.

```yaml
# bundles/work-extras.yaml
name: work-extras
description: Additional assets from work sources

assets:
  - type: persona
    path: work-shared:personas/company-style.md
  - type: delegate
    path: work-shared:delegates/security-reviewer.md
```

Here `work-shared` is a source defined in `tai.yaml` that points to a shared directory or another tai config repo.

## Diamond dependency deduplication

When the same asset appears in multiple included bundles, tai deduplicates using "later wins" semantics. The asset from the later bundle in the list takes precedence.

For example, if both `core` and `work-extras` include `personas/senior-swe.md`, and a profile lists them as:

```yaml
bundles:
  - core
  - work-extras
```

The version from `work-extras` wins. Tai logs an info message when an override occurs so you can see what happened.

## Circular reference detection

Tai detects circular bundle references and fails immediately with a clear error. If bundle A includes bundle B and bundle B includes bundle A, installation will not proceed.

This follows tai's fail-fast philosophy - a configuration error should produce a loud failure, not a silent loop or partial install.

## Complete example

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
  - type: file
    path: files/CLAUDE.md
    dest: ""

---

# bundles/work-extras.yaml
name: work-extras
description: Additional assets from work sources

assets:
  - type: persona
    path: work-shared:personas/company-style.md
  - type: delegate
    path: work-shared:delegates/security-reviewer.md
  - type: statusline
    path: statuslines/detailed.yaml

---

# bundles/full-setup.yaml
name: full-setup
description: Everything combined

bundles:
  - core
  - work-extras

assets:
  - type: skill
    path: skills/council-participant
```
