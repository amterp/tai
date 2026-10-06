package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultCommandTimeout is the default timeout for command-based includes
// when the include doesn't specify one.
const DefaultCommandTimeout = 30 * time.Second

// SkillManifest represents a skill's tai.yaml build configuration.
// It declares external file dependencies that should be resolved at install time.
type SkillManifest struct {
	Include []Include `yaml:"include"`
}

// Include represents an external file dependency for a skill.
// Exactly one of (From+Path), URL, or Command must be set.
type Include struct {
	// Source-based include (mutually exclusive with URL and Command)
	From string `yaml:"from"` // Named source from config's tai.yaml
	Path string `yaml:"path"` // Path within the source

	// URL-based include (mutually exclusive with From and Command)
	URL string `yaml:"url"`

	// Command-based include (mutually exclusive with From and URL).
	// The command is executed with a tai-provided staging directory (via the
	// TAI_DEST_DIR env var); any files it writes there are atomically swapped
	// into Dest on success.
	Command []string `yaml:"command"`

	// Timeout for command-based includes, parsed via time.ParseDuration.
	// Defaults to DefaultCommandTimeout when unset. Ignored for
	// non-command includes.
	Timeout string `yaml:"timeout"`

	// Destination within the skill directory.
	// If ends with "/", preserves the original filename/dirname.
	// Otherwise, uses the exact path specified.
	Dest string `yaml:"dest"`

	// Fields maps tai field names to source field names for frontmatter remapping.
	// For example, {"name": "title"} means the source file's "title" field
	// should be treated as tai's "name" field.
	Fields map[string]string `yaml:"fields"`
}

// Validate checks that the include is well-formed.
func (i *Include) Validate() error {
	hasFrom := i.From != ""
	hasPath := i.Path != ""
	hasURL := i.URL != ""
	hasCommand := len(i.Command) > 0

	// Enforce mutual exclusion across the three modes.
	if hasCommand && (hasFrom || hasPath || hasURL) {
		return fmt.Errorf("include cannot mix 'command' with 'url' or 'from'/'path'")
	}
	if hasURL && (hasFrom || hasPath) {
		return fmt.Errorf("include cannot have both 'url' and 'from'/'path'")
	}

	switch {
	case hasCommand:
		// Command-based include
		if err := validateCommandArgv(i.Command, "command"); err != nil {
			return fmt.Errorf("include %w", err)
		}
		if i.Dest == "" {
			return fmt.Errorf("include with 'command' requires 'dest'")
		}
		// A command's output is a directory, so this dest would install
		// SKILL.md/SKILL.md - never what someone writing it means.
		if filepath.Clean(i.Dest) == "SKILL.md" {
			return fmt.Errorf("include 'dest' %q would make SKILL.md a directory: a command include writes a directory. "+
				"To generate SKILL.md, use dest: ./ and write $TAI_DEST_DIR/SKILL.md", i.Dest)
		}
		// 'fields' is a frontmatter-remap feature for source-based includes
		// of SKILL.md - meaningless and silently ignored otherwise.
		if len(i.Fields) > 0 {
			return fmt.Errorf("include 'fields' is only valid with 'from'+'path', not 'command'")
		}
		if err := validateTimeoutString(i.Timeout, "timeout"); err != nil {
			return fmt.Errorf("include %w", err)
		}
	case hasURL:
		// URL-based include
		if i.Dest == "" {
			return fmt.Errorf("include with 'url' requires 'dest'")
		}
	case hasFrom || hasPath:
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
	default:
		return fmt.Errorf("include must have one of 'from'+'path', 'url', or 'command'")
	}

	// Timeout is meaningful only for command includes.
	if !hasCommand && i.Timeout != "" {
		return fmt.Errorf("include 'timeout' is only valid with 'command'")
	}

	return nil
}

// IsURLBased returns true if this is a URL-based include.
func (i *Include) IsURLBased() bool {
	return i.URL != ""
}

// IsCommandBased returns true if this is a command-based include.
func (i *Include) IsCommandBased() bool {
	return len(i.Command) > 0
}

// ResolvedTimeout returns the parsed timeout for command-based includes, or
// DefaultCommandTimeout when Timeout is empty. Validate guarantees the
// string parses cleanly, so this never returns an error path; if Validate was
// skipped and Timeout is malformed, the default is returned instead.
func (i *Include) ResolvedTimeout() time.Duration {
	return resolveTimeout(i.Timeout)
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
