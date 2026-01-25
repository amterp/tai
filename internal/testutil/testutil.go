// Package testutil provides test helpers for tai tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnv provides an isolated filesystem environment for tests.
type TestEnv struct {
	T          *testing.T
	Root       string // Root temp directory
	ConfigDir  string // Simulated ~/.config/tai
	CacheDir   string // Simulated ~/.cache/tai
	ConfigRepo string // Simulated config repository
	TargetDir  string // Simulated target directory (e.g., ~/.claude)
}

// NewTestEnv creates a new isolated test environment.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	root := t.TempDir()

	env := &TestEnv{
		T:          t,
		Root:       root,
		ConfigDir:  filepath.Join(root, "config", "tai"),
		CacheDir:   filepath.Join(root, "cache", "tai"),
		ConfigRepo: filepath.Join(root, "config-repo"),
		TargetDir:  filepath.Join(root, "target"),
	}

	// Create directories
	for _, dir := range []string{
		env.ConfigDir,
		filepath.Join(env.ConfigDir, "manifests"),
		env.CacheDir,
		env.ConfigRepo,
		filepath.Join(env.ConfigRepo, "assets", "personas"),
		filepath.Join(env.ConfigRepo, "assets", "delegates"),
		filepath.Join(env.ConfigRepo, "assets", "skills"),
		filepath.Join(env.ConfigRepo, "assets", "docs"),
		filepath.Join(env.ConfigRepo, "assets", "statuslines"),
		filepath.Join(env.ConfigRepo, "bundles"),
		filepath.Join(env.ConfigRepo, "profiles"),
		env.TargetDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
	}

	return env
}

// WriteFile writes content to a file relative to the test environment root.
func (e *TestEnv) WriteFile(relativePath, content string) {
	e.T.Helper()
	path := filepath.Join(e.Root, relativePath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.T.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.T.Fatalf("failed to write file %s: %v", path, err)
	}
}

// WriteConfigRepoFile writes a file relative to the config repo.
func (e *TestEnv) WriteConfigRepoFile(relativePath, content string) {
	e.T.Helper()
	path := filepath.Join(e.ConfigRepo, relativePath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.T.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.T.Fatalf("failed to write file %s: %v", path, err)
	}
}

// WriteTargetFile writes a file relative to the target directory.
func (e *TestEnv) WriteTargetFile(relativePath, content string) {
	e.T.Helper()
	path := filepath.Join(e.TargetDir, relativePath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.T.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.T.Fatalf("failed to write file %s: %v", path, err)
	}
}

// ReadFile reads a file and returns its contents.
func (e *TestEnv) ReadFile(path string) string {
	e.T.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		e.T.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(content)
}

// ReadTargetFile reads a file relative to the target directory.
func (e *TestEnv) ReadTargetFile(relativePath string) string {
	e.T.Helper()
	return e.ReadFile(filepath.Join(e.TargetDir, relativePath))
}

// FileExists checks if a file exists.
func (e *TestEnv) FileExists(path string) bool {
	e.T.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// TargetFileExists checks if a file exists relative to the target directory.
func (e *TestEnv) TargetFileExists(relativePath string) bool {
	e.T.Helper()
	return e.FileExists(filepath.Join(e.TargetDir, relativePath))
}

// SetupTaiConfig writes a tai.yaml configuration file.
func (e *TestEnv) SetupTaiConfig(content string) {
	e.T.Helper()
	e.WriteConfigRepoFile("tai.yaml", content)
}

// SetupBundle writes a bundle file.
func (e *TestEnv) SetupBundle(name, content string) {
	e.T.Helper()
	e.WriteConfigRepoFile(filepath.Join("bundles", name+".yaml"), content)
}

// SetupProfile writes a profile file.
func (e *TestEnv) SetupProfile(name, content string) {
	e.T.Helper()
	e.WriteConfigRepoFile(filepath.Join("profiles", name+".yaml"), content)
}

// SetupAsset writes an asset file.
func (e *TestEnv) SetupAsset(assetType, name, content string) {
	e.T.Helper()
	e.WriteConfigRepoFile(filepath.Join("assets", assetType, name), content)
}

// SetupMinimalConfig sets up a minimal working configuration.
func (e *TestEnv) SetupMinimalConfig() {
	e.T.Helper()

	e.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
default_profile: default
`)

	e.SetupAsset("personas", "default.md", `# Default Persona
You are a helpful assistant.
`)

	e.SetupBundle("core", `name: core
description: Core assets
assets:
  - type: persona
    path: personas/default.md
`)

	e.SetupProfile("default", `name: default
description: Default profile
target: claude
bundles:
  - core
`)
}
