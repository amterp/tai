---
title: Composing Bundles
description: How to create bundles, compose them together, and manage cross-source asset references
---

Bundles are named collections of assets - think of them as "shopping lists" that group related assets together. This guide covers creating bundles, nesting them for composition, and referencing assets across sources.

## Creating a basic bundle

A bundle is a YAML file in your config repo's `bundles/` directory. Each entry needs an explicit `type` and `path`:

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily development

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: skill
    path: assets/skills/deploy-checklist
  - type: file
    path: assets/files/project-instructions.md
    dest: ""
```

A few things to note:

- `type` must be one of: `persona`, `delegate`, `skill`, `file`, `statusline`
- `path` is relative to the source's asset directory (the implicit `local` source points to `./assets/`)
- Skills are referenced by directory name, not by `SKILL.md`
- File assets require a `dest` field

## Including other bundles

Bundles can include other bundles using the `bundles` key. This is the primary composition mechanism - build small, focused bundles and combine them into larger ones.

```yaml
# bundles/base.yaml
name: base
description: Foundational persona and delegate

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
```

```yaml
# bundles/ops.yaml
name: ops
description: Operations and deployment tooling

assets:
  - type: skill
    path: assets/skills/deploy-checklist
  - type: delegate
    path: assets/delegates/incident-responder.md
```

```yaml
# bundles/full-setup.yaml
name: full-setup
description: Everything combined

bundles:
  - base
  - ops

assets:
  - type: persona
    path: assets/personas/brainstormer.md
```

When you include `full-setup` in a profile, tai resolves it by expanding `base` and `ops` first, then adding `brainstormer.md`. The result is the union of all assets.

Bundles can include bundles that include other bundles - nesting works to any depth. Tai detects circular references and fails fast if it finds one (e.g., bundle A includes B which includes A).

## Cross-source asset references

When you have multiple sources configured, bundles can reference assets from any source using the `source:path` syntax:

```yaml
# bundles/work-extras.yaml
name: work-extras
description: Additional assets from the work shared source

assets:
  - type: persona
    path: work-shared:personas/company-style.md
  - type: delegate
    path: work-shared:delegates/security-reviewer.md
```

Without a source prefix, the path resolves against the `local` source (your config repo's `assets/` directory). See the [Working with Multiple Sources](/tai/guides/multiple-sources/) guide for more on configuring sources.

## Override behavior

When the same asset path appears more than once - either from nested bundles or from different sources - **later entries win**. Tai logs an info message when an override occurs.

Within a single bundle, asset order matters:

```yaml
# bundles/example.yaml
name: example
description: Later entries override earlier ones

assets:
  - type: persona
    path: assets/personas/senior-swe.md               # This gets overridden
  - type: persona
    path: work-shared:personas/senior-swe.md   # This wins (same path, later entry)
```

When bundles include other bundles, the included bundles are resolved first (in order), then the parent bundle's own assets are applied on top:

```yaml
# bundles/combined.yaml
name: combined

bundles:
  - base        # Resolved first
  - overrides   # Resolved second (wins over base on conflicts)

assets:
  - type: persona
    path: assets/personas/custom.md    # Applied last (wins over everything)
```

In profiles, the same rule applies - later bundles override earlier ones:

```yaml
# profiles/work.yaml
bundles:
  - core           # Base setup
  - work-extras    # Overrides core on conflicts
```

## Real-world example

Here is a realistic set of bundles for a team that has shared standards, project-specific tooling, and individual preferences.

### The core bundle

Shared across all team members. Contains the personas and delegates everyone uses:

```yaml
# bundles/core.yaml
name: core
description: Standard team assets

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: delegate
    path: assets/delegates/quick-summarizer.md
  - type: file
    path: assets/files/team-standards.md
    dest: ""
```

### The backend bundle

For backend developers. Includes core and adds backend-specific tooling:

```yaml
# bundles/backend.yaml
name: backend
description: Backend development setup

bundles:
  - core

assets:
  - type: skill
    path: assets/skills/deploy-checklist
  - type: delegate
    path: assets/delegates/db-migration-helper.md
  - type: statusline
    path: assets/statuslines/detailed.yaml
```

### The frontend bundle

For frontend developers. Also includes core but with different additions:

```yaml
# bundles/frontend.yaml
name: frontend
description: Frontend development setup

bundles:
  - core

assets:
  - type: skill
    path: assets/skills/component-generator
  - type: delegate
    path: assets/delegates/accessibility-checker.md
```

### The lead bundle

For tech leads. Includes both backend and frontend bundles (diamond composition):

```yaml
# bundles/lead.yaml
name: lead
description: Tech lead setup with full visibility

bundles:
  - backend
  - frontend

assets:
  - type: persona
    path: assets/personas/tech-lead.md
  - type: skill
    path: assets/skills/council-participant
```

When tai resolves `lead`, it expands `backend` (which includes `core`), then `frontend` (which also includes `core`). The core assets appear once - deduplicated via tai's diamond resolution. The result is the union of all unique assets from all bundles, plus the lead-specific persona and skill.

### Profiles

Each role gets a profile that binds their bundle to a target:

```yaml
# profiles/backend-dev.yaml
name: backend-dev
target: claude
bundles:
  - backend
```

```yaml
# profiles/tech-lead.yaml
name: tech-lead
target: claude
bundles:
  - lead
```

## Inspecting bundles

Use `tai show bundle` to see what a bundle resolves to:

```bash
tai show bundle lead
```

This displays all assets that would be included, with their sources, showing which bundles contributed each asset.

Use `tai list bundles` to see all available bundles:

```bash
tai list bundles
```

## Next steps

- [Working with Multiple Sources](/tai/guides/multiple-sources/) - reference assets from external sources
- [Switching & Managing Profiles](/tai/guides/managing-profiles/) - install and manage profiles
- [Bundle Format reference](/tai/reference/bundle-format/) - complete bundle YAML specification
