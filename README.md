# tai

[![Latest Release](https://img.shields.io/github/v/release/amterp/tai?color=blue)](https://github.com/amterp/tai/releases)
[![Docs](https://img.shields.io/badge/-Docs-grey)](https://amterp.dev/tai/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

**One config repo for your AI coding agents, installed to Claude Code, Codex and OpenCode.**

tai keeps your personas, subagents, skills, hooks, MCP servers and settings in a Git repo you own,
written once in a target-neutral format. `tai install` translates them into each tool's own layout,
records what it wrote, and leaves everything else in the target's directory alone.

## What it looks like

A config repo holds assets, groups them into bundles, and binds bundles to a target with a profile:

```
my-tai-config/
├── tai.yaml
├── assets/
│   ├── personas/senior-swe.md
│   ├── delegates/code-reviewer.md
│   └── skills/release-notes/SKILL.md
├── bundles/core.yaml          # persona + delegate + skill
└── profiles/
    ├── claude.yaml            # target: claude, bundles: [core]
    └── codex.yaml             # target: codex,  bundles: [core]
```

The same bundle installs to both tools:

```
$ tai install claude
ℹ Installing profile 'claude' for claude...
  + output-styles/senior-swe.md (new)
  + agents/code-reviewer.md (new)
  + skills/release-notes (new)

$ tai install codex
ℹ Installing profile 'codex' for codex...
  + skills/senior-swe/SKILL.md (new)
  + agents/code-reviewer.toml (new)
  + skills/release-notes (new)
Warning: Codex does not support personas - installed as skill instead: skills/senior-swe/SKILL.md
```

The delegate becomes a Markdown subagent for Claude Code and a TOML custom agent for Codex. Codex
has no personas, so the persona goes in as a skill, and tai says so rather than dropping it.

If you edit an installed file by hand, the next install stops instead of overwriting your change:

```
$ tai install claude
Error: 1 conflict(s) detected:
  ! agents/code-reviewer.md: file modified since last install

ℹ Use --force to overwrite, --skip-conflicts to skip, or --diff to inspect.
```

## Features

- **Several targets from one source.** Claude Code, Codex and OpenCode, each with an adapter that
  renders its frontmatter and file layout. A field only one tool understands goes under `native`.
- **Files and config entries.** Personas, delegates, skills and loose files are copied. Hooks, MCP
  servers, status lines, settings and auto-approval policy are merged into the target's config files
  one entry at a time, so entries written by you or other tools survive.
- **Composition across repos.** A profile can pull assets and whole bundles from other config repos,
  such as a team's shared repo, with per-import exclusions.
- **Patches on content you don't own.** Keep a small local change to a file from someone else's repo.
  tai applies it on install, and stops the install if upstream changes so the patch no longer applies.
- **Clean switching and uninstall.** A manifest records every file and entry tai wrote. Installing a
  different profile to a target removes what the old one installed there, and `tai uninstall
  <target>` removes everything tai wrote.
- **Templates.** Variables and conditionals, so one `AGENTS.md` can install as `CLAUDE.md` for Claude
  Code and `AGENTS.md` for Codex.

## Install

```bash
go install github.com/amterp/tai/cmd/tai@latest
```

## Quick start

```bash
mkdir -p ~/my-tai-config && cd ~/my-tai-config
# add tai.yaml, an asset, a bundle and a profile - see the quick start guide
tai config set ~/my-tai-config
tai install claude --dry-run    # preview
tai install claude
tai status
```

The [quick start guide](https://amterp.dev/tai/getting-started/quick-start/) writes each file with
you. On a new machine, clone your config repo, run `tai config set` and `tai install`.

## Documentation

- [Docs site](https://amterp.dev/tai/): concepts, guides, and a reference for every file format and
  command.
- [SPEC.md](SPEC.md): the full specification, including design rationale.
- [AGENTS.md](AGENTS.md): architecture and development notes.

## Status

tai is pre-1.0 and used daily by its author on macOS and Linux. File formats may still change between
minor versions; release notes call out anything that needs action.

## Contributing

Issues and pull requests are welcome. Read [AI_POLICY.md](AI_POLICY.md) before opening one, and
[AGENTS.md](AGENTS.md) for how the code is laid out.

## License

[Apache 2.0](LICENSE)
