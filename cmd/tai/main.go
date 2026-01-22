package main

import (
	"fmt"
	"os"

	"github.com/amterp/ra"
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
		// No subcommand - show help (handled by SetAutoHelpOnNoArgs)
		fmt.Print(root.GenerateUsage(false))
	}
}

// Placeholder handlers - will be implemented in separate files
func runConfigSet(path string) {
	fmt.Printf("Setting config repo to: %s\n", path)
	// TODO: Implement
}

func runConfigShow() {
	fmt.Println("Current config settings:")
	// TODO: Implement
}

func runListSources() {
	fmt.Println("Available sources:")
	// TODO: Implement
}

func runListBundles() {
	fmt.Println("Available bundles:")
	// TODO: Implement
}

func runListProfiles() {
	fmt.Println("Available profiles:")
	// TODO: Implement
}

func runListAssets(source string) {
	if source != "" {
		fmt.Printf("Assets from source '%s':\n", source)
	} else {
		fmt.Println("All assets:")
	}
	// TODO: Implement
}

func runShowProfile(name string) {
	fmt.Printf("Profile: %s\n", name)
	// TODO: Implement
}

func runShowBundle(name string) {
	fmt.Printf("Bundle: %s\n", name)
	// TODO: Implement
}

func runInstall(profile string, dryRun, force, skipConflicts bool) {
	if dryRun {
		fmt.Printf("Would install profile '%s' (dry run)\n", profile)
	} else {
		fmt.Printf("Installing profile '%s'\n", profile)
	}
	// TODO: Implement
}

func runUninstall(target string) {
	fmt.Printf("Uninstalling from target: %s\n", target)
	// TODO: Implement
}

func runStatus(target string) {
	if target != "" {
		fmt.Printf("Status for target '%s':\n", target)
	} else {
		fmt.Println("Status for all targets:")
	}
	// TODO: Implement
}

func runLocate(assetPath string) {
	fmt.Printf("Locating: %s\n", assetPath)
	// TODO: Implement
}

func runRun(target string, args []string) {
	fmt.Printf("Running %s with args: %v\n", target, args)
	// TODO: Implement
}
