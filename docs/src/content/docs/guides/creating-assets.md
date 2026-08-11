---
title: Creating Personas, Delegates & Files
description: How to create and organize persona, delegate, and file assets for your tai config repo
---

This guide walks through creating each type of file asset - personas, delegates, and files - showing what they look like in your config repo and what tai produces after installation.

## Personas

A persona defines a behavioral style or role for the AI agent. Think of it as a "mode" - senior engineer, brainstormer, technical writer, etc.

### Single-file persona

Create a markdown file in `assets/personas/` with `name` in the frontmatter:

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

Save this as `assets/personas/senior-swe.md`.

### What gets installed

During installation, tai strips its own frontmatter and the target adapter renders the appropriate format. For Claude Code, the file above installs to `~/.claude/output-styles/senior-swe.md` as:

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

The frontmatter looks similar in this case, but tai controls the transformation. Fields unknown to the target adapter are stripped, and the adapter may reformat the output.

### Directory-based persona

When a persona needs supporting files (coding standards, examples, references), use a directory with `PERSONA.md` as the main file:

```
assets/personas/senior-swe/
├── PERSONA.md
└── references/
    └── coding-standards.md
```

`PERSONA.md` contains the same frontmatter as a single-file persona:

```markdown
---
name: Senior SWE
description: Experienced software engineer who writes clean, well-tested code.
---

You are a senior software engineer acting as a co-owner on this codebase.

For coding standards, see the references/ directory.
```

This installs to:

```
~/.claude/output-styles/senior-swe/
├── senior-swe.md           # PERSONA.md renamed to match directory
└── references/
    └── coding-standards.md  # Copied as-is
```

The main file (`PERSONA.md`) gets frontmatter transformation. Supporting files are copied verbatim.

## Delegates

A delegate is a specialized helper - an agent that handles specific tasks like code review, security analysis, or documentation.

### Creating a delegate

Create a markdown file in `assets/delegates/` with frontmatter:

```markdown
---
name: Code Reviewer
description: Reviews code changes for correctness, style, and potential issues.
tools: Read, Glob, Grep, Bash
---

You are a code reviewer. When asked to review code:

1. Check for correctness - does the code do what it claims?
2. Look for edge cases and potential bugs.
3. Evaluate naming, structure, and readability.
4. Suggest concrete improvements with code examples.
5. Note any missing tests or documentation.

Be direct and specific. Reference line numbers when possible.
```

Save this as `assets/delegates/code-reviewer.md`.

### Frontmatter fields for delegates

Delegates support additional fields beyond what personas use:

| Field | Required | Purpose |
|-------|----------|---------|
| `name` | Yes | Human-readable display name |
| `description` | No | What this delegate does |
| `tools` | No | Comma-separated list of tools the delegate may use |
| `model` | No | Model to use (e.g., `haiku`, `sonnet`, `opus`) |
| `depends` | No | Asset dependencies (see below) |

The `tools` field controls which tools the delegate has access to. For Claude Code, this maps directly to the agent's tool permissions.

The `model` field lets you assign a specific model to the delegate. This is useful for cost optimization - a delegate that only reads and summarizes code might work fine with a smaller model:

```markdown
---
name: Quick Summarizer
description: Generates concise summaries of code files.
tools: Read, Glob
model: haiku
---

Summarize the given code files. Focus on:
- What the code does (purpose)
- Key data structures and functions
- Notable patterns or dependencies
```

### What gets installed

For Claude Code, the delegate above installs to `~/.claude/agents/code-reviewer.md`:

```markdown
---
name: Code Reviewer
description: Reviews code changes for correctness, style, and potential issues.
tools: Read, Glob, Grep, Bash
---

You are a code reviewer. When asked to review code:
...
```

### Directory-based delegates

Like personas, delegates can be directories with a `DELEGATE.md` main file:

```
assets/delegates/code-reviewer/
├── DELEGATE.md
├── scripts/
│   └── lint-check.sh
└── references/
    └── style-guide.md
```

## File assets

The `file` type copies content exactly as-is - no frontmatter parsing, no transformation. Use it for loose files that don't fit the persona/delegate/skill categories: project instructions, reference docs, configuration files.

### Creating a file asset

File assets live in `assets/files/` and have no frontmatter requirements. The content is preserved byte-for-byte:

```markdown
# Project Instructions

This codebase uses Go 1.24+ with standard library conventions.

## Build

Run `go build ./cmd/myapp` to build the project.

## Test

Run `go test ./...` for all tests.
```

Save this as `assets/files/project-instructions.md`.

### Referencing in a bundle

File assets require a `dest` field in the bundle (or profile) to specify where they land:

```yaml
# bundles/core.yaml
name: core
description: Essential project assets

assets:
  - type: file
    path: assets/files/project-instructions.md
    dest: ""              # Empty string = target root (~/.claude/)
```

### Dest field behavior

The `dest` field is required for all file assets. It controls where the file is placed relative to the target root:

| `dest` value | Installed path (Claude) |
|-------------|------------------------|
| `""` (empty) | `~/.claude/project-instructions.md` |
| `"docs"` | `~/.claude/docs/project-instructions.md` |
| `"docs/setup.md"` | `~/.claude/docs/setup.md` |

The rules:
- Empty string means the target root directory
- A path with a file extension (e.g., `docs/setup.md`) is treated as the full destination path
- A path without an extension (e.g., `docs`) is treated as a directory - the original filename is appended
- A path ending with `/` (e.g., `output/`) is explicitly a directory

The `dest` field also supports template variables - see the [Templating guide](/tai/guides/templating/) and [Target-Aware Assets guide](/tai/guides/target-aware-assets/) for details.

### Key difference from other asset types

The `dest` field is only valid on file assets. If you add `dest` to a persona or delegate, tai will reject it. Conversely, omitting `dest` on a file asset is an error.

## Asset dependencies

Personas, delegates, and skills can declare dependencies on other assets using the `depends` field. When you include an asset in a bundle or profile, its dependencies are automatically pulled in.

This is especially useful for skills that need specific delegates, or delegates that work together as a group:

```markdown
---
name: Code Review
description: Request a thorough code review with specialized reviewers.
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - delegates/performance-reviewer.md
  - skills/council-participant
---

# Code Review Skill

When asked to review code, orchestrate the review delegates...
```

Each dependency is a path in `type-dir/name` format. You can also reference assets from other sources:

```yaml
depends:
  - delegates/bug-hunter.md              # From local source
  - skills/council-participant           # Directory-based skill
  - work-shared:delegates/helper.md     # From a named source
```

Dependencies are resolved depth-first, deduplicated, and stripped from the installed output. Circular dependencies cause installation to fail with a clear error. See the [Asset Formats reference](/tai/reference/asset-formats/) for full details.

Note that file assets do not support `depends` because their frontmatter is not parsed.

## Putting it all together

Here is a complete example - a config repo with one persona, two delegates, and a file asset:

```
my-tai-config/
├── tai.yaml
├── assets/
│   ├── personas/
│   │   └── senior-swe.md
│   ├── delegates/
│   │   ├── code-reviewer.md
│   │   └── quick-summarizer.md
│   └── files/
│       └── project-instructions.md
├── bundles/
│   └── core.yaml
└── profiles/
    └── personal.yaml
```

The bundle references all of them:

```yaml
# bundles/core.yaml
name: core
description: Essential assets for daily development

assets:
  - type: persona
    path: assets/personas/senior-swe.md
  - type: delegate
    path: assets/delegates/code-reviewer.md
  - type: delegate
    path: assets/delegates/quick-summarizer.md
  - type: file
    path: assets/files/project-instructions.md
    dest: ""
```

And a profile binds the bundle to Claude Code:

```yaml
# profiles/personal.yaml
name: personal
description: Personal development setup
target: claude

bundles:
  - core
```

Install it:

```bash
tai install personal
```

After installation, `~/.claude/` contains:

```
~/.claude/
├── output-styles/
│   └── senior-swe.md
├── agents/
│   ├── code-reviewer.md
│   └── quick-summarizer.md
└── project-instructions.md
```

## Next steps

- [Creating Skills](/tai/guides/creating-skills/) - build directory-based skills with supporting files
- [Composing Bundles](/tai/guides/composing-bundles/) - group assets into reusable collections
- [Templating](/tai/guides/templating/) - add variables and conditionals to your assets
