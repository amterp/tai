package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amterp/ra"
	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/cmdexec"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/output"
	"github.com/amterp/tai/internal/profile"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/state"
	"github.com/amterp/tai/internal/template"
)

const version = "0.2.0"

// out is the global output instance, initialized after flag parsing.
var out output.Output

// repoOverride is the config repo named by --config or TAI_CONFIG for this
// invocation, or nil. Resolved after flag parsing.
var repoOverride *state.RepoOverride

func main() {
	root := ra.NewCmd("tai")
	root.SetDescription("Manage AI agent configurations across targets (v" + version + ")")
	root.SetAutoHelpOnNoArgs(true)

	// Global flags, so they can follow a subcommand. Ra copies globals onto
	// each subcommand as it is registered, so these must precede every
	// RegisterCmd.
	noColor, _ := ra.NewBool("no-color").
		SetDefault(false).
		SetUsage("Disable colored output").
		Register(root, ra.WithGlobal(true))
	configOverride, _ := ra.NewString("config").
		SetUsage("Config repo to read for this command instead of the active one (read-only commands; also TAI_CONFIG)").
		Register(root, ra.WithGlobal(true))

	// Config subcommands
	configCmd := ra.NewCmd("config")
	configCmd.SetDescription("Manage tai configuration")

	configSetCmd := ra.NewCmd("set")
	configSetCmd.SetDescription("Set the active config repository path")
	configPath, _ := ra.NewString("path").
		SetUsage("Path to the tai config repository").
		Register(configSetCmd)
	configSetUsed, _ := configCmd.RegisterCmd(configSetCmd)

	configShowCmd := ra.NewCmd("show")
	configShowCmd.SetDescription("Show current configuration settings")
	configShowUsed, _ := configCmd.RegisterCmd(configShowCmd)

	configUsed, _ := root.RegisterCmd(configCmd)

	// List subcommands
	listCmd := ra.NewCmd("list")
	listCmd.SetDescription("List available resources")

	listSourcesCmd := ra.NewCmd("sources")
	listSourcesCmd.SetDescription("List configured sources")
	listSourcesUsed, _ := listCmd.RegisterCmd(listSourcesCmd)

	listBundlesCmd := ra.NewCmd("bundles")
	listBundlesCmd.SetDescription("List available bundles")
	listBundlesUsed, _ := listCmd.RegisterCmd(listBundlesCmd)

	listProfilesCmd := ra.NewCmd("profiles")
	listProfilesCmd.SetDescription("List available profiles")
	listProfilesUsed, _ := listCmd.RegisterCmd(listProfilesCmd)

	listAssetsCmd := ra.NewCmd("assets")
	listAssetsCmd.SetDescription("List available assets")
	listAssetsSource, _ := ra.NewString("source").
		SetShort("s").
		SetOptional(true).
		SetUsage("Filter assets by source name").
		Register(listAssetsCmd)
	listAssetsUsed, _ := listCmd.RegisterCmd(listAssetsCmd)

	listUsed, _ := root.RegisterCmd(listCmd)

	// Show subcommands
	showCmd := ra.NewCmd("show")
	showCmd.SetDescription("Show details of a resource")

	showProfileCmd := ra.NewCmd("profile")
	showProfileCmd.SetDescription("Show resolved assets for a profile")
	showProfileName, _ := ra.NewString("name").
		SetUsage("Profile name to show").
		Register(showProfileCmd)
	showProfileUsed, _ := showCmd.RegisterCmd(showProfileCmd)

	showBundleCmd := ra.NewCmd("bundle")
	showBundleCmd.SetDescription("Show assets in a bundle")
	showBundleName, _ := ra.NewString("name").
		SetUsage("Bundle name to show").
		Register(showBundleCmd)
	showBundleUsed, _ := showCmd.RegisterCmd(showBundleCmd)

	showUsed, _ := root.RegisterCmd(showCmd)

	// Patch subcommands
	patchCmd := ra.NewCmd("patch")
	patchCmd.SetDescription("Inspect and bless patches applied to imported sources")

	patchListCmd := ra.NewCmd("list")
	patchListCmd.SetDescription("List patches and whether they still apply")
	patchListUsed, _ := patchCmd.RegisterCmd(patchListCmd)

	patchDiffCmd := ra.NewCmd("diff")
	patchDiffCmd.SetDescription("Show what changed upstream since a patch was blessed")
	patchDiffName, _ := ra.NewString("name").
		SetUsage("Patch name (or source:name)").
		Register(patchDiffCmd)
	patchDiffUsed, _ := patchCmd.RegisterCmd(patchDiffCmd)

	patchBlessCmd := ra.NewCmd("bless")
	patchBlessCmd.SetDescription("Record current upstream content as reviewed")
	patchBlessName, _ := ra.NewString("name").
		SetOptional(true).
		SetUsage("Patch name (or source:name)").
		Register(patchBlessCmd)
	patchBlessAll, _ := ra.NewBool("all").
		SetDefault(false).
		SetUsage("Bless every patch that applies cleanly").
		Register(patchBlessCmd)
	patchBlessUsed, _ := patchCmd.RegisterCmd(patchBlessCmd)

	patchWorkspaceCmd := ra.NewCmd("workspace")
	patchWorkspaceCmd.SetDescription("Open an editable copy of a source to author or repair patches in")
	patchWorkspaceSource, _ := ra.NewString("source").
		SetUsage("Source name").
		Register(patchWorkspaceCmd)
	patchWorkspaceUsed, _ := patchCmd.RegisterCmd(patchWorkspaceCmd)

	patchRegenerateCmd := ra.NewCmd("regenerate")
	patchRegenerateCmd.SetDescription("Rewrite a patch from your edits in its workspace")
	patchRegenerateName, _ := ra.NewString("name").
		SetUsage("Patch name (or source:name)").
		Register(patchRegenerateCmd)
	patchRegenerateUsed, _ := patchCmd.RegisterCmd(patchRegenerateCmd)

	patchUsed, _ := root.RegisterCmd(patchCmd)

	// Install command
	installCmd := ra.NewCmd("install")
	installCmd.SetDescription("Install a profile to its target")
	installProfile, _ := ra.NewString("profile").
		SetUsage("Profile to install").
		Register(installCmd)
	installDryRun, _ := ra.NewBool("dry-run").
		SetShort("n").
		SetDefault(false).
		SetUsage("Preview changes without applying").
		Register(installCmd)
	installForce, _ := ra.NewBool("force").
		SetShort("f").
		SetDefault(false).
		SetUsage("Overwrite conflicting files").
		Register(installCmd)
	installSkipConflicts, _ := ra.NewBool("skip-conflicts").
		SetDefault(false).
		SetUsage("Leave conflicting files as-is").
		Register(installCmd)
	installVars, _ := ra.NewString("var").
		SetShort("v").
		SetOptional(true).
		SetUsage("Template variables (format: key1=val1,key2=val2)").
		Register(installCmd)
	installDiff, _ := ra.NewBool("diff").
		SetDefault(false).
		SetUsage("Show unified diffs for conflicting files").
		Register(installCmd)
	installAllowRewrite, _ := ra.NewBool("allow-config-rewrite").
		SetDefault(false).
		SetUsage("Allow rewriting target config files that need consent, without asking").
		Register(installCmd)
	installUsed, _ := root.RegisterCmd(installCmd)

	// Reinstall command
	reinstallCmd := ra.NewCmd("reinstall")
	reinstallCmd.SetDescription("Re-run installation for already-installed profiles")
	reinstallTarget, _ := ra.NewString("target").
		SetOptional(true).
		SetUsage("Target to reinstall (default: all installed targets)").
		Register(reinstallCmd)
	reinstallDryRun, _ := ra.NewBool("dry-run").
		SetShort("n").
		SetDefault(false).
		SetUsage("Preview changes without applying").
		Register(reinstallCmd)
	reinstallForce, _ := ra.NewBool("force").
		SetShort("f").
		SetDefault(false).
		SetUsage("Overwrite conflicting files").
		Register(reinstallCmd)
	reinstallSkipConflicts, _ := ra.NewBool("skip-conflicts").
		SetDefault(false).
		SetUsage("Leave conflicting files as-is").
		Register(reinstallCmd)
	reinstallVars, _ := ra.NewString("var").
		SetShort("v").
		SetOptional(true).
		SetUsage("Template variables (format: key1=val1,key2=val2)").
		Register(reinstallCmd)
	reinstallDiff, _ := ra.NewBool("diff").
		SetDefault(false).
		SetUsage("Show unified diffs for conflicting files").
		Register(reinstallCmd)
	reinstallAllowRewrite, _ := ra.NewBool("allow-config-rewrite").
		SetDefault(false).
		SetUsage("Allow rewriting target config files that need consent, without asking").
		Register(reinstallCmd)
	reinstallUsed, _ := root.RegisterCmd(reinstallCmd)

	// Refresh command
	refreshCmd := ra.NewCmd("refresh")
	refreshCmd.SetDescription("Fetch remote sources into the cache")
	refreshSource, _ := ra.NewString("source").
		SetOptional(true).
		SetUsage("Source to refresh (default: all fetchable sources)").
		Register(refreshCmd)
	refreshUsed, _ := root.RegisterCmd(refreshCmd)

	// Uninstall command
	uninstallCmd := ra.NewCmd("uninstall")
	uninstallCmd.SetDescription("Remove tai-managed files for a target")
	uninstallTarget, _ := ra.NewString("target").
		SetUsage("Target to uninstall (e.g., claude, codex)").
		Register(uninstallCmd)
	uninstallUsed, _ := root.RegisterCmd(uninstallCmd)

	// Status command
	statusCmd := ra.NewCmd("status")
	statusCmd.SetDescription("Show what's currently installed")
	statusTarget, _ := ra.NewString("target").
		SetOptional(true).
		SetUsage("Target to show status for").
		Register(statusCmd)
	statusUsed, _ := root.RegisterCmd(statusCmd)

	// Locate command
	locateCmd := ra.NewCmd("locate")
	locateCmd.SetDescription("Print full filesystem path to an asset")
	locateAsset, _ := ra.NewString("asset-path").
		SetUsage("Asset path (e.g., personas/senior-swe.md)").
		Register(locateCmd)
	locateUsed, _ := root.RegisterCmd(locateCmd)

	// Run command
	runCmd := ra.NewCmd("run")
	runCmd.SetDescription("Run a target with tai config applied")
	// Positional-only so that splitRunArgs can take the first non-flag
	// token after "run" as the target.
	runTarget, _ := ra.NewString("target").
		SetPositionalOnly(true).
		SetUsage("Target to run (e.g., claude, codex)").
		Register(runCmd)
	// Registered for the usage text only: splitRunArgs removes the target's
	// arguments before Ra parses, so this is never set.
	_, _ = ra.NewStringSlice("args").
		SetPositionalOnly(true).
		SetVariadic(true).
		SetOptional(true).
		SetUsage("Arguments passed to the target as written, flags included").
		Register(runCmd)
	runUsed, _ := root.RegisterCmd(runCmd)

	// Parse
	taiArgs, targetArgs := splitRunArgs(os.Args[1:])
	root.ParseOrExit(taiArgs)

	// Initialize output after flag parsing
	out = output.New(output.Options{
		Writer:    os.Stdout,
		ErrWriter: os.Stderr,
		NoColor:   *noColor,
	})

	var err error
	repoOverride, err = state.ResolveOverride(*configOverride, os.Getenv("TAI_CONFIG"))
	if err != nil {
		fatal("%v", err)
	}

	// Route to appropriate handler
	switch {
	case *configUsed:
		switch {
		case *configSetUsed:
			runConfigSet(*configPath)
		case *configShowUsed:
			runConfigShow()
		default:
			out.Error("Usage: tai config <set|show>")
			os.Exit(1)
		}
	case *listUsed:
		switch {
		case *listSourcesUsed:
			runListSources()
		case *listBundlesUsed:
			runListBundles()
		case *listProfilesUsed:
			runListProfiles()
		case *listAssetsUsed:
			runListAssets(*listAssetsSource)
		default:
			out.Error("Usage: tai list <sources|bundles|profiles|assets>")
			os.Exit(1)
		}
	case *showUsed:
		switch {
		case *showProfileUsed:
			runShowProfile(*showProfileName)
		case *showBundleUsed:
			runShowBundle(*showBundleName)
		default:
			out.Error("Usage: tai show <profile|bundle> <name>")
			os.Exit(1)
		}
	case *patchUsed:
		switch {
		case *patchListUsed:
			runPatchList()
		case *patchDiffUsed:
			runPatchDiff(*patchDiffName)
		case *patchBlessUsed:
			runPatchBless(*patchBlessName, *patchBlessAll)
		case *patchWorkspaceUsed:
			runPatchWorkspace(*patchWorkspaceSource)
		case *patchRegenerateUsed:
			runPatchRegenerate(*patchRegenerateName)
		default:
			out.Error("Usage: tai patch <list|diff|bless|workspace|regenerate>")
			os.Exit(1)
		}
	case *installUsed:
		runInstall(*installProfile, installFlags{
			dryRun:             *installDryRun,
			force:              *installForce,
			skipConflicts:      *installSkipConflicts,
			vars:               *installVars,
			showDiff:           *installDiff,
			allowConfigRewrite: *installAllowRewrite,
		})
	case *reinstallUsed:
		runReinstall(*reinstallTarget, installFlags{
			dryRun:             *reinstallDryRun,
			force:              *reinstallForce,
			skipConflicts:      *reinstallSkipConflicts,
			vars:               *reinstallVars,
			showDiff:           *reinstallDiff,
			allowConfigRewrite: *reinstallAllowRewrite,
		})
	case *refreshUsed:
		runRefresh(*refreshSource)
	case *uninstallUsed:
		runUninstall(*uninstallTarget)
	case *statusUsed:
		runStatus(*statusTarget)
	case *locateUsed:
		runLocate(*locateAsset)
	case *runUsed:
		runRun(*runTarget, targetArgs)
	default:
		out.Raw(root.GenerateUsage(false))
	}
}

func fatal(format string, args ...any) {
	out.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// parseVariables parses a comma-separated list of key=value pairs.
// Returns an error if any pair is malformed (missing '=' sign).
// Note: values containing commas are not supported with comma-separated
// syntax. Use repeated --var flags for values that need commas.
func parseVariables(varsStr string) (template.Variables, error) {
	vars := template.Variables{}
	if varsStr == "" {
		return vars, nil
	}
	for _, pair := range strings.Split(varsStr, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed --var entry %q: expected key=value format", pair)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("malformed --var entry %q: key cannot be empty", pair)
		}
		vars[key] = strings.TrimSpace(parts[1])
	}
	return vars, nil
}

func loadState() *state.State {
	s, err := state.Load(repoOverride)
	if err != nil {
		fatal("loading state: %v", err)
	}
	s.SetVersion(version)
	return s
}

// loadGlobalState reads tai's global config without loading the config repo.
// Commands that never read the repo use it, so a repo whose sources are
// missing on this machine cannot lock them out - least of all 'config set',
// the command that replaces it.
func loadGlobalState() *state.State {
	paths, err := state.DefaultPaths()
	if err != nil {
		fatal("%v", err)
	}
	s, err := state.New(paths, repoOverride)
	if err != nil {
		fatal("loading state: %v", err)
	}
	s.SetVersion(version)
	return s
}

// Why a command refuses to run while a config repo override is in effect.
const (
	overrideWritesManifests    = "it writes install manifests and the target's files, and every config repo shares those manifests, so acting from a second repo would mix the two repos' records of what tai owns"
	overrideWritesGlobalConfig = "it changes the active config, which an override exists to leave alone"
	overrideWritesRepo         = "it writes into the config repo, and an override is read-only"
)

// refuseUnderOverride exits when a config repo override is in effect. Called
// before any loading, so the refusal is what the user sees even when the
// override repo itself is broken.
func refuseUnderOverride(command, reason string) {
	if repoOverride != nil {
		fatal("%s", overrideRefusal(repoOverride, command, reason))
	}
}

func overrideRefusal(o *state.RepoOverride, command, reason string) string {
	drop := "drop --config"
	if o.Origin == "TAI_CONFIG" {
		drop = "unset TAI_CONFIG"
	}
	msg := fmt.Sprintf("'tai %s' cannot use the config repo override %s (from %s): %s. To run it against the active config, %s",
		command, o.Path, o.Origin, reason, drop)
	if command != "config set" {
		msg += fmt.Sprintf("; to run it against %s, make that repo active with 'tai config set %s'", o.Path, o.Path)
	}
	return msg + "."
}

func requireActiveConfig(s *state.State) {
	if err := s.RequireActiveConfig(); err != nil {
		fatal("%v", err)
	}
}

// --- Config commands ---

func runConfigSet(path string) {
	refuseUnderOverride("config set", overrideWritesGlobalConfig)
	s := loadGlobalState()
	if err := s.SetActiveConfig(path); err != nil {
		fatal("setting active config: %v", err)
	}
	out.Success("Active config set to: " + s.GlobalConfig.ActiveConfig)
}

func runConfigShow() {
	s := loadGlobalState()
	pairs := []output.KeyValuePair{
		{Key: "Config directory", Value: s.Paths.ConfigDir},
		{Key: "Cache directory", Value: s.Paths.CacheDir},
		{Key: "Active config", Value: valueOrNone(s.GlobalConfig.ActiveConfig)},
	}
	if o := s.Override; o != nil {
		pairs = append(pairs, output.KeyValuePair{Key: "Config override", Value: o.Path + " (from " + o.Origin + ")"})
	}
	out.KeyValueBlock(pairs)
}

func valueOrNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// --- List commands ---

func runListSources() {
	s := loadState()
	requireActiveConfig(s)

	items := make([]output.ListItem, 0, len(s.TaiConfig.Sources))
	for _, src := range s.TaiConfig.Sources {
		typeStr := src.Type
		if typeStr == "" {
			typeStr = "local"
		}
		extra := src.Path
		if src.NeedsFetch() {
			extra = "not fetched (run 'tai refresh')"
			if loaded, ok := s.Sources[src.Name]; ok && loaded.Materialized {
				extra = "cached"
			}
		}
		items = append(items, output.ListItem{
			Name:        src.Name,
			Description: typeStr,
			Extra:       extra,
		})
	}
	out.List("Sources:", items)
}

func runListBundles() {
	s := loadState()
	requireActiveConfig(s)

	printed := false
	if len(s.Bundles) > 0 {
		out.List("Bundles:", bundleListItems(s.Bundles, ""))
		printed = true
	}

	// Bundles offered by other sources are importable as "source:name", so
	// list them too - otherwise you have to already know a name to use it.
	activeBundles := filepath.Clean(filepath.Join(s.ConfigRepo(), source.BundlesDirName))
	for _, name := range sortedSourceNames(s.Sources) {
		src := s.Sources[name]

		// A source pointed at the active repo - the implicit "local" one, most
		// often - offers the very bundles listed above. Printing that directory
		// twice under two names reads as two sets of bundles.
		if filepath.Clean(src.BundlesPath()) == activeBundles {
			continue
		}

		// A source awaiting its first fetch has nothing to list yet. That is an
		// ordinary state for a url or command source, so omit it the way
		// `tai list assets` does rather than warning about it every time.
		if src.RequireMaterialized() != nil {
			continue
		}

		bundles, err := src.LoadBundles()
		if err != nil {
			// One unreadable source must not take out the whole listing.
			out.Warning(fmt.Sprintf("Could not read bundles from source %q: %v. "+
				"Bundles it offers cannot be imported until this is fixed; check the source's bundles/ dir for malformed YAML.", name, err))
			continue
		}
		if len(bundles) == 0 {
			continue
		}
		out.List(name+":", bundleListItems(bundles, name+":"))
		printed = true
	}

	if !printed {
		out.Info("No bundles defined.")
	}
}

// bundleListItems renders a bundle map for listing, prefixing each name with
// the reference form it is imported by.
func bundleListItems(bundles map[string]*config.Bundle, prefix string) []output.ListItem {
	names := make([]string, 0, len(bundles))
	for name := range bundles {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]output.ListItem, 0, len(names))
	for _, name := range names {
		desc := bundles[name].Description
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, output.ListItem{
			Name:        prefix + name,
			Description: desc,
		})
	}
	return items
}

func sortedSourceNames(sources map[string]*source.LoadedSource) []string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runListProfiles() {
	s := loadState()
	requireActiveConfig(s)

	if len(s.Profiles) == 0 {
		out.Info("No profiles defined.")
		return
	}

	names := make([]string, 0, len(s.Profiles))
	for name := range s.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]output.ListItem, 0, len(names))
	for _, name := range names {
		p := s.Profiles[name]
		desc := p.Description
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, output.ListItem{
			Name:        name,
			Description: p.Target,
			Extra:       desc,
		})
	}
	out.List("Profiles:", items)
}

func runListAssets(sourceName string) {
	s := loadState()
	requireActiveConfig(s)

	if sourceName != "" {
		// Filter by source
		src, ok := s.Sources[sourceName]
		if !ok {
			fatal("source not found: %s", sourceName)
		}
		if err := src.RequireMaterialized(); err != nil {
			fatal("%v", err)
		}
		assets := src.ListAllAssets()
		sort.Strings(assets)

		items := make([]output.ListItem, 0, len(assets))
		for _, a := range assets {
			items = append(items, output.ListItem{Name: a})
		}
		out.List("Assets from source '"+sourceName+"':", items)
	} else {
		// All sources
		for name, src := range s.Sources {
			assets := src.ListAllAssets()
			if len(assets) == 0 {
				continue
			}
			sort.Strings(assets)

			items := make([]output.ListItem, 0, len(assets))
			for _, a := range assets {
				items = append(items, output.ListItem{Name: a})
			}
			out.List(name+":", items)
		}
	}
}

// --- Show commands ---

func runShowProfile(name string) {
	s := loadState()
	requireActiveConfig(s)

	resolved, err := s.ResolveProfile(name)
	if err != nil {
		fatal("resolving profile: %v", err)
	}

	out.KeyValueBlock([]output.KeyValuePair{
		{Key: "Profile", Value: resolved.Name},
		{Key: "Target", Value: resolved.Target},
		{Key: "Assets", Value: strconv.Itoa(len(resolved.Assets))},
	})

	out.Blank()
	items := make([]output.ListItem, 0, len(resolved.Assets))
	for _, asset := range resolved.Assets {
		items = append(items, output.ListItem{
			Name: asset.Type + "/" + asset.Name + " (" + asset.SourceName + ")",
		})
	}
	for _, item := range items {
		out.Raw("  " + item.Name)
	}
	showPostInstallSteps(resolved.PostInstall)
}

func runShowBundle(name string) {
	s := loadState()
	requireActiveConfig(s)

	resolved, err := s.BundleResolver.Resolve(name)
	if err != nil {
		fatal("resolving bundle: %v", err)
	}

	pairs := []output.KeyValuePair{
		{Key: "Bundle", Value: resolved.Name},
	}
	// An imported bundle is not in the active repo's map, so this lookup only
	// finds local ones. The resolver already proved the bundle exists.
	if b, ok := s.Bundles[name]; ok && b.Description != "" {
		pairs = append(pairs, output.KeyValuePair{Key: "Description", Value: b.Description})
	}
	pairs = append(pairs, output.KeyValuePair{Key: "Assets", Value: strconv.Itoa(len(resolved.Assets))})
	out.KeyValueBlock(pairs)

	out.Blank()
	for _, asset := range resolved.Assets {
		out.Raw("  " + asset.Type + "/" + asset.Name + " (" + asset.SourceName + ")")
	}

	steps := make([]profile.PostInstallStep, len(resolved.PostInstall))
	for i, step := range resolved.PostInstall {
		steps[i] = profile.FromBundleStep(step)
	}
	showPostInstallSteps(steps)
}

// showPostInstallSteps lists the commands an install would run afterwards. An
// imported bundle can add one, so this is where a user sees it before it runs.
func showPostInstallSteps(steps []profile.PostInstallStep) {
	if len(steps) == 0 {
		return
	}
	out.Blank()
	out.Raw("Post-install steps:")
	for _, step := range steps {
		out.Raw("  " + postInstallStepName(step) + ": " + strings.Join(step.Argv, " "))
	}
}

// --- Install commands ---

// installFlags carries the install/reinstall command-line options. They travel
// together through several layers, and a growing positional parameter list was
// getting easy to misorder.
type installFlags struct {
	dryRun             bool
	force              bool
	skipConflicts      bool
	vars               string
	showDiff           bool
	allowConfigRewrite bool
}

func runInstall(profileName string, flags installFlags) {
	refuseUnderOverride("install", overrideWritesManifests)
	s := loadState()
	requireActiveConfig(s)
	refreshSourcesForInstall(s, flags.dryRun)
	buildPatchedViews(s)
	if err := doInstall(s, profileName, flags); err != nil {
		os.Exit(1)
	}
}

// buildPatchedViews materializes patched copies of any sources that declare
// patches, before anything resolves assets out of them.
//
// Runs in dry-run too. A preview that resolved unpatched content would be
// previewing something tai will never install, and it is also how --dry-run
// surfaces a broken patch without touching the target.
func buildPatchedViews(s *state.State) {
	warnAboutPatchDrift(s)
	if err := s.BuildPatchedViews(); err != nil {
		reportPatchFailure(err)
		os.Exit(1)
	}
}

// refreshSourcesForInstall fetches remote sources so installs always see
// fresh content. Dry-run skips the fetch (it's a side effect a preview
// shouldn't have) and resolves from whatever is cached.
func refreshSourcesForInstall(s *state.State, dryRun bool) {
	if !s.HasFetchableSources() {
		return
	}
	if dryRun {
		for _, src := range s.TaiConfig.Sources {
			if !src.NeedsFetch() {
				continue
			}
			if loaded, ok := s.Sources[src.Name]; ok && !loaded.Materialized {
				// Resolution will fail if the profile uses this source; warn
				// up front so the failure isn't a surprise.
				out.Info("Would refresh source '" + src.Name + "' (not fetched yet; run 'tai refresh' to enable previewing its assets)")
			} else {
				out.Info("Would refresh source '" + src.Name + "'")
			}
		}
		return
	}
	err := s.RefreshSources("", func(name string) {
		out.Info("Refreshing source '" + name + "'...")
	})
	if err != nil {
		fatal("refreshing sources: %v", err)
	}
}

func runRefresh(sourceName string) {
	s := loadState()
	requireActiveConfig(s)

	if !s.HasFetchableSources() {
		out.Info("No fetchable sources configured (only 'command' and 'url' sources are fetched).")
		return
	}

	refreshed := 0
	err := s.RefreshSources(sourceName, func(name string) {
		out.Info("Refreshing source '" + name + "'...")
		refreshed++
	})
	if err != nil {
		fatal("refreshing sources: %v", err)
	}
	out.Success(fmt.Sprintf("Refreshed %d source(s).", refreshed))
}

func runReinstall(target string, flags installFlags) {
	refuseUnderOverride("reinstall", overrideWritesManifests)
	dryRun := flags.dryRun
	s := loadState()
	requireActiveConfig(s)

	type reinstallTarget struct {
		target  string
		profile string
	}

	var toReinstall []reinstallTarget
	if target != "" {
		manifest, err := s.ManifestStore.Load(target)
		if err != nil {
			fatal("loading manifest for '%s': %v", target, err)
		}
		if manifest == nil {
			fatal("No profile installed for target '%s'.", target)
			return
		}
		toReinstall = []reinstallTarget{{target: target, profile: manifest.Profile}}
	} else {
		targets, err := s.ManifestStore.List()
		if err != nil {
			fatal("listing installed targets: %v", err)
		}
		if len(targets) == 0 {
			fatal("Nothing installed. Use 'tai install <profile>' first.")
		}
		for _, t := range targets {
			manifest, err := s.ManifestStore.Load(t)
			if err != nil {
				fatal("loading manifest for '%s': %v", t, err)
			}
			toReinstall = append(toReinstall, reinstallTarget{target: t, profile: manifest.Profile})
		}
	}

	refreshSourcesForInstall(s, dryRun)
	buildPatchedViews(s)

	failed := false
	for i, rt := range toReinstall {
		if i > 0 {
			out.Blank()
		}
		if err := doInstall(s, rt.profile, flags); err != nil {
			// Keep reinstalling the remaining targets; surface the failure
			// in the exit code at the end.
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// doInstall performs profile resolution and installation against pre-loaded
// state. Errors that abort the whole run (bad input, unresolvable profile)
// are fatal and exit the process; failures already reported to the user that
// should not stop other targets - conflicts, a failed post_install step -
// are returned instead, so runReinstall can continue its loop and aggregate
// them into the final exit code.
func doInstall(s *state.State, profileName string, flags installFlags) error {
	dryRun, force, skipConflicts, varsStr, showDiff := flags.dryRun, flags.force, flags.skipConflicts, flags.vars, flags.showDiff
	// The logger goes on before resolution: overrides, exclusions and skipped
	// post_install steps are all reported while the profile resolves.
	s.ProfileResolver.SetLogger(&outputLogger{out: out})
	s.Installer.SetLogger(&outputLogger{out: out})

	resolved, err := s.ResolveProfile(profileName)
	if err != nil {
		fatal("resolving profile: %v", err)
	}

	adap, err := s.GetAdapter(resolved.Target)
	if err != nil {
		fatal("%v", err)
	}

	cliVars, err := parseVariables(varsStr)
	if err != nil {
		fatal("%v", err)
	}
	mergedVars := template.MergeVariables(
		template.Variables(s.TaiConfig.Vars),
		template.Variables(resolved.Vars),
		cliVars,
	)
	if err := template.ValidateVarNames(mergedVars); err != nil {
		fatal("%v", err)
	}
	consent, err := newConsentResolver(out, s.Paths.ConfigDir, flags.allowConfigRewrite)
	if err != nil {
		fatal("%v", err)
	}

	opts := install.InstallOptions{
		DryRun:        dryRun,
		Force:         force,
		SkipConflicts: skipConflicts,
		Variables:     mergedVars,
		Scope:         resolved.Scope,
		Consent:       consent.Resolve,
	}

	assets := make([]*config.Asset, len(resolved.Assets))
	copy(assets, resolved.Assets)

	if dryRun {
		out.Info("Would install profile '" + profileName + "' for " + resolved.Target + "...")
	} else {
		out.Info("Installing profile '" + profileName + "' for " + resolved.Target + "...")
	}

	result, err := s.Installer.Install(adap, profileName, assets, opts)
	if err != nil {
		// Check if this is a conflicts error - display nicely instead of generic fatal
		if conflictsErr, ok := err.(*install.ConflictsError); ok {
			out.Error(fmt.Sprintf("%d conflict(s) detected:", len(conflictsErr.Conflicts)))

			// Build conflict items for display
			items := make([]output.ConflictItem, len(conflictsErr.Conflicts))
			for i, c := range conflictsErr.Conflicts {
				items[i] = output.ConflictItem{
					Path:   c.Path,
					Reason: c.Reason,
				}
				if showDiff {
					items[i].Diff = install.GenerateDiff(c)
				}
			}
			out.ConflictReport(&output.ConflictReportData{Conflicts: items})

			out.Blank()
			if showDiff {
				out.Info("Use --force to overwrite or --skip-conflicts to skip.")
			} else {
				out.Info("Use --force to overwrite, --skip-conflicts to skip, or --diff to inspect.")
			}
			// Already reported; return so multi-target reinstall can continue
			// with its remaining targets.
			return err
		}
		fatal("installation failed: %v", err)
	}

	manifestPath := ""
	if !dryRun {
		manifestPath = s.Paths.ManifestDir
	}
	out.InstallResult(&output.InstallResultData{
		Added:     result.Added,
		Restored:  result.Restored,
		Updated:   result.Updated,
		Unchanged: result.Unchanged,
		Removed:   result.Removed,
		Skipped:   result.Skipped,
		DryRun:    dryRun,
		Manifest:  manifestPath,
	})

	// Warnings were collected but never shown, so users had no way to learn
	// that an asset had been skipped as unsupported by their target.
	for _, w := range result.Warnings {
		out.Warning(w)
	}

	return runPostInstall(s, adap, resolved, result, dryRun)
}

// runPostInstall runs the profile's post_install steps, if any. They run only
// after a successful install, each one whether or not earlier ones failed, so
// one broken command cannot hold back the rest. A failure is reported (the
// install itself stands - files and manifest are already in place, and side
// effects can't be rolled back) and returned so callers exit nonzero.
//
// Steps may post-process managed files, so the manifest baseline is brought up
// to date after each one - see install.PostInstallTracker.
func runPostInstall(
	s *state.State,
	adap adapter.Adapter,
	resolved *profile.ResolvedProfile,
	result *install.InstallResult,
	dryRun bool,
) error {
	steps := resolved.PostInstall
	if len(steps) == 0 {
		return nil
	}
	if dryRun {
		for _, step := range steps {
			out.Info("Would run " + postInstallLabel(step))
		}
		return nil
	}

	env := map[string]string{
		"TAI_TARGET":       resolved.Target,
		"TAI_TARGET_DIR":   adap.TargetDir(),
		"TAI_PROFILE":      resolved.Name,
		"TAI_MANIFEST_DIR": s.Paths.ManifestDir,
		"TAI_VERSION":      s.Version,
	}
	// With several steps the closing summary says the install succeeded, so
	// the per-step line need not repeat it.
	failureSuffix := ""
	if len(steps) == 1 {
		failureSuffix = " (install succeeded)"
	}

	tracker := s.Installer.TrackPostInstall(adap, result)
	var failed []string
	for _, step := range steps {
		label := postInstallLabel(step)
		stepEnv := maps.Clone(env)
		stepEnv["TAI_SOURCE_DIR"] = step.SourceDir
		err := cmdexec.Run(cmdexec.Spec{Argv: step.Argv, Timeout: step.Timeout, Env: stepEnv, Dir: step.Dir})
		if err != nil {
			out.Error(fmt.Sprintf("%s failed%s: %v", label, failureSuffix, err))
			failed = append(failed, postInstallStepName(step))
		}
		changes, trackErr := tracker.AfterStep(err == nil)
		reportStepChanges(label, err == nil, changes, trackErr)
	}

	if len(failed) == 0 {
		return nil
	}
	if len(steps) > 1 {
		out.Error(fmt.Sprintf("%d of %d post_install steps failed (install succeeded): %s",
			len(failed), len(steps), strings.Join(failed, ", ")))
	}
	return fmt.Errorf("%d post_install step(s) failed", len(failed))
}

// postInstallLabel names a step in output. The single-argv shorthand keeps the
// wording it had before steps existed.
func postInstallLabel(step profile.PostInstallStep) string {
	if step.Name == "" {
		return shorthandStepLabel
	}
	return "post_install step " + postInstallStepName(step)
}

// shorthandStepLabel names the unnamed step a single-argv post_install makes.
const shorthandStepLabel = "post_install hook"

// postInstallStepName quotes a step's name, adding the bundle that declared it
// so a bundle step and a profile step sharing a name can be told apart.
func postInstallStepName(step profile.PostInstallStep) string {
	if step.Name == "" {
		return shorthandStepLabel
	}
	if step.Bundle == "" {
		return fmt.Sprintf("%q", step.Name)
	}
	return fmt.Sprintf("%q from bundle %q", step.Name, step.Bundle)
}

// reportStepChanges tells the user which managed files a step changed and
// whether those edits were baselined. Reporting is the point: a baselined edit
// is one tai will no longer flag, so this is the one moment the user gets to
// see what the step did.
func reportStepChanges(label string, succeeded bool, changes install.StepChanges, trackErr error) {
	if len(changes.Baselined) > 0 {
		out.Info(fmt.Sprintf("%s modified %d managed file(s) (manifest baseline updated):", label, len(changes.Baselined)))
		for _, path := range changes.Baselined {
			out.Raw("    ~ " + path)
		}
	}
	if trackErr != nil {
		out.Warning(fmt.Sprintf("could not update the manifest baseline after the %s: %v. "+
			"Files it modified will be reported as conflicts on the next install; "+
			"rerun the install, or pass --force once to accept them.", label, trackErr))
	}
	if len(changes.Held) == 0 {
		return
	}

	why := "before failing"
	if succeeded {
		why = "that a failed step had already modified"
	}
	out.Warning(fmt.Sprintf("%s modified %d managed file(s) %s. They keep their previous baseline, "+
		"so the next install reports them as conflicts; once the failure is fixed, "+
		"reinstall with --force to render them from source again:", label, len(changes.Held), why))
	for _, path := range changes.Held {
		out.Raw("    ! " + path)
	}
}

// reportPatchFailure renders a patch that could not be applied, with the steps
// to resolve it. A patch failure is a routine part of tracking upstream, not an
// internal error - if it surfaces as an opaque message people will stop using
// patches rather than fix them, so it names the file to edit, the content it
// failed against, and the workspace left on disk to repair it in.
func reportPatchFailure(err error) {
	var viewErr *source.ViewError
	if !errors.As(err, &viewErr) {
		fatal("%v", err)
		return
	}

	p := viewErr.Patch
	out.Error(fmt.Sprintf("patch '%s' could not be applied - install aborted, nothing was changed:", p.Name))

	var b strings.Builder
	fmt.Fprintf(&b, "\n  patch:  %s\n", p.PatchPath)
	fmt.Fprintf(&b, "  file:   %s\n", p.FileRef())

	if ae := viewErr.ApplyErr; ae != nil {
		if ae.Matches == 0 {
			fmt.Fprintf(&b, "  edit #%d: find text not found - upstream has likely changed since this patch was written\n", ae.EditIndex+1)
		} else {
			fmt.Fprintf(&b, "  edit #%d: ambiguous - matched %d locations (lines %s)\n", ae.EditIndex+1, ae.Matches, joinInts(ae.Lines))
		}
		for _, line := range strings.Split(strings.TrimSuffix(ae.Find, "\n"), "\n") {
			fmt.Fprintf(&b, "    | %s\n", line)
		}
		if ae.Hint != "" {
			fmt.Fprintf(&b, "  Hint: %s\n", ae.Hint)
		}
	} else {
		fmt.Fprintf(&b, "  %s\n", viewErr.Reason)
	}

	// Steps are numbered as they are written, so the workspace step can be
	// omitted without leaving a gap in the sequence.
	b.WriteString("\n  To fix:\n")
	step := 0
	addStep := func(format string, args ...any) {
		step++
		fmt.Fprintf(&b, "    %d. %s\n", step, fmt.Sprintf(format, args...))
	}

	addStep("%-38s tai patch diff %s", "See what changed upstream:", p.Name)
	if viewErr.Workspace != "" {
		// Point at regenerate rather than the patch file: hand-writing
		// find/replace is what the workspace exists to avoid, and every other
		// message in this feature routes people through regenerate.
		addStep("%-38s %s", "Edit the file to match what you want:", viewErr.Workspace)
		addStep("%-38s tai patch regenerate %s", "Turn that edit into the patch:", p.Name)
		addStep("%-38s %s", "Review the rewritten edits:", p.PatchPath)
	} else {
		addStep("%-38s %s", "Update the edits in:", p.PatchPath)
	}
	addStep("%-38s tai patch bless %s", "Accept the new upstream:", p.Name)
	addStep("Re-run the install")

	out.Raw(b.String())
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// outputLogger wraps Output to implement the logger interface expected by ProfileResolver.
type outputLogger struct {
	out output.Output
}

func (l *outputLogger) Printf(format string, args ...any) {
	msg := strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
	l.out.Info(msg)
}

func runUninstall(target string) {
	refuseUnderOverride("uninstall", overrideWritesManifests)
	s := loadGlobalState()

	adapter, err := s.GetAdapter(target)
	if err != nil {
		fatal("%v", err)
	}

	out.Info("Uninstalling from " + target + "...")

	result, err := s.Installer.Uninstall(adapter)
	if err != nil {
		fatal("uninstallation failed: %v", err)
	}

	if len(result.Removed) == 0 {
		out.Info("Nothing to uninstall.")
		return
	}

	for _, path := range result.Removed {
		out.Raw("  - " + path)
	}

	// "files" would be wrong for config assets, which are removed as entries
	// inside a target's config files rather than as files of their own.
	out.Success(strconv.Itoa(len(result.Removed)) + " removed")

	if len(result.Errors) > 0 {
		out.Warning("Some files could not be removed:")
		for _, e := range result.Errors {
			out.Raw("  " + e.Error())
		}
	}
}

// --- Status command ---

func runStatus(target string) {
	s := loadGlobalState()
	if o := repoOverride; o != nil {
		out.Info(fmt.Sprintf("Ignoring the config repo override %s (from %s): status reports what is installed on this machine, which every config repo shares.", o.Path, o.Origin))
	}

	targets := []string{target}
	if target == "" {
		// Show all registered targets
		targets = adapter.DefaultRegistry.List()
		sort.Strings(targets)
	}

	for _, t := range targets {
		manifest, err := s.Installer.Status(t)
		if err != nil {
			out.Error(t + ": error loading status: " + err.Error())
			continue
		}

		if manifest == nil {
			out.Info(t + ": not installed")
			continue
		}

		out.Header(t + ":")
		out.KeyValueBlock([]output.KeyValuePair{
			{Key: "Profile", Value: manifest.Profile},
			{Key: "Config repo", Value: manifest.ConfigRepo},
			{Key: "Installed", Value: manifest.InstalledAt.Format("2006-01-02 15:04:05")},
			{Key: "Files", Value: strconv.Itoa(len(manifest.Files))},
			{Key: "Config entries", Value: strconv.Itoa(len(manifest.ConfigEntries))},
		})

		if len(manifest.Files) > 0 {
			out.Header("  Installed files:")
			for _, f := range manifest.Files {
				out.Raw("    " + f.Path)
			}
		}

		// Config assets install as entries inside a target's config files, so
		// they show up nowhere in the file list. Without this, "did my hook
		// install?" has no answer short of grepping settings.json by hand.
		if len(manifest.ConfigEntries) > 0 {
			out.Header("  Installed config entries:")
			for _, e := range manifest.ConfigEntries {
				out.Raw(fmt.Sprintf("    [%s] %s -> %s", e.Type, e.Name, e.SettingsFile))
			}
		}
	}
}

// --- Locate command ---

func runLocate(assetPath string) {
	s := loadState()
	requireActiveConfig(s)

	path, err := s.LocateAsset(assetPath)
	if err != nil {
		fatal("%v", err)
	}

	out.Raw(path)
}

// --- Run command ---

func runRun(target string, args []string) {
	// A --config that reaches tai came before the target; one after it is
	// in args. run reads no config repo, so tai's --config does nothing here,
	// and was likely meant for the target (codex has a --config of its own).
	// TAI_CONFIG stays ignored: exported for other commands, it is not a mistake.
	if repoOverride != nil && repoOverride.Origin == "--config" {
		fatal("--config before the target is tai's own flag, and 'tai run' reads no config repo, so it has no effect; to pass --config to %s, put it after the target: tai run %s --config ...", target, target)
	}
	s := loadGlobalState()

	adapter, err := s.GetAdapter(target)
	if err != nil {
		fatal("%v", err)
	}

	// Just pass through to the target
	if err := adapter.Run(args); err != nil {
		// If it's an exit error, preserve the exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fatal("running %s: %v", target, err)
	}
}
