package main

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitRunArgs(t *testing.T) {
	tests := []struct {
		args       string
		wantTai    string
		wantTarget string
	}{
		// Other commands are left alone, including a --config value that
		// happens to be "run".
		{"list profiles --no-color", "list profiles --no-color", ""},
		{"--config X list profiles", "--config X list profiles", ""},
		{"--config run list profiles", "--config run list profiles", ""},

		{"run claude", "run claude", ""},
		{"run claude --model x --resume", "run claude", "--model x --resume"},
		{"run claude -- --model x", "run claude", "--model x"},
		{"run claude --help", "run claude", "--help"},
		{"run claude -h", "run claude", "-h"},

		// Only the "--" right after the target is tai's.
		{"run claude -- -- x", "run claude", "-- x"},
		{"run claude x -- y", "run claude", "x -- y"},

		// tai's flags before the target stay tai's; after it they are the target's.
		{"run codex --config k=v", "run codex", "--config k=v"},
		{"--config X run claude", "--config X run claude", ""},
		{"run --config X claude --resume", "run --config X claude", "--resume"},
		{"run --config=X claude", "run --config=X claude", ""},
		{"run --no-color claude --no-color", "run --no-color claude", "--no-color"},

		{"run -- claude --x", "run -- claude", "--x"},

		// No target: Ra shows help or reports the missing target.
		{"run", "run", ""},
		{"run --help", "run --help", ""},
		{"run --config", "run --config", ""},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			gotTai, gotTarget := splitRunArgs(strings.Fields(tt.args))
			if want := strings.Fields(tt.wantTai); !slices.Equal(gotTai, want) {
				t.Errorf("tai args = %q, want %q", gotTai, want)
			}
			if want := strings.Fields(tt.wantTarget); !slices.Equal(gotTarget, want) {
				t.Errorf("target args = %q, want %q", gotTarget, want)
			}
		})
	}
}
