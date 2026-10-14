package main

import (
	"strings"
	"testing"

	"github.com/amterp/tai/internal/state"
)

func TestOverrideRefusalTellsHowToRunTheCommand(t *testing.T) {
	tests := []struct {
		origin, command string
		want, notWant   []string
	}{
		{
			origin:  "--config",
			command: "install",
			want:    []string{"'tai install'", "/repo (from --config)", "drop --config", "'tai config set /repo'"},
		},
		{
			origin:  "TAI_CONFIG",
			command: "install",
			want:    []string{"from TAI_CONFIG", "unset TAI_CONFIG"},
			notWant: []string{"drop --config"},
		},
		{
			// Suggesting 'tai config set' as the way to run 'tai config set' is circular.
			origin:  "TAI_CONFIG",
			command: "config set",
			want:    []string{"unset TAI_CONFIG"},
			notWant: []string{"make that repo active"},
		},
	}
	for _, tt := range tests {
		msg := overrideRefusal(&state.RepoOverride{Path: "/repo", Origin: tt.origin}, tt.command, overrideWritesManifests)
		for _, w := range tt.want {
			if !strings.Contains(msg, w) {
				t.Errorf("%s/%s: message missing %q:\n%s", tt.origin, tt.command, w, msg)
			}
		}
		for _, nw := range tt.notWant {
			if strings.Contains(msg, nw) {
				t.Errorf("%s/%s: message should not contain %q:\n%s", tt.origin, tt.command, nw, msg)
			}
		}
	}
}
