package template

import (
	"fmt"
	"os"

	pongo2 "github.com/amterp/pongo2/v6"
)

// registerTaiFilters registers tai's custom filters on a TemplateSet.
// In pongo2 v7, filters are per-set rather than global.
func registerTaiFilters(set *pongo2.TemplateSet) {
	if err := set.RegisterFilter("env", filterEnv); err != nil {
		panic(fmt.Sprintf("failed to register env filter: %v", err))
	}
}

// filterEnv looks up an environment variable by name.
// Usage: {{ "HOME"|env }} returns the value of $HOME.
// Returns empty string if the environment variable is not set.
func filterEnv(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, error) {
	envName := in.String()
	if envName == "" {
		return nil, fmt.Errorf("env filter requires a non-empty string")
	}
	return pongo2.AsValue(os.Getenv(envName)), nil
}
