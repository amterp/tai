package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/amterp/ra"
	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/output"
	"github.com/amterp/tai/internal/state"
	"github.com/amterp/tai/internal/template"
)

const version = "0.1.0"

// out is the global output instance, initialized after flag parsing.
var out output.Output

func main() {
	root := ra.NewCmd("tai")
	root.SetDescription("Manage AI agent configurations across targets (v" + version + ")")
	root.SetAutoHelpOnNoArgs(true)

	// Global flags
	noColor, _ := ra.NewBool("no-color").
		SetDefault(false).
		SetUsage("Disable colored output").
		Register(root)

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
	reinstallUsed, _ := root.RegisterCmd(reinstallCmd)

	// Refresh command
	refreshCmd := ra.NewCmd("refresh")
	refreshCmd.SetDescription("Fetch command sources into the cache")
	refreshSource, _ := ra.NewString("source").
		SetOptional(true).
		SetUsage("Source to refresh (default: all command sources)").
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
	runTarget, _ := ra.NewString("target").
		SetUsage("Target to run (e.g., claude, codex)").
		Register(runCmd)
	runArgs, _ := ra.NewStringSlice("args").
		SetOptional(true).
		SetUsage("Arguments to pass to the target").
		Register(runCmd)
	runUsed, _ := root.RegisterCmd(runCmd)

	// Parse
	root.ParseOrExit(os.Args[1:])

	// Initialize output after flag parsing
	out = output.New(output.Options{
		Writer:    os.Stdout,
		ErrWriter: os.Stderr,
		NoColor:   *noColor,
	})

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
	case *installUsed:
		runInstall(*installProfile, *installDryRun, *installForce, *installSkipConflicts, *installVars, *installDiff)
	case *reinstallUsed:
		runReinstall(*reinstallTarget, *reinstallDryRun, *reinstallForce, *reinstallSkipConflicts, *reinstallVars, *reinstallDiff)
	case *refreshUsed:
		runRefresh(*refreshSource)
	case *uninstallUsed:
		runUninstall(*uninstallTarget)
	case *statusUsed:
		runStatus(*statusTarget)
	case *locateUsed:
		runLocate(*locateAsset)
	case *runUsed:
		runRun(*runTarget, *runArgs)
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
	s, err := state.Load()
	if err != nil {
		fatal("loading state: %v", err)
	}
	s.SetVersion(version)
	return s
}

func requireActiveConfig(s *state.State) {
	if err := s.RequireActiveConfig(); err != nil {
		fatal("%v", err)
	}
}

// --- Config commands ---

func runConfigSet(path string) {
	s := loadState()
	if err := s.SetActiveConfig(path); err != nil {
		fatal("setting active config: %v", err)
	}
	out.Success("Active config set to: " + s.GlobalConfig.ActiveConfig)
}

func runConfigShow() {
	s := loadState()
	out.KeyValueBlock([]output.KeyValuePair{
		{Key: "Config directory", Value: s.Paths.ConfigDir},
		{Key: "Cache directory", Value: s.Paths.CacheDir},
		{Key: "Active config", Value: valueOrNone(s.GlobalConfig.ActiveConfig)},
	})
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
		if src.IsCommand() {
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

	if len(s.Bundles) == 0 {
		out.Info("No bundles defined.")
		return
	}

	names := make([]string, 0, len(s.Bundles))
	for name := range s.Bundles {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]output.ListItem, 0, len(names))
	for _, name := range names {
		b := s.Bundles[name]
		desc := b.Description
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, output.ListItem{
			Name:        name,
			Description: desc,
		})
	}
	out.List("Bundles:", items)
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
}

func runShowBundle(name string) {
	s := loadState()
	requireActiveConfig(s)

	resolved, err := s.BundleResolver.Resolve(name)
	if err != nil {
		fatal("resolving bundle: %v", err)
	}

	b := s.Bundles[name]
	pairs := []output.KeyValuePair{
		{Key: "Bundle", Value: resolved.Name},
	}
	if b.Description != "" {
		pairs = append(pairs, output.KeyValuePair{Key: "Description", Value: b.Description})
	}
	pairs = append(pairs, output.KeyValuePair{Key: "Assets", Value: strconv.Itoa(len(resolved.Assets))})
	out.KeyValueBlock(pairs)

	out.Blank()
	for _, asset := range resolved.Assets {
		out.Raw("  " + asset.Type + "/" + asset.Name + " (" + asset.SourceName + ")")
	}
}

// --- Install commands ---

func runInstall(profileName string, dryRun, force, skipConflicts bool, varsStr string, showDiff bool) {
	s := loadState()
	requireActiveConfig(s)
	refreshSourcesForInstall(s, dryRun)
	if err := doInstall(s, profileName, dryRun, force, skipConflicts, varsStr, showDiff); err != nil {
		os.Exit(1)
	}
}

// refreshSourcesForInstall fetches command sources so installs always see
// fresh content. Dry-run skips the fetch (it's a side effect a preview
// shouldn't have) and resolves from whatever is cached.
func refreshSourcesForInstall(s *state.State, dryRun bool) {
	if !s.HasCommandSources() {
		return
	}
	if dryRun {
		for _, src := range s.TaiConfig.Sources {
			if src.IsCommand() {
				out.Info("Would refresh command source '" + src.Name + "'")
			}
		}
		return
	}
	err := s.RefreshCommandSources("", func(name string) {
		out.Info("Refreshing command source '" + name + "'...")
	})
	if err != nil {
		fatal("refreshing sources: %v", err)
	}
}

func runRefresh(sourceName string) {
	s := loadState()
	requireActiveConfig(s)

	if !s.HasCommandSources() {
		out.Info("No command sources configured.")
		return
	}

	refreshed := 0
	err := s.RefreshCommandSources(sourceName, func(name string) {
		out.Info("Refreshing command source '" + name + "'...")
		refreshed++
	})
	if err != nil {
		fatal("refreshing sources: %v", err)
	}
	out.Success(fmt.Sprintf("Refreshed %d source(s).", refreshed))
}

func runReinstall(target string, dryRun, force, skipConflicts bool, varsStr string, showDiff bool) {
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

	failed := false
	for i, rt := range toReinstall {
		if i > 0 {
			out.Blank()
		}
		if err := doInstall(s, rt.profile, dryRun, force, skipConflicts, varsStr, showDiff); err != nil {
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
// state. Errors that abort the install are fatal (they exit the process);
// the returned error reports post-install-hook failure, where the install
// itself succeeded but tai should exit nonzero.
func doInstall(s *state.State, profileName string, dryRun, force, skipConflicts bool, varsStr string, showDiff bool) error {
	resolved, err := s.ResolveProfile(profileName)
	if err != nil {
		fatal("resolving profile: %v", err)
	}

	adapter, err := s.GetAdapter(resolved.Target)
	if err != nil {
		fatal("%v", err)
	}

	s.ProfileResolver.SetLogger(&outputLogger{out: out})
	s.Installer.SetLogger(&outputLogger{out: out})

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
	opts := install.InstallOptions{
		DryRun:        dryRun,
		Force:         force,
		SkipConflicts: skipConflicts,
		Variables:     mergedVars,
	}

	assets := make([]*config.Asset, len(resolved.Assets))
	copy(assets, resolved.Assets)

	if dryRun {
		out.Info("Would install profile '" + profileName + "' for " + resolved.Target + "...")
	} else {
		out.Info("Installing profile '" + profileName + "' for " + resolved.Target + "...")
	}

	result, err := s.Installer.Install(adapter, profileName, assets, opts)
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
			os.Exit(1)
		}
		fatal("installation failed: %v", err)
	}

	manifestPath := ""
	if !dryRun {
		manifestPath = s.Paths.ManifestDir
	}
	out.InstallResult(&output.InstallResultData{
		Added:     result.Added,
		Updated:   result.Updated,
		Unchanged: result.Unchanged,
		Removed:   result.Removed,
		Skipped:   result.Skipped,
		DryRun:    dryRun,
		Manifest:  manifestPath,
	})

	return nil
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
	s := loadState()

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

	out.Success(strconv.Itoa(len(result.Removed)) + " files removed")

	if len(result.Errors) > 0 {
		out.Warning("Some files could not be removed:")
		for _, e := range result.Errors {
			out.Raw("  " + e.Error())
		}
	}
}

// --- Status command ---

func runStatus(target string) {
	s := loadState()

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
		})

		if len(manifest.Files) > 0 {
			out.Header("  Installed files:")
			for _, f := range manifest.Files {
				out.Raw("    " + f.Path)
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
	s := loadState()

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
