// Package adapter provides the interface for target-specific adapters.
package adapter

import (
	"fmt"

	"github.com/amterp/tai/internal/config"
)

// Adapter defines the interface for target-specific adapters.
type Adapter interface {
	// Name returns the adapter name (e.g., "claude", "cursor")
	Name() string

	// AssetMapping returns how tai asset types map to target paths.
	// e.g., "personas" -> "output-styles", "delegates" -> "agents"
	AssetMapping() map[string]string

	// TargetDir returns the base directory where this target's config lives.
	// e.g., ~/.claude
	TargetDir() string

	// MapAssetPath maps a tai asset path to a target path.
	// e.g., "personas/senior-swe.md" -> "output-styles/senior-swe.md"
	MapAssetPath(asset *config.Asset) string

	// Run invokes the target with the given arguments.
	Run(args []string) error
}

// Registry holds all registered adapters.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry creates a new adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the registry.
func (r *Registry) Register(adapter Adapter) {
	r.adapters[adapter.Name()] = adapter
}

// Get returns an adapter by name.
func (r *Registry) Get(name string) (Adapter, error) {
	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("unknown target: %s", name)
	}
	return adapter, nil
}

// List returns all registered adapter names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the global registry with all built-in adapters.
// Adapters register themselves via init().
var DefaultRegistry = NewRegistry()
