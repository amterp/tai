package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/config"
)

// ResolveFileDest resolves the target-relative path for a file-type asset
// based on its Dest field. The rules:
//   - Empty dest: root level with original name
//   - Trailing "/": explicit directory, appends source filename
//   - Basename contains ".": treated as full file path
//   - Otherwise: treated as directory, appends source filename
func ResolveFileDest(asset *config.Asset) string {
	if asset.Dest == "" {
		return asset.Name
	}
	if strings.HasSuffix(asset.Dest, "/") {
		return filepath.Join(strings.TrimSuffix(asset.Dest, "/"), asset.Name)
	}
	if strings.Contains(filepath.Base(asset.Dest), ".") {
		return asset.Dest
	}
	return filepath.Join(asset.Dest, asset.Name)
}

// DefaultMapAssetPath maps a tai asset to a target path using the provided
// asset-type-to-directory mapping. File-type assets use ResolveFileDest.
// Unknown types fall back to type/name.
func DefaultMapAssetPath(mapping map[string]string, asset *config.Asset) string {
	if asset.Type == config.AssetTypeFile {
		return ResolveFileDest(asset)
	}

	targetType, ok := mapping[asset.Type]
	if !ok {
		return filepath.Join(asset.Type, asset.Name)
	}

	return filepath.Join(targetType, asset.Name)
}

// RunBinary executes a CLI binary with the given arguments, connecting
// stdin/stdout/stderr to the current process.
func RunBinary(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
