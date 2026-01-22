package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SkillManifest represents a skill's tai.yaml build configuration.
// It declares external file dependencies that should be resolved at install time.
type SkillManifest struct {
	Include []Include `yaml:"include"`
}

// Include represents an external file dependency for a skill.
// Either From+Path (source-based) or URL (URL-based) must be set, not both.
type Include struct {
	// Source-based include (mutually exclusive with URL)
	From string `yaml:"from"` // Named source from config's tai.yaml
	Path string `yaml:"path"` // Path within the source

	// URL-based include (mutually exclusive with From)
	URL string `yaml:"url"`

	// Destination within the skill directory.
	// If ends with "/", preserves the original filename/dirname.
	// Otherwise, uses the exact path specified.
	Dest string `yaml:"dest"`
}

// Validate checks that the include is well-formed.
func (i *Include) Validate() error {
	hasFrom := i.From != ""
	hasPath := i.Path != ""
	hasURL := i.URL != ""

	// Must have either from+path or url, not both, not neither
	if hasURL && (hasFrom || hasPath) {
		return fmt.Errorf("include cannot have both 'url' and 'from'/'path'")
	}

	if hasURL {
		// URL-based include
		if i.Dest == "" {
			return fmt.Errorf("include with 'url' requires 'dest'")
		}
	} else if hasFrom || hasPath {
		// Source-based include - both from and path are required
		if !hasFrom {
			return fmt.Errorf("include with 'path' requires 'from'")
		}
		if !hasPath {
			return fmt.Errorf("include with 'from' requires 'path'")
		}
		if i.Dest == "" {
			return fmt.Errorf("include with 'from' requires 'dest'")
		}
	} else {
		return fmt.Errorf("include must have either 'from'+'path' or 'url'")
	}

	return nil
}

// IsURLBased returns true if this is a URL-based include.
func (i *Include) IsURLBased() bool {
	return i.URL != ""
}

// Validate checks that the skill manifest is well-formed.
func (m *SkillManifest) Validate() error {
	for idx, inc := range m.Include {
		if err := inc.Validate(); err != nil {
			return fmt.Errorf("include[%d]: %w", idx, err)
		}
	}
	return nil
}

// LoadSkillManifest loads tai.yaml from a skill directory if it exists.
// Returns (nil, nil) if the manifest doesn't exist (it's optional).
// Returns an error if the file exists but is malformed.
func LoadSkillManifest(skillPath string) (*SkillManifest, error) {
	manifestPath := filepath.Join(skillPath, "tai.yaml")

	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return nil, nil // No manifest - skill will install without includes
	}
	if err != nil {
		return nil, fmt.Errorf("reading skill manifest: %w", err)
	}

	var manifest SkillManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing skill manifest: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid skill manifest: %w", err)
	}

	return &manifest, nil
}
