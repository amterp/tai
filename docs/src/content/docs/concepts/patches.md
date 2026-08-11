---
title: Patches
description: Keep local edits on assets you import from sources you do not control, and find out when upstream moves
---

A patch is a local edit tai reapplies to content from a [source](/tai/concepts/sources/) you do not own. You declare it once; tai reapplies it on every install, and tells you when upstream changes underneath it.

The alternative - forking the content into your own repo - works until upstream improves and you have to notice, re-merge, and remember why you diverged. A patch keeps the delta small and explicit, and makes upstream drift something tai reports rather than something you discover months later.

## Declaring a patch

A source declares one directory of patch files, relative to your config repo:

```yaml
# tai.yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    patches: patches/shared-skills/
```

Each file in that directory targets one file in the source:

```yaml
# patches/shared-skills/use-pnpm.yaml
version: 1
file: skills/rad/SKILL.md
description: we standardized on pnpm
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
```

Any source type can declare `patches:` - local directories, tai repos, command sources, and url sources alike.

## Each edit must match exactly once

`find` is literal text, not a regex or a pattern. It must occur exactly once in the target file. Zero matches or two matches aborts the install before anything is written.

This is the property that makes patches age well. A patch that silently stopped applying would reintroduce content you explicitly rejected, and you would not find out. A patch that silently applied twice would be worse. So tai refuses both:

```
Error: patch 'use-pnpm' could not be applied - install aborted, nothing was changed:

  patch:  /home/you/tai-config/patches/shared-skills/use-pnpm.yaml
  file:   shared-skills:skills/rad/SKILL.md
  edit #1: find text not found - upstream has likely changed since this patch was written
    | Always install with npm.
```

An empty `replace` deletes the matched text. Edits within one patch apply in order, each seeing the previous one's result.

## The pristine source is never written to

Tai does not edit your source. It copies the source into a **view** under `~/.cache/tai/views/`, applies the patches there, and installs from the view.

```
~/src/shared-skills/          <- pristine, never modified
~/.cache/tai/views/<key>/shared-skills/   <- patched copy, rebuilt every install
```

Two things depend on this. Drift detection is only meaningful if the pristine copy stays an honest mirror of upstream, and `tai locate` keeps pointing at a checkout you can actually edit rather than at a cache directory the next install replaces.

The view is rebuilt from pristine on every install. There is no staleness check to get wrong.

## Blessing and drift

A patch that still applies is not the same as a patch that is still right. Upstream may have changed something the patch has an opinion about, somewhere the patch does not touch.

`tai patch bless` records the current upstream file as reviewed, writing a `.blessed` snapshot next to the patch:

```
patches/shared-skills/
  use-pnpm.yaml
  use-pnpm.blessed    <- byte-for-byte copy of upstream at review time
```

Commit the `.blessed` file. It lives in your config repo so drift detection travels with it through git.

From then on, tai compares upstream against the snapshot on every install:

| Situation | What tai does |
|-----------|---------------|
| Applies; upstream matches the snapshot | Silent |
| Applies; upstream differs from the snapshot | Warns, installs the patched content |
| Applies; no snapshot yet | One-line notice suggesting `tai patch bless` |
| Does not apply | **Install fails**; nothing is written |

The warning names the patch and points at the remedy:

```
Warning: upstream file for patch 'use-pnpm' changed since it was blessed
  file: shared-skills:skills/rad/SKILL.md
  The patch still applies cleanly; installing the patched content.
  Review what changed:  tai patch diff shared-skills:use-pnpm
  Accept it:            tai patch bless shared-skills:use-pnpm
```

`tai patch diff` shows what moved upstream since you blessed it. If the change is fine, bless again and the warning clears.

Tai refuses to bless a patch that does not apply, targets a file the source does not carry, or belongs to a source that has not been fetched yet. Recording a snapshot in any of those states would bake in a broken patch and permanently silence the warning that would prompt a fix.

## Authoring and repairing

Hand-writing `find`/`replace` blocks is the weak point of the format, so tai does not ask you to. `tai patch workspace` gives you an editable copy of the source with the existing patches applied:

```bash
tai patch workspace shared-skills
# Workspace ready: ~/.cache/tai/workspaces/<key>/shared-skills
```

Edit the files there as you would any checkout, then turn your edits back into a patch:

```bash
tai patch regenerate use-pnpm
tai patch bless shared-skills:use-pnpm
```

`regenerate` derives the `find`/`replace` blocks from the diff, widening context until each `find` is unambiguous, and verifies the result reproduces your edits byte-for-byte before writing.

This is also the repair loop when upstream breaks a patch. The failure message walks through it in order, and when a patch fails to apply, tai leaves the half-built workspace in place so it is already waiting for you.

## Excluding instead of patching

`exclude:` is the neighboring tool, and it expresses a different relationship to upstream:

```yaml
sources:
  - name: shared-skills
    path: ~/src/shared-skills
    exclude:
      - "skills/legacy-*"
```

A patch says "tell me when upstream touches these lines". An exclusion says "upstream can do what it likes with this, I am not taking it". Excluded files are not installed, not copied into the view, and not resolvable - referencing one in a bundle is an error that names the exclusion rather than reporting "not found".

## What patches are not

- **Not a regex or fuzzy matcher.** `find` is literal text. This is what lets tai promise the patch either applied exactly as written or failed loudly.
- **Not a file creator.** A patch edits existing content. Adding a new file is what your own `assets/` directory is for.
- **Not version pinning.** A patch tells you when the lines it anchors on change; it does not freeze the source at a revision.

## See also

- [Patching imported assets](/tai/guides/patching-imported-assets/) - the end-to-end workflow
- [Sources](/tai/concepts/sources/) - where patched content comes from
- [CLI reference](/tai/reference/cli/#tai-patch) - every `tai patch` subcommand
