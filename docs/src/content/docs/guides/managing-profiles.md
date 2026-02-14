---
title: Switching & Managing Profiles
description: How to install, switch, update, and manage profiles across targets
---

Profiles are what you actually install - they bind bundles to a target and produce the final set of assets. This guide covers the day-to-day workflows for managing profiles: checking status, switching between them, updating after edits, and cleaning up.

## Checking current status

To see what's currently installed:

```bash
tai status
```

This shows all targets with tai-managed installations, the profile installed for each, and the number of managed assets. To check a specific target:

```bash
tai status claude
```

The output shows:
- Which profile is installed
- When it was installed
- How many assets are managed

## Installing a profile

Install a profile with:

```bash
tai install personal
```

Tai resolves the profile's bundles, processes templates, transforms frontmatter, and writes the results to the target directory. The target is determined by the profile's `target` field.

### Using default_profile

If your `tai.yaml` has a `default_profile` set:

```yaml
# tai.yaml
version: 1
default_profile: personal
```

You can install it without specifying the name:

```bash
tai install
```

This is convenient for your primary profile on each machine.

## Previewing changes with dry-run

Before installing, preview what tai would do:

```bash
tai install personal --dry-run
```

The dry-run output shows:
- Files that would be added (new)
- Files that would be updated (changed since last install)
- Files that would be removed (no longer in the profile)

No files are created, modified, or deleted during a dry-run.

### Viewing diffs for conflicts

If tai detects conflicting files (files that exist but aren't managed by tai, or managed files that were modified externally), use `--diff` to see the differences:

```bash
tai install personal --diff
```

This shows unified diffs for each conflicting file, helping you decide whether to force-overwrite or skip.

## Switching profiles

To switch from one profile to another, install the new profile:

```bash
tai install work
```

Tai handles the transition automatically:

1. Loads the manifest from the previous installation
2. Resolves the new profile's assets
3. Removes files that are in the old profile but not the new one
4. Adds files that are in the new profile but not the old one
5. Updates files that exist in both but have changed
6. Writes a new manifest

You don't need to uninstall first. A single `tai install` command handles the switch.

```bash
# Currently using personal profile
tai status
# Profile: personal

tai install work
# Removes personal-only assets, adds work assets, keeps shared assets

tai status
# Profile: work
```

## Re-installing after edits

When you edit asset files in your config repo, re-install to apply the changes:

```bash
tai install personal
```

Tai compares checksums from the manifest against the newly processed assets. Only changed files are updated - unchanged files are skipped.

### The reinstall command

If you just want to re-run the installation for whatever profile is already installed (without having to remember or type its name), use `tai reinstall`:

```bash
tai reinstall
```

This reads the manifest to find which profile was last installed, then re-installs it. It accepts the same flags as `tai install`:

```bash
tai reinstall --dry-run        # Preview what would change
tai reinstall --force          # Overwrite conflicting files
tai reinstall --skip-conflicts # Leave conflicting files as-is
tai reinstall --var key=val    # Pass template variables
```

You can also target a specific target if you have multiple installed:

```bash
tai reinstall claude
```

Without a target argument, `tai reinstall` re-installs all currently installed targets.

## Locating assets for editing

To find where a specific asset lives in your config repo:

```bash
tai locate personas/senior-swe.md
# /Users/you/my-tai-config/assets/personas/senior-swe.md
```

This prints the full filesystem path, so you can open it directly in your editor:

```bash
vim $(tai locate personas/senior-swe.md)
```

After editing, re-install to apply:

```bash
tai install personal
```

## Handling conflicts

Conflicts arise when tai tries to install a file that already exists and isn't managed by tai, or when a managed file was modified outside of tai. Tai offers three strategies:

### Force overwrite

Overwrite all conflicting files with tai's version:

```bash
tai install personal --force
```

### Skip conflicts

Leave conflicting files as-is and install everything else:

```bash
tai install personal --skip-conflicts
```

### Preview first

Use `--dry-run` and `--diff` together to understand conflicts before deciding:

```bash
tai install personal --dry-run --diff
```

Then install with `--force` or `--skip-conflicts` as needed.

## Uninstalling

To remove all tai-managed files for a target:

```bash
tai uninstall claude
```

This:
1. Reads the manifest to find all tai-managed files
2. Removes each file tai installed
3. Restores any backed-up files (from `{{ existing }}` templating)
4. Removes the manifest

Files that tai doesn't manage are left untouched. The target directory itself is not deleted.

## Template variables across installs

If your assets use template variables (via `--var`), you need to pass them on every install and reinstall:

```bash
tai install work --var project_name=MyApp,language=Go
```

Variables are not stored in the manifest - they are resolved at install time. If you reinstall without passing the same variables, those template expressions will resolve to empty strings.

For variables that rarely change, consider using [adapter-provided variables](/tai/guides/target-aware-assets/) instead, which are automatic.

## Workflow summary

| Task | Command |
|------|---------|
| Check what's installed | `tai status` |
| Preview installation | `tai install <profile> --dry-run` |
| Install a profile | `tai install <profile>` |
| Switch profiles | `tai install <other-profile>` |
| Re-install after edits | `tai install <profile>` or `tai reinstall` |
| Re-install all targets | `tai reinstall` |
| Find an asset to edit | `tai locate <path>` |
| Remove tai from a target | `tai uninstall <target>` |
| View conflict diffs | `tai install <profile> --diff` |
| Force past conflicts | `tai install <profile> --force` |
| Skip conflicts | `tai install <profile> --skip-conflicts` |

## Next steps

- [Composing Bundles](/tai/guides/composing-bundles/) - organize assets into bundles
- [Working with Multiple Sources](/tai/guides/multiple-sources/) - combine assets from multiple locations
- [CLI Reference](/tai/reference/cli/) - complete command reference
- [Installation Behavior](/tai/reference/installation-behavior/) - detailed install/uninstall mechanics
