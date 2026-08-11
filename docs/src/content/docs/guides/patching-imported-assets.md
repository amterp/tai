---
title: Patching Imported Assets
description: Keep a local edit on a file you pull from someone else's repo or a raw URL, and handle it when upstream changes
---

You import a skill from a repo you do not control. It is almost right - one paragraph tells your team to do something you stopped doing a year ago. You want that one line changed, you want the rest of the file to keep improving, and you want to know when upstream touches it.

This guide walks the whole loop: declare the patch, review upstream, and repair the patch when upstream breaks it.

## Set up the source

Two shapes come up most often.

**A repo you have checked out locally:**

```yaml
# tai.yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    patches: patches/shared-skills/
```

**A single file off a raw host:**

```yaml
# tai.yaml
sources:
  - name: upstream-guide
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    patches: patches/upstream-guide/
```

A [url source](/tai/concepts/sources/#url-source) carries no assets of its own - it is one file in a directory. Reach it from a skill with an include:

```yaml
# assets/skills/mine/tai.yaml
include:
  - from: upstream-guide
    path: GUIDE.md
    dest: refs/guide.md
```

The include receives the patched content. Nothing else in the skill needs to know.

The rest of this guide uses the url source, but every step is identical for the local one.

## Write the patch

Create a file in the patch directory. The name becomes the patch's name.

```yaml
# patches/upstream-guide/use-pnpm.yaml
version: 1
file: GUIDE.md
description: we standardized on pnpm
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
```

`file` is relative to the source. For the url source above it is just the filename; for the local repo it would be something like `skills/rad/SKILL.md`.

Fetch the source and check the patch applies:

```bash
tai refresh
tai patch list
```

```
Patches:
  upstream-guide:use-pnpm  we standardized on pnpm (upstream-guide:GUIDE.md)  unblessed
```

`unblessed` means the patch applies but you have not recorded what upstream looked like when you wrote it. That is the next step.

## Bless it

```bash
tai patch bless upstream-guide:use-pnpm
```

```
Blessed 'upstream-guide:use-pnpm' against current upstream-guide:GUIDE.md
Commit the .blessed file(s) so drift detection travels with your config repo.
```

This writes `patches/upstream-guide/use-pnpm.blessed` - a copy of the upstream file exactly as it is right now. Commit it. It is what tai compares against later to tell you upstream moved.

Now install:

```bash
tai install my-profile
```

The installed file has your edit. The cached copy of upstream does not - tai never writes to the pristine source, so the snapshot stays an honest mirror.

## When upstream changes

Two things can happen, and tai treats them differently.

### Upstream moved, the patch still applies

```
Warning: upstream file for patch 'use-pnpm' changed since it was blessed
  file: upstream-guide:GUIDE.md
  The patch still applies cleanly; installing the patched content.
  Review what changed:  tai patch diff upstream-guide:use-pnpm
  Accept it:            tai patch bless upstream-guide:use-pnpm
```

The install proceeds. Nothing is broken - but something changed in a file you have an opinion about, so tai makes you look:

```bash
tai patch diff upstream-guide:use-pnpm
```

```diff
--- a/upstream-guide:GUIDE.md
+++ b/upstream-guide:GUIDE.md
@@ -2,4 +2,4 @@

 Always install with npm.

-Run the tests.
+Run the tests with coverage.
```

If the change is fine, accept it and the warning clears:

```bash
tai patch bless upstream-guide:use-pnpm
```

### Upstream moved, the patch no longer applies

The install **fails**, and nothing is written:

```
Error: patch 'use-pnpm' could not be applied - install aborted, nothing was changed:

  patch:  ~/tai-config/patches/upstream-guide/use-pnpm.yaml
  file:   upstream-guide:GUIDE.md
  edit #1: find text not found - upstream has likely changed since this patch was written
    | Always install with npm.

  To fix:
    1. See what changed upstream:             tai patch diff use-pnpm
    2. Edit the file to match what you want:  ~/.cache/tai/workspaces/<key>/upstream-guide
    3. Turn that edit into the patch:         tai patch regenerate use-pnpm
    4. Review the rewritten edits:            ~/tai-config/patches/upstream-guide/use-pnpm.yaml
    5. Accept the new upstream:               tai patch bless use-pnpm
    6. Re-run the install
```

This is the point of the exactly-once rule. Upstream rewrote the line your patch anchors on, and tai refuses to guess what you meant.

## Repair the patch

Do not hand-edit the `find` block. Use the workspace, which tai has already built for you:

```bash
tai patch workspace upstream-guide
```

```
Workspace ready: ~/.cache/tai/workspaces/<key>/upstream-guide
Warning: These patches could not be applied and are NOT reflected in the workspace:
  use-pnpm
Re-make their changes by hand, then regenerate them.
```

The workspace is a normal directory with the new upstream content and every patch that still applies. Open the file and make your edit again - this time against the text upstream actually has now:

```
# GUIDE.md in the workspace
Always install with yarn.     <- upstream's new line
```

Change it to what you want:

```
Always install with pnpm.
```

Then turn that into the patch:

```bash
tai patch regenerate use-pnpm
```

```
Regenerated 'use-pnpm' with 1 edit(s): ~/tai-config/patches/upstream-guide/use-pnpm.yaml
Review the result, then run:  tai patch bless upstream-guide:use-pnpm
```

Tai derives the `find`/`replace` blocks from your edit, widening context until each `find` matches exactly once, and verifies the generated edits reproduce your file byte-for-byte. Read the result:

```yaml
version: 1
file: GUIDE.md
description: we standardized on pnpm
edits:
  - find: |
      Always install with yarn.
    replace: |
      Always install with pnpm.
```

Bless the new upstream and install:

```bash
tai patch bless upstream-guide:use-pnpm
tai install my-profile
```

## Patching several files

One patch file targets one source file. To patch several, write several patch files in the same directory - they can be organized in subdirectories, and the name is the path within the patch directory:

```
patches/shared-skills/
  rad/use-pnpm.yaml       -> patch name "rad/use-pnpm"
  rad/drop-telemetry.yaml -> patch name "rad/drop-telemetry"
  go/style.yaml           -> patch name "go/style"
```

Several patches can also target the same file. They apply in name order, chaining - a later patch can anchor on text an earlier one introduced.

## When to exclude instead

If you do not want the file at all, do not patch it to nothing:

```yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    exclude:
      - "skills/legacy-*"
```

An exclusion has no coupling to upstream, which is the right relationship when you have no opinion about the content beyond not wanting it. A patch is for when you want the file and want to know when it changes.

## See also

- [Patches](/tai/concepts/patches/) - the concept and its guarantees
- [Sources](/tai/concepts/sources/) - source types, including url sources
- [CLI reference](/tai/reference/cli/#tai-patch) - every `tai patch` subcommand
