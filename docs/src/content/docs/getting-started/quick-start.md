---
title: Quick Start
description: Go from zero to a working tai installation in five minutes
---

This guide walks you through creating a minimal config repo and installing it to Claude Code. By the end you'll have a persona and a delegate installed and managed by tai.

## 1. Install tai

If you haven't already, [install the tai CLI](/tai/getting-started/installation/).

## 2. Create a config repo

Create a new directory with the following structure:

```bash
mkdir -p ~/my-tai-config/{assets/{personas,delegates},bundles,profiles}
```

### tai.yaml

Every config repo needs a `tai.yaml` at its root. Create `~/my-tai-config/tai.yaml`:

```yaml
version: 1
default_profile: personal
```

That's it. Tai automatically creates an implicit `local` source pointing to `./assets`, so you don't need to declare it.

### A persona asset

Create `~/my-tai-config/assets/personas/senior-swe.md`:

```markdown
---
name: Senior SWE
description: Experienced software engineer who writes clean, well-tested code.
---

You are a senior software engineer acting as a co-owner on this codebase.

Key principles:
- Write clear, idiomatic code. Favor readability over cleverness.
- Always consider edge cases and error handling.
- When proposing changes, explain the tradeoffs.
- Suggest tests for any non-trivial logic.
```

### A delegate asset

Create `~/my-tai-config/assets/delegates/code-reviewer.md`:

```markdown
---
name: Code Reviewer
description: Reviews code changes for correctness, style, and potential issues.
---

You are a code reviewer. When asked to review code:

1. Check for correctness - does the code do what it claims?
2. Look for edge cases and potential bugs.
3. Evaluate naming, structure, and readability.
4. Suggest concrete improvements with code examples.
5. Note any missing tests or documentation.

Be direct and specific. Reference line numbers when possible.
```

### A bundle

Bundles group assets together. Create `~/my-tai-config/bundles/core.yaml`:

```yaml
name: core
description: Essential assets for daily development

assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
```

### A profile

Profiles bind bundles to a target. Create `~/my-tai-config/profiles/personal.yaml`:

```yaml
name: personal
description: My personal development setup
target: claude

bundles:
  - core
```

## 3. Point tai at the config repo

Tell tai where your config repo lives:

```bash
tai config set ~/my-tai-config
```

Verify it's set correctly:

```bash
tai config show
```

## 4. Preview the installation

Run a dry-run to see what tai would install without actually writing anything:

```bash
tai install personal --dry-run
```

Tai will show the files it would create, where they'd be placed, and the target-specific transformations it would apply.

## 5. Install the profile

When you're satisfied with the preview, install for real:

```bash
tai install personal
```

Tai copies your assets to Claude Code's config directory (`~/.claude/`), translating tai frontmatter into Claude's format. Personas become output-styles, delegates become agents.

## 6. Verify the installation

Check what tai has installed:

```bash
tai status
```

This shows all tai-managed files for each target, along with their current state.

## What's next

- [Config Repo Setup](/tai/getting-started/config-repo-setup/) - understand the full config repo structure
- [Assets](/tai/concepts/assets/) - learn about all asset types (skills, files, statuslines, and more)
- [Bundles](/tai/concepts/bundles/) - compose assets into reusable groups
- [Profiles](/tai/concepts/profiles/) - bind bundles to targets with options
