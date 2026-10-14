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
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: skill
    path: assets/skills/council-participant
  - type: file
    path: assets/files/CLAUDE.md
    dest: ""
  - type: statusline
    path: assets/statuslines/detailed.yaml
```

Paths are relative to the [source](/tai/concepts/sources/) root, which for your own repo is the repo itself - hence the `assets/` prefix.

## Valid asset types

| Type | Description |
|------|-------------|
| `persona` | Behavioral style or role |
| `delegate` | Specialized helper |
| `skill` | Procedural capability (often a directory) |
| `file` | Loose file copied as-is (requires `dest`) |
| `statusline` | Status bar configuration |
| `hook` | Command run when something happens in the target |
| `mcp` | Model Context Protocol server |

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
    path: assets/personas/brainstormer.md
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

## Importing bundles from other sources

A `bundles:` entry takes the same `source:name` form, importing a bundle from a source that keeps a `bundles/` directory at its root:

```yaml
# profiles/dev.yaml
bundles:
  - my-local-bundle
  - work:work-tools      # from the "work" source's bundles/work-tools.yaml
```

The import stays live. Add a skill to `work-tools` upstream and it lands on your next install, with nothing to update on your side. That is the point: a hand-copied bundle is a snapshot, and snapshots go stale without ever erroring.

An imported bundle's unprefixed paths resolve against **its own** repo, not yours. When `work-tools` says `assets/skills/dredge`, it means work's copy, even if you have a file at the same path. A prefixed path such as `recl:assets/skills/recl` resolves in **your** `tai.yaml`, so you must declare that source too - tai says so by name if you have not.

### Excluding assets from an import

Machines differ. A bundle you otherwise want may carry one asset that cannot work where you are installing it:

```yaml
bundles:
  - name: work:work-tools
    exclude:
      - assets/skills/beagle
```

Everything else keeps flowing through automatically. Exclusions apply to the import's fully resolved contents, so they reach assets that arrived via a nested include. A pattern matching nothing warns rather than failing - upstream may simply have deleted the asset, which is the outcome you wanted anyway.

For "never take this from that source at all", use the source's own `exclude:` in `tai.yaml` instead. Both drop the asset; the source-level one covers every bundle you import from it.

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

## Post-install steps

A bundle that does nothing until a command runs can carry that command itself, so every profile installing the bundle doesn't have to repeat it:

```yaml
# bundles/approval.yaml
name: approval
assets:
  - type: file
    path: assets/files/approval-policy.yaml
    dest: ""
post_install:
  - name: render
    command: [./scripts/render-approval.sh]
```

The step runs after any install that includes the bundle, once even if the bundle is reached through several imports, and before the profile's own steps. It runs in the root of the repo the bundle came from, so the script path works whichever repo imports it. See [post_install](/tai/reference/bundle-format/#post_install) for the details.

## Complete example

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily work

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: file
    path: assets/files/CLAUDE.md
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
    path: assets/statuslines/detailed.yaml

---

# bundles/full-setup.yaml
name: full-setup
description: Everything combined

bundles:
  - core
  - work-extras

assets:
  - type: skill
    path: assets/skills/council-participant
```
