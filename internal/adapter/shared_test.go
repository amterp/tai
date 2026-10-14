package adapter

import (
	"testing"

	"github.com/amterp/tai/internal/config"
)

func TestResolveFileDest(t *testing.T) {
	tests := []struct {
		name  string
		asset *config.Asset
		want  string
	}{
		{"empty dest is target root", &config.Asset{Name: "CLAUDE.md"}, "CLAUDE.md"},
		{"no dot is a directory", &config.Asset{Name: "plugin.json", Dest: "hooks"}, "hooks/plugin.json"},
		{"trailing slash is a directory", &config.Asset{Name: "plugin.json", Dest: "out/"}, "out/plugin.json"},
		{"extension is a file path", &config.Asset{Name: "AGENTS.md", Dest: "CLAUDE.md"}, "CLAUDE.md"},
		{"dotfile is a file path", &config.Asset{Name: ".gitignore", Dest: ".gitignore"}, ".gitignore"},
		// Same shape as .gitignore, so it stays a file path; the installer
		// warns about this one instead of guessing.
		{"dot-name is a file path", &config.Asset{Name: "plugin.json", Dest: "skills/x/.claude-plugin"}, "skills/x/.claude-plugin"},
		{"dot-name with trailing slash is a directory", &config.Asset{Name: "plugin.json", Dest: "skills/x/.claude-plugin/"}, "skills/x/.claude-plugin/plugin.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveFileDest(tt.asset); got != tt.want {
				t.Errorf("ResolveFileDest() = %q, want %q", got, tt.want)
			}
		})
	}
}
