package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func setupStatusLineTest(t *testing.T) (*testutil.TestEnv, *Installer, *claude.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	return env, installer, adapter
}

func TestInstallerStatusLineTaiFormat(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	// Create a tai-format statusline config
	statuslineContent := `level: detailed
show:
  - cost
  - model
  - tokens
`
	env.SetupAsset("statuslines", "detailed.yaml", statuslineContent)

	assets := []*config.Asset{
		{Type: "statusline", Name: "detailed.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "detailed.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// Check settings.json was created with correct content
	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	statusLine, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine not found in settings.json")
	}

	if statusLine["style"] != "detailed" {
		t.Errorf("statusLine.style = %v, want 'detailed'", statusLine["style"])
	}
	if statusLine["showCost"] != true {
		t.Errorf("statusLine.showCost = %v, want true", statusLine["showCost"])
	}
	if statusLine["showModel"] != true {
		t.Errorf("statusLine.showModel = %v, want true", statusLine["showModel"])
	}
	if statusLine["showTokens"] != true {
		t.Errorf("statusLine.showTokens = %v, want true", statusLine["showTokens"])
	}
}

func TestInstallerStatusLineMinimal(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	statuslineContent := `level: minimal
show:
  - model
`
	env.SetupAsset("statuslines", "minimal.yaml", statuslineContent)

	assets := []*config.Asset{
		{Type: "statusline", Name: "minimal.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "minimal.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	if statusLine["style"] != "minimal" {
		t.Errorf("statusLine.style = %v, want 'minimal'", statusLine["style"])
	}
	if statusLine["showModel"] != true {
		t.Errorf("statusLine.showModel = %v, want true", statusLine["showModel"])
	}
	// showCost should be false since it wasn't in show
	if statusLine["showCost"] != false {
		t.Errorf("statusLine.showCost = %v, want false", statusLine["showCost"])
	}
}

func TestInstallerStatusLineHidden(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	statuslineContent := `level: hidden
`
	env.SetupAsset("statuslines", "hidden.yaml", statuslineContent)

	assets := []*config.Asset{
		{Type: "statusline", Name: "hidden.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "hidden.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	if statusLine["style"] != "hidden" {
		t.Errorf("statusLine.style = %v, want 'hidden'", statusLine["style"])
	}
}

func TestInstallerStatusLineCustomCommand(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	statuslineContent := `level: custom
command: ~/.scripts/statusline.sh
padding: 2
`
	env.SetupAsset("statuslines", "custom.yaml", statuslineContent)

	assets := []*config.Asset{
		{Type: "statusline", Name: "custom.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "custom.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	if statusLine["type"] != "command" {
		t.Errorf("statusLine.type = %v, want 'command'", statusLine["type"])
	}
	if statusLine["command"] != "~/.scripts/statusline.sh" {
		t.Errorf("statusLine.command = %v, want '~/.scripts/statusline.sh'", statusLine["command"])
	}
	// padding is stored as float64 in JSON
	if statusLine["padding"] != float64(2) {
		t.Errorf("statusLine.padding = %v, want 2", statusLine["padding"])
	}
}

func TestInstallerStatusLineNativeFormat(t *testing.T) {
	// Native YAML format passthrough is not yet fully implemented.
	// The plan specifies native configs should be passed through, but the
	// current implementation tries to JSON unmarshal native content.
	// TODO: Implement proper YAML-to-JSON conversion for native passthrough.
	t.Skip("Native YAML passthrough not yet fully implemented")
}

func TestInstallerStatusLineSingletonOverride(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	// Create two statusline configs - later should win
	env.SetupAsset("statuslines", "first.yaml", "level: minimal\n")
	env.SetupAsset("statuslines", "second.yaml", "level: detailed\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "first.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "first.yaml")},
		{Type: "statusline", Name: "second.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "second.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Only one should be added (the second one wins)
	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	// Second config should win (detailed)
	if statusLine["style"] != "detailed" {
		t.Errorf("statusLine.style = %v, want 'detailed' (second should override first)", statusLine["style"])
	}
}

func TestInstallerStatusLineConflictUnmanaged(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	// Create existing settings.json with statusLine not managed by tai
	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	if err := os.MkdirAll(env.TargetDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingSettings := map[string]any{
		"statusLine": map[string]any{
			"style": "minimal",
		},
	}
	existingData, _ := json.MarshalIndent(existingSettings, "", "  ")
	if err := os.WriteFile(settingsPath, existingData, 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("statuslines", "new.yaml", "level: detailed\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "new.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "new.yaml")},
	}

	// Should fail without --force
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected conflict error for unmanaged statusLine")
	}
	if !strings.Contains(err.Error(), "not managed by tai") {
		t.Errorf("error = %q, want mention of 'not managed by tai'", err.Error())
	}

	// Should succeed with --force
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}
}

func TestInstallerStatusLineUpdate(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "config.yaml", "level: minimal\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	// First install
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Update the config
	env.SetupAsset("statuslines", "config.yaml", "level: detailed\n")

	// Second install
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Updated) != 1 {
		t.Errorf("len(Updated) = %d, want 1", len(result.Updated))
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	if statusLine["style"] != "detailed" {
		t.Errorf("statusLine.style = %v, want 'detailed'", statusLine["style"])
	}
}

func TestInstallerStatusLineUnchanged(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "config.yaml", "level: minimal\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	// First install
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Second install with same config
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("len(Unchanged) = %d, want 1", len(result.Unchanged))
	}
	if len(result.Updated) != 0 {
		t.Errorf("len(Updated) = %d, want 0", len(result.Updated))
	}
}

func TestInstallerStatusLineRemoved(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "config.yaml", "level: minimal\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	// First install
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Second install without statusline
	result, err := installer.Install(adapter, "test-profile", []*config.Asset{}, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	// statusLine should be removed from settings.json
	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	if _, exists := settings["statusLine"]; exists {
		t.Error("statusLine should have been removed from settings.json")
	}
}

func TestInstallerStatusLineUninstall(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "config.yaml", "level: detailed\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	// Install
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Uninstall
	result, err := installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	// statusLine should be removed from settings.json
	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	if _, exists := settings["statusLine"]; exists {
		t.Error("statusLine should have been removed from settings.json")
	}
}

func TestInstallerStatusLineMixedWithFileAssets(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	// Create both file assets and statusline
	env.SetupAsset("personas", "test.md", "---\nname: Test\n---\n\n# Test Persona")
	env.SetupAsset("statuslines", "config.yaml", "level: detailed\n")

	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 2 {
		t.Errorf("len(Added) = %d, want 2", len(result.Added))
	}

	// Check persona was installed
	if !env.TargetFileExists("output-styles/test.md") {
		t.Error("output-styles/test.md not created")
	}

	// Check statusLine in settings.json
	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	if _, exists := settings["statusLine"]; !exists {
		t.Error("statusLine not found in settings.json")
	}

	// Check manifest has both entries
	if result.Manifest == nil {
		t.Fatal("Manifest is nil")
	}
	if len(result.Manifest.Files) != 1 {
		t.Errorf("len(Manifest.Files) = %d, want 1", len(result.Manifest.Files))
	}
	if len(result.Manifest.ConfigEntries) != 1 {
		t.Errorf("len(Manifest.ConfigEntries) = %d, want 1", len(result.Manifest.ConfigEntries))
	}
}

func TestInstallerStatusLineDryRun(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "config.yaml", "level: detailed\n")

	assets := []*config.Asset{
		{Type: "statusline", Name: "config.yaml", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "config.yaml")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// settings.json should NOT be created in dry-run mode
	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings.json should not be created in dry-run mode")
	}
}

// --- Validation tests ---

func TestValidateStatusLineConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.StatusLineConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: false,
		},
		{
			name:    "valid minimal",
			cfg:     &config.StatusLineConfig{Level: "minimal"},
			wantErr: false,
		},
		{
			name:    "valid detailed with show",
			cfg:     &config.StatusLineConfig{Level: "detailed", Show: []string{"cost", "model"}},
			wantErr: false,
		},
		{
			name:    "valid hidden",
			cfg:     &config.StatusLineConfig{Level: "hidden"},
			wantErr: false,
		},
		{
			name:    "valid custom",
			cfg:     &config.StatusLineConfig{Level: "custom", Command: "~/script.sh"},
			wantErr: false,
		},
		{
			name:    "valid custom with padding",
			cfg:     &config.StatusLineConfig{Level: "custom", Command: "~/script.sh", Padding: intPtr(2)},
			wantErr: false,
		},
		{
			name:    "invalid level",
			cfg:     &config.StatusLineConfig{Level: "invalid"},
			wantErr: true,
			errMsg:  "invalid level",
		},
		{
			name:    "custom without command",
			cfg:     &config.StatusLineConfig{Level: "custom"},
			wantErr: true,
			errMsg:  "command",
		},
		{
			name:    "custom with show",
			cfg:     &config.StatusLineConfig{Level: "custom", Command: "~/script.sh", Show: []string{"cost"}},
			wantErr: true,
			errMsg:  "show",
		},
		{
			name:    "hidden with show",
			cfg:     &config.StatusLineConfig{Level: "hidden", Show: []string{"cost"}},
			wantErr: true,
			errMsg:  "show",
		},
		{
			name:    "minimal with command",
			cfg:     &config.StatusLineConfig{Level: "minimal", Command: "~/script.sh"},
			wantErr: true,
			errMsg:  "command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStatusLineConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStatusLineConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}
