---
title: Codex
description: How tai's Codex adapter maps assets, handles unsupported types, and installs configuration
---

Codex is tai's second supported target. The Codex adapter translates tai's vocabulary into Codex's formats. Codex has no persona concept, so personas are remapped onto skills, with a warning per asset. Delegates become Codex custom agents.

## Target directory

Assets are installed to `$CODEX_HOME` if that environment variable is set. Otherwise, tai defaults to `~/.codex/`.

## Asset mapping

The Codex adapter maps tai asset types to Codex's directory structure:

| Tai asset type | Codex directory | Frontmatter rendered | Notes |
|----------------|-----------------|---------------------|-------|
| `persona` | `skills/` | `name`, `description` | Mapped to skills since Codex has no persona concept; single-file personas are wrapped as `skills/<name>/SKILL.md` (warning per asset) |
| `delegate` | `agents/<name>.toml` | `name`, `description`, `model`, body as `developer_instructions` | Custom agent; see [Delegates](#delegates) |
| `skill` | `skills/` | `name`, `description` | First-class support |
| `file` | Location specified by `dest` | None (copied as-is) | |
| `statusline` | N/A | N/A | Unsupported (skipped with warning) |
| `settings` | N/A | N/A | Unsupported (skipped with warning when the asset has a `native.codex` block, silently otherwise) |

## Partial support and warnings

Codex has first-class support for `AGENTS.md`, skills, and custom agents. It does not have a native persona concept.

Each persona is installed under `skills/` as a regular skill so Codex's skill discovery picks it up. Single-file personas are wrapped into directory form (`skills/<name>/SKILL.md`); directory-form personas keep their layout. A per-asset warning notes the remapping:

```
WARN: Codex does not support personas - installed as skill instead: skills/senior-swe/SKILL.md
```

**StatusLine assets are unsupported entirely.** If a profile includes a statusline asset and targets Codex, the asset is skipped and tai logs a warning. No file is written.

## Delegates

A delegate becomes a Codex custom agent: a TOML file in `agents/`, which Codex loads so a session can spawn the agent by name. This delegate:

```markdown
---
name: code-reviewer
description: Reviews code for correctness.
tools: Read, Grep
model: opus
native:
  codex:
    model: gpt-5
    model_reasoning_effort: high
---

You are a code reviewer.
```

installs as `agents/code-reviewer.toml`:

```toml
name = 'code-reviewer'
description = 'Reviews code for correctness.'
developer_instructions = """
You are a code reviewer.
"""
model = 'gpt-5'
model_reasoning_effort = 'high'
```

tai's `model` and `tools` fields are written for Claude, so Codex gets them as follows:

- **`model`** is written as-is unless it names a Claude model (`sonnet`, `opus`, `haiku`, a `claude-` ID, and the other Claude Code aliases). A Claude model is dropped with a warning, and the agent uses the parent session's model. `inherit` is dropped without a warning, since that is what an agent without a `model` does anyway.
- **`tools`** is always dropped with a warning: Codex agents have no tool allowlist, so the agent can use every tool the parent session can. To restrict it, turn tools off under `native.codex`, for example `features: {shell_tool: false}`.
- **`native.codex`** fields are written at the top level of the TOML and replace any generated field of the same name. Setting `native.codex.model` silences the model warning, and setting `native.codex.features` silences the tools warning, since each tells Codex what to do instead.

Codex skips an agent with no description or no instructions. tai uses the name when `description` is empty, and warns when the body is empty.

A directory delegate (`DELEGATE.md` plus supporting files) cannot become a single TOML file. It is installed as before, as a Markdown reference under `delegates/<name>/` that Codex does not load, with a warning.

Delegates that an earlier version of tai installed under `delegates/<name>.md` are removed on the next install.

## Content transformation

During installation, the adapter re-renders frontmatter for each asset type:

- **Personas** are transformed as skills - they get `name` and `description` fields.
- **Delegates** are rendered as TOML custom agents; see [Delegates](#delegates).
- **Skills** get `name` and `description` fields. Note that unlike Claude Code, Codex skills do not include `tools` or `model` in their frontmatter.
- **Files** receive no transformation. Content is copied as-is.

## Template variables

The Codex adapter provides these variables for use in [templates](/tai/guides/templating/):

| Variable | Value |
|----------|-------|
| `target_name` | `codex` |
| `target_name_display` | `Codex` |
| `target_agents_file` | `AGENTS.md` |

These enable target-aware assets that adapt their content depending on which target they are installed to. User-provided variables (via `--var`) take precedence over adapter-provided ones.

## Running Codex

You can launch Codex through tai using:

```bash
tai run codex [args...]
```

Any arguments after `codex` are passed through to the `codex` CLI command.

## Future targets

## Related pages

- [Assets](/tai/concepts/assets/) - asset types and frontmatter fields
- [Targets](/tai/concepts/targets/) - overview of the target concept
- [Claude Code](/tai/targets/claude-code/) - the Claude Code target adapter
- [OpenCode](/tai/targets/opencode/) - the OpenCode target adapter
- [Templating](/tai/guides/templating/) - using template variables in assets
