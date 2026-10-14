package configasset

import (
	"reflect"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
)

func TestFieldEditsOwnsTheFinestGrain(t *testing.T) {
	fields := map[string]any{
		"defaultMode": "auto",
		"env":         map[string]any{"A": "1"},
		"sandbox":     map[string]any{"network": map[string]any{"allow": true}},
		"empty":       map[string]any{},
		"permissions": map[string]any{"allow": []any{"x", "y"}},
		"weird":       map[any]any{1: "one"},
	}

	got := FieldEdits("f.json", nil, fields, "owner")
	want := []Edit{
		{File: "f.json", KeyPath: []string{"defaultMode"}, Mode: ReplaceKey, Value: "auto"},
		{File: "f.json", KeyPath: []string{"empty"}, Mode: ReplaceKey, Value: map[string]any{}},
		{File: "f.json", KeyPath: []string{"env"}, Mode: UpsertMapEntry, ID: "A", Value: "1"},
		{File: "f.json", KeyPath: []string{"permissions", "allow"}, Mode: AppendToArray, ID: "owner[0]", Value: "x"},
		{File: "f.json", KeyPath: []string{"permissions", "allow"}, Mode: AppendToArray, ID: "owner[1]", Value: "y"},
		{File: "f.json", KeyPath: []string{"sandbox", "network"}, Mode: UpsertMapEntry, ID: "allow", Value: true},
		{File: "f.json", KeyPath: []string{"weird"}, Mode: UpsertMapEntry, ID: "1", Value: "one"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FieldEdits =\n%#v\nwant\n%#v", got, want)
	}
}

func TestFieldEditsRootsBelowBase(t *testing.T) {
	got := FieldEdits("f.json", []string{"autoMode"}, map[string]any{"allow": []any{"x"}, "mode": "on"}, "policy")
	want := []Edit{
		{File: "f.json", KeyPath: []string{"autoMode", "allow"}, Mode: AppendToArray, ID: "policy[0]", Value: "x"},
		{File: "f.json", KeyPath: []string{"autoMode"}, Mode: UpsertMapEntry, ID: "mode", Value: "on"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FieldEdits =\n%#v\nwant\n%#v", got, want)
	}
}

func TestParseSettingsRejectsFieldsOutsideNative(t *testing.T) {
	_, err := config.ParseSettingsConfig([]byte("env: {A: \"1\"}\n"))
	if err == nil || !strings.Contains(err.Error(), "native.<target>") {
		t.Fatalf("err = %v, want one pointing at native.<target>", err)
	}
}

func TestNamesTarget(t *testing.T) {
	content := []byte("native: {claude: {env: {A: \"1\"}}}\n")
	if !NamesTarget(content, "claude") || NamesTarget(content, "codex") {
		t.Error("NamesTarget should be true for claude only")
	}
}
