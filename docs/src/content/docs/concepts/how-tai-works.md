---
title: How Tai Works
description: Overview of tai's data flow, design principles, and installation pipeline
---

Tai is a CLI tool for managing AI agent configurations across different targets (Claude Code, Codex, and more in the future). You define assets once in tai's vocabulary, compose them into bundles and profiles, and install them to any supported target.

## The problem

AI tools each have their own config formats, directory structures, and terminology. If you use multiple tools - or want to share configs across machines - you end up maintaining duplicate files, manually syncing changes, and dealing with drift.

Tai solves this by giving you a single, version-controlled config repo that serves as your source of truth. Target directories like `~/.claude/` become derived outputs, fully reconstructable from your config.

## Data flow

```
Sources          Assets          Bundles         Profiles        Target
  |                |               |               |               |
  |  provide       |  composed     |  combined     |  installed    |
  +--------------->+-------------->+-------------->+-------------->+
  |                |               |               |               |
  | Local dirs     | Personas      | Named groups  | Bundle(s)     | ~/.claude/
  | Tai repos      | Delegates     | of assets     | + target      | ~/.codex/
  | (Git remotes)  | Skills        | with optional | + options     | etc.
  |                | Files         | nesting       |               |
  |                | StatusLines   |               |               |
  |                | (MCPs, Hooks) |               |               |
```

1. **[Sources](/tai/concepts/sources/)** provide assets from local directories, other tai config repos, or (in the future) git remotes.
2. **[Assets](/tai/concepts/assets/)** are the atomic units - personas, delegates, skills, files, and config entries.
3. **[Bundles](/tai/concepts/bundles/)** group assets into named collections and can nest other bundles for composition.
4. **[Profiles](/tai/concepts/profiles/)** bind one or more bundles to a specific target, producing the final set of assets to install.
5. **[Targets](/tai/concepts/targets/)** are the AI tools tai installs to. Each target has an adapter that translates tai terminology into the target's format.

## Design principles

**Target-agnostic assets.** Assets are defined in tai's own vocabulary - "persona" not "output-style", "delegate" not "agent". Adapters handle the translation during installation, so your config works across Claude Code, Codex, and future targets without changes.

**Additive installation.** Tai never wipes target directories. It only manages files it installed (tracked via a manifest), leaving your other files untouched.

**Fail-fast.** Missing files, circular references, and invalid configs cause immediate failure - not partial installs. A typo in a bundle should produce a loud error, not a silently incomplete setup.

**Version-controlled source of truth.** Your config repo is the canonical source. Target directories are derived outputs you can reconstruct on any machine by running `tai install`.

## What happens during `tai install`

When you run `tai install <profile>`, tai executes the following pipeline for each file asset:

1. **Read** the source file from the resolved source path
2. **Template processing** - substitute variables, evaluate conditionals, resolve `{% include_asset %}` tags (powered by pongo2)
3. **Frontmatter validation** - parse and validate tai frontmatter (name, description, tools, etc.)
4. **Adapter transformation** - the target adapter renders target-specific frontmatter from the parsed metadata
5. **Checksum** - compute SHA-256 of the final content
6. **Write** - copy the transformed file to the target directory

Config assets (like statuslines) follow a different path - they are merged into the target's JSON settings files rather than written as standalone files.

After installation, tai writes a manifest tracking every file it installed, enabling clean re-installs, uninstalls, and status checks.
