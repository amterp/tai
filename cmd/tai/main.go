package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"

	"github.com/amterp/ra"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/state"
	"github.com/amterp/tai/internal/template"
)

const version = "0.1.0"

func main() {
	root := ra.NewCmd("tai")
	root.SetDescription("Manage AI agent configurations across targets (v" + version + ")")
	root.SetAutoHelpOnNoArgs(true)

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
	installUsed, _ := root.RegisterCmd(installCmd)

	// Uninstall command
	uninstallCmd := ra.NewCmd("uninstall")
	uninstallCmd.SetDescription("Remove tai-managed files for a target")
	uninstallTarget, _ := ra.NewString("target").
		SetUsage("Target to uninstall (e.g., claude)").
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
		SetUsage("Target to run (e.g., claude)").
		Register(runCmd)
	runArgs, _ := ra.NewStringSlice("args").
		SetOptional(true).
		SetUsage("Arguments to pass to the target").
		Register(runCmd)
	runUsed, _ := root.RegisterCmd(runCmd)

	// Parse
	root.ParseOrExit(os.Args[1:])

	// Route to appropriate handler
	switch {
	case *configUsed:
		switch {
		case *configSetUsed:
			runConfigSet(*configPath)
		case *configShowUsed:
			runConfigShow()
		default:
			fmt.Fprintln(os.Stderr, "Usage: tai config <set|show>")
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
			fmt.Fprintln(os.Stderr, "Usage: tai list <sources|bundles|profiles|assets>")
			os.Exit(1)
		}
	case *showUsed:
		switch {
		case *showProfileUsed:
			runShowProfile(*showProfileName)
		case *showBundleUsed:
			runShowBundle(*showBundleName)
		default:
			fmt.Fprintln(os.Stderr, "Usage: tai show <profile|bundle> <name>")
			os.Exit(1)
		}
	case *installUsed:
		runInstall(*installProfile, *installDryRun, *installForce, *installSkipConflicts)
	case *uninstallUsed:
		runUninstall(*uninstallTarget)
	case *statusUsed:
		runStatus(*statusTarget)
	case *locateUsed:
		runLocate(*locateAsset)
	case *runUsed:
		runRun(*runTarget, *runArgs)
	default:
		fmt.Print(root.GenerateUsage(false))
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func loadState() *state.State {
	s, err := state.Load()
	if err != nil {
		fatal("loading state: %v", err)
	}
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
	fmt.Printf("Active config set to: %s\n", s.GlobalConfig.ActiveConfig)
}

func runConfigShow() {
	s := loadState()
	fmt.Printf("Config directory: %s\n", s.Paths.ConfigDir)
	fmt.Printf("Cache directory:  %s\n", s.Paths.CacheDir)
	if s.GlobalConfig.ActiveConfig != "" {
		fmt.Printf("Active config:    %s\n", s.GlobalConfig.ActiveConfig)
	} else {
		fmt.Println("Active config:    (none)")
	}
}

// --- List commands ---

func runListSources() {
	s := loadState()
	requireActiveConfig(s)

	fmt.Println("Sources:")
	for _, src := range s.TaiConfig.Sources {
		typeStr := src.Type
		if typeStr == "" {
			typeStr = "local"
		}
		fmt.Printf("  %-15s  %-10s  %s\n", src.Name, typeStr, src.Path)
	}
}

func runListBundles() {
	s := loadState()
	requireActiveConfig(s)

	if len(s.Bundles) == 0 {
		fmt.Println("No bundles defined.")
		return
	}

	fmt.Println("Bundles:")
	names := make([]string, 0, len(s.Bundles))
	for name := range s.Bundles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		b := s.Bundles[name]
		desc := b.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  %-20s  %s\n", name, desc)
	}
}

func runListProfiles() {
	s := loadState()
	requireActiveConfig(s)

	if len(s.Profiles) == 0 {
		fmt.Println("No profiles defined.")
		return
	}

	fmt.Println("Profiles:")
	names := make([]string, 0, len(s.Profiles))
	for name := range s.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := s.Profiles[name]
		desc := p.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  %-15s  %-10s  %s\n", name, p.Target, desc)
	}
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
		fmt.Printf("Assets from source '%s':\n", sourceName)
		assets := src.ListAllAssets()
		sort.Strings(assets)
		for _, a := range assets {
			fmt.Printf("  %s\n", a)
		}
	} else {
		// All sources
		for name, src := range s.Sources {
			assets := src.ListAllAssets()
			if len(assets) == 0 {
				continue
			}
			fmt.Printf("%s:\n", name)
			sort.Strings(assets)
			for _, a := range assets {
				fmt.Printf("  %s\n", a)
			}
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

	fmt.Printf("Profile: %s\n", resolved.Name)
	fmt.Printf("Target:  %s\n", resolved.Target)
	fmt.Printf("Assets:  %d\n", len(resolved.Assets))
	fmt.Println()

	for _, asset := range resolved.Assets {
		fmt.Printf("  %s/%s (%s)\n", asset.Type, asset.Name, asset.SourceName)
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
	fmt.Printf("Bundle: %s\n", resolved.Name)
	if b.Description != "" {
		fmt.Printf("Description: %s\n", b.Description)
	}
	fmt.Printf("Assets: %d\n", len(resolved.Assets))
	fmt.Println()

	for _, asset := range resolved.Assets {
		fmt.Printf("  %s/%s (%s)\n", asset.Type, asset.Name, asset.SourceName)
	}
}

// --- Install commands ---

func runInstall(profileName string, dryRun, force, skipConflicts bool) {
	s := loadState()
	requireActiveConfig(s)

	// Resolve profile
	resolved, err := s.ResolveProfile(profileName)
	if err != nil {
		fatal("resolving profile: %v", err)
	}

	// Get adapter
	adapter, err := s.GetAdapter(resolved.Target)
	if err != nil {
		fatal("%v", err)
	}

	// Set up logging for overrides
	logger := log.New(os.Stdout, "ℹ ", 0)
	s.ProfileResolver.SetLogger(logger)

	// Prepare install options
	opts := install.InstallOptions{
		DryRun:        dryRun,
		Force:         force,
		SkipConflicts: skipConflicts,
		Variables:     template.Variables{}, // Could add custom variables here
	}

	// Convert resolved assets
	assets := make([]*config.Asset, len(resolved.Assets))
	for i, a := range resolved.Assets {
		assets[i] = a
	}

	// Install
	if dryRun {
		fmt.Printf("Would install profile '%s' for %s...\n", profileName, resolved.Target)
	} else {
		fmt.Printf("Installing profile '%s' for %s...\n", profileName, resolved.Target)
	}

	result, err := s.Installer.Install(adapter, profileName, assets, opts)
	if err != nil {
		fatal("installation failed: %v", err)
	}

	// Report results
	if len(result.Added) > 0 {
		for _, path := range result.Added {
			fmt.Printf("  + %s (new)\n", path)
		}
	}
	if len(result.Updated) > 0 {
		for _, path := range result.Updated {
			fmt.Printf("  ~ %s (updated)\n", path)
		}
	}
	if len(result.Removed) > 0 {
		for _, path := range result.Removed {
			fmt.Printf("  - %s (removed)\n", path)
		}
	}
	if len(result.Skipped) > 0 {
		for _, path := range result.Skipped {
			fmt.Printf("  ? %s (skipped)\n", path)
		}
	}

	// Summary
	fmt.Println()
	if dryRun {
		fmt.Printf("%d would be added, %d updated, %d removed\n",
			len(result.Added), len(result.Updated), len(result.Removed))
	} else {
		fmt.Printf("%d added, %d updated, %d removed\n",
			len(result.Added), len(result.Updated), len(result.Removed))
		fmt.Printf("Manifest written to %s\n", s.Paths.ManifestDir)
	}
}

func runUninstall(target string) {
	s := loadState()

	adapter, err := s.GetAdapter(target)
	if err != nil {
		fatal("%v", err)
	}

	fmt.Printf("Uninstalling from %s...\n", target)

	result, err := s.Installer.Uninstall(adapter)
	if err != nil {
		fatal("uninstallation failed: %v", err)
	}

	if len(result.Removed) == 0 {
		fmt.Println("Nothing to uninstall.")
		return
	}

	for _, path := range result.Removed {
		fmt.Printf("  - %s\n", path)
	}

	fmt.Printf("\n%d files removed\n", len(result.Removed))

	if len(result.Errors) > 0 {
		fmt.Println("\nWarnings:")
		for _, err := range result.Errors {
			fmt.Printf("  %v\n", err)
		}
	}
}

// --- Status command ---

func runStatus(target string) {
	s := loadState()

	targets := []string{target}
	if target == "" {
		// Show all known targets
		targets = []string{"claude"} // Could dynamically discover installed targets
	}

	for _, t := range targets {
		manifest, err := s.Installer.Status(t)
		if err != nil {
			fmt.Printf("%s: error loading status: %v\n", t, err)
			continue
		}

		if manifest == nil {
			fmt.Printf("%s: not installed\n", t)
			continue
		}

		fmt.Printf("%s:\n", t)
		fmt.Printf("  Profile:     %s\n", manifest.Profile)
		fmt.Printf("  Config repo: %s\n", manifest.ConfigRepo)
		fmt.Printf("  Installed:   %s\n", manifest.InstalledAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Files:       %d\n", len(manifest.Files))

		if len(manifest.Files) > 0 {
			fmt.Println("  Installed files:")
			for _, f := range manifest.Files {
				fmt.Printf("    %s\n", f.Path)
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

	fmt.Println(path)
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
