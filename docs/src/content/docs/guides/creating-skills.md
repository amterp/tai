---
title: Creating Skills
description: How to create skill assets with SKILL.md, supporting files, and external includes
---

Skills are the most powerful asset type in tai. They are directories containing a `SKILL.md` with instructions, plus optional supporting files like scripts, references, and examples. This guide walks through creating a skill from scratch.

## Skill structure

Every skill is a directory under `assets/skills/` with at least a `SKILL.md` file:

```
assets/skills/council-participant/
├── SKILL.md              # Required: frontmatter + instructions
├── scripts/              # Optional: executable code
│   └── tally-votes.sh
├── references/           # Optional: documentation loaded as needed
│   └── voting-protocol.md
└── assets/               # Optional: templates or output-ready files
    └── summary-template.md
```

The `SKILL.md` is the main file - it contains YAML frontmatter and the instructions that the AI agent follows when the skill is activated.

## SKILL.md format

```markdown
---
name: Council Participant
description: >
  Participate in a multi-agent review council. Use this skill when asked to:
  conduct group reviews, gather perspectives from multiple reviewers, or
  synthesize feedback from a panel of specialized agents.
tools: Read, Glob, Grep, Bash
---

# Council Participant

When activated, follow this protocol:

1. Read the item under review
2. Consult each reviewer delegate for their perspective
3. Tally votes using `scripts/tally-votes.sh`
4. Synthesize feedback into a summary using the template in `assets/summary-template.md`

## Review Protocol

For each reviewer:
- Present the item and ask for their assessment
- Record their vote (approve, request-changes, or abstain)
- Capture their key feedback points

## Final Summary

After all reviews, see `references/voting-protocol.md` for tie-breaking rules.
```

### Frontmatter fields

| Field | Required | Notes |
|-------|----------|-------|
| `name` | Yes | Human-readable display name |
| `description` | No | Under 1024 characters. Determines when the skill activates - write it comprehensively |
| `tools` | No | Comma-separated list of tools the skill may use |
| `model` | No | Model to use for this skill (e.g., `haiku`, `sonnet`) |
| `depends` | No | Asset dependencies automatically pulled in when this skill is included |

## Best practices

### Keep SKILL.md concise

The context window is a shared resource. Aim for SKILL.md to stay under 500 lines. If your skill needs extensive documentation, move it to `references/` and instruct the agent to read it on demand:

```markdown
For the full API reference, read `references/api-guide.md`.
For large files, use grep to search for specific sections.
```

### Use progressive disclosure

Information loads in stages, so structure your skill accordingly:

1. **Metadata** (~100 words) - The `name` and `description` fields. Always available to the agent. This is what triggers skill activation, so make the description comprehensive.
2. **SKILL.md body** (<5k words) - Loaded when the skill activates. Contains the core instructions.
3. **Bundled resources** - Files in `scripts/`, `references/`, `assets/`. Loaded as needed during execution.

### Put fragile operations in scripts

For deterministic or critical operations, use executable scripts rather than prose instructions:

```
scripts/
├── validate-config.sh    # Validation that must be exact
├── transform-data.py     # Data processing with specific logic
└── generate-report.sh    # Output formatting
```

Reference them from SKILL.md:

```markdown
Run `scripts/validate-config.sh <path>` to verify the configuration is valid
before proceeding.
```

This avoids the risk of the agent "improvising" on operations that need to be precise.

### Use references for large content

Documentation over 10k words belongs in `references/`, not in SKILL.md. Include grep-friendly patterns so agents can search efficiently:

```markdown
## API Reference

The full API documentation is in `references/api-guide.md`. Key sections:
- Authentication: search for "## Auth"
- Rate limits: search for "## Rate Limiting"
- Error codes: search for "## Error Codes"
```

## Write effective descriptions

The `description` field determines when your skill activates. A good description enumerates what the skill handles and uses action words:

**Good:**

```yaml
description: >
  Participate in a multi-agent review council. Use this skill when asked to:
  conduct group reviews, gather perspectives from multiple reviewers, or
  synthesize feedback from a panel of specialized agents.
```

**Bad:**

```yaml
description: A council skill.
```

The good description lists specific tasks and includes terms a user might mention. The bad one gives the agent nothing to match against.

## Declaring dependencies

Skills often work with specific delegates. Use `depends` to ensure they are automatically included:

```markdown
---
name: Council Participant
description: >
  Participate in a multi-agent review council...
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - delegates/performance-reviewer.md
---
```

When someone includes this skill in a bundle, the three delegates come along automatically. No need to list them separately.

## Complete worked example

Let's build a "deploy-checklist" skill from scratch.

### 1. Create the directory structure

```bash
mkdir -p ~/my-tai-config/assets/skills/deploy-checklist/{scripts,references}
```

### 2. Write SKILL.md

Create `assets/skills/deploy-checklist/SKILL.md`:

```markdown
---
name: Deploy Checklist
description: >
  Run a pre-deployment checklist. Use this skill when asked to: verify a
  deployment is ready, run pre-deploy checks, validate configuration before
  shipping, or audit a release candidate.
tools: Read, Glob, Grep, Bash
---

# Deploy Checklist

Run through the following checks before any deployment.

## Step 1: Run Tests

Execute the full test suite:

```bash
scripts/run-tests.sh
```

If any tests fail, stop and report the failures. Do not proceed to Step 2.

## Step 2: Check Configuration

Verify environment-specific configuration:

1. Read the deployment config file
2. Confirm all required environment variables are set
3. Validate against the schema in `references/config-schema.md`

## Step 3: Audit Dependencies

Check for known vulnerabilities:

```bash
scripts/audit-deps.sh
```

Review the output. Flag any critical or high-severity issues.

## Step 4: Generate Summary

Produce a deployment readiness report covering:
- Test results (pass/fail counts)
- Configuration validation status
- Dependency audit findings
- Overall go/no-go recommendation
```

### 3. Add supporting scripts

Create `assets/skills/deploy-checklist/scripts/run-tests.sh`:

```bash
#!/bin/bash
set -euo pipefail
echo "Running test suite..."
go test ./... 2>&1
echo "Tests complete."
```

Create `assets/skills/deploy-checklist/scripts/audit-deps.sh`:

```bash
#!/bin/bash
set -euo pipefail
echo "Auditing dependencies..."
go list -m all | head -20
echo "Audit complete."
```

### 4. Add reference documentation

Create `assets/skills/deploy-checklist/references/config-schema.md`:

```markdown
# Configuration Schema

## Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `app_name` | string | Application identifier |
| `environment` | string | Target environment (staging, production) |
| `version` | string | Semantic version to deploy |
| `rollback_version` | string | Version to roll back to on failure |

## Optional Fields

| Field | Type | Default |
|-------|------|---------|
| `replicas` | int | 3 |
| `health_check_path` | string | /health |
```

### 5. Reference in a bundle

Add the skill to a bundle in `bundles/ops.yaml`:

```yaml
name: ops
description: Operations and deployment tooling

assets:
  - type: skill
    path: assets/skills/deploy-checklist
```

Note that skills are referenced by their directory name, not the `SKILL.md` file.

### 6. Install and test

```bash
tai install personal --dry-run    # Preview the installation
tai install personal              # Install for real
```

The skill installs to `~/.claude/skills/deploy-checklist/` (for Claude Code) with the full directory structure preserved. `SKILL.md` gets its frontmatter transformed to the target format; all other files are copied as-is.

## Skill includes via tai.yaml

Skills can declare external file dependencies using a `tai.yaml` manifest inside the skill directory. These includes are resolved at install time - the `tai.yaml` file itself is never installed.

### When to use includes

- **Reference documentation** from external projects (syntax guides, API docs)
- **Shared scripts** from a common repository
- **Examples** from upstream projects
- **Machine-specific paths** that vary per developer

### tai.yaml format

Create a `tai.yaml` file inside the skill directory:

```yaml
# assets/skills/my-skill/tai.yaml
include:
  # From a named source (defined in your config repo's tai.yaml)
  - from: rad
    path: SYNTAX.md
    dest: references/           # Trailing "/" preserves the original filename

  # From a URL
  - url: https://raw.githubusercontent.com/example/repo/main/examples.md
    dest: references/examples.md    # Explicit destination path

  # Copy an entire directory
  - from: shared-scripts
    path: helpers/
    dest: scripts/
```

The sources referenced by `from` must be defined in your config repo's `tai.yaml`:

```yaml
# my-tai-config/tai.yaml
version: 1
sources:
  - name: rad
    path: ~/src/rad
  - name: shared-scripts
    path: ~/work/common-scripts
```

This keeps skills portable. The same skill definition works on any machine - each machine just defines its sources with local paths.

### Destination rules

- `dest: dir/` (trailing slash) preserves the original filename: `path: SYNTAX.md` + `dest: references/` produces `references/SYNTAX.md`
- `dest: dir/file.md` (no trailing slash) is an exact target path

### Field mapping for external SKILL.md

When including a SKILL.md from an external source that uses different frontmatter field names, use `fields` to remap them:

```yaml
include:
  - from: external-repo
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title            # Source's "title" field becomes tai's "name"
      description: summary   # Source's "summary" field becomes tai's "description"
```

If the source file already has a field with the tai name (e.g., it already has `name`), the explicit value wins over the mapping.

### Security notes

Skill includes implement several safety measures:
- Path traversal (`../`) is rejected
- Symlinks in source directories are skipped
- URL downloads are limited to 10MB per file with a 30-second timeout
- Only `http://` and `https://` URLs are allowed

Includes follow tai's fail-fast philosophy - a missing source, missing file, or failed URL fetch causes installation to fail immediately.

## Next steps

- [Composing Bundles](/tai/guides/composing-bundles/) - group skills with other assets into bundles
- [Templating](/tai/guides/templating/) - add variables and conditionals to your skill content
- [Asset Formats reference](/tai/reference/asset-formats/) - full field reference for all asset types
- [Skill Includes reference](/tai/reference/skill-includes/) - complete tai.yaml include specification
