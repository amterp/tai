package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/configasset"
)

// mustDecode parses JSON the way the installer does.
func mustDecode(t *testing.T, s string) map[string]any {
	t.Helper()
	doc, err := decodeJSON([]byte(s))
	if err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
	return doc
}

// mustApply applies an edit and returns its manifest record.
func mustApply(t *testing.T, doc map[string]any, e configasset.Edit) configasset.AppliedEdit {
	t.Helper()
	applied, err := applyEdit(doc, e)
	if err != nil {
		t.Fatalf("applyEdit: %v", err)
	}
	return applied
}

// foreignHooks mirrors a real ~/.claude/settings.json: one event array holding
// entries from three different owners, none of them tai.
const foreignHooks = `{
  "hooks": {
    "Stop": [
      { "hooks": [{ "type": "command", "command": "afplay /System/Library/Sounds/Glass.aiff" }] },
      { "matcher": "", "hooks": [{ "type": "command", "command": "/peon-ping/peon.sh", "timeout": 10, "async": true }] }
    ],
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "memri hook claude-code session-start" }] }
    ]
  },
  "statusLine": { "type": "command", "command": "~/.claude/statusline.sh" }
}`

func TestAppendToArrayPreservesForeignEntries(t *testing.T) {
	doc := mustDecode(t, foreignHooks)

	edit := configasset.Edit{
		File:    "settings.json",
		KeyPath: []string{"hooks", "Stop"},
		Mode:    configasset.AppendToArray,
		Value:   map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "tai-hook.sh"}}},
	}
	applied := mustApply(t, doc, edit)

	stop := doc["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 3 {
		t.Fatalf("len(Stop) = %d, want 3 (two foreign entries plus tai's)", len(stop))
	}

	// Removing tai's entry must leave both foreign entries exactly as they were.
	changed, found, err := removeEdit(doc, applied)
	if err != nil {
		t.Fatalf("removeEdit: %v", err)
	}
	if !changed || !found {
		t.Fatalf("removeEdit: changed=%v found=%v, want both true", changed, found)
	}

	after, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	before, err := json.Marshal(mustDecode(t, foreignHooks))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("install+uninstall did not round-trip.\n got: %s\nwant: %s", after, before)
	}
}

func TestRemoveEditLeavesUserModifiedEntryAlone(t *testing.T) {
	doc := mustDecode(t, foreignHooks)

	applied := mustApply(t, doc, configasset.Edit{
		File:    "settings.json",
		KeyPath: []string{"hooks", "Stop"},
		Mode:    configasset.AppendToArray,
		Value:   map[string]any{"command": "original.sh"},
	})

	// The user edits tai's entry after install, taking ownership of it.
	stop := doc["hooks"].(map[string]any)["Stop"].([]any)
	stop[len(stop)-1].(map[string]any)["command"] = "user-edited.sh"

	changed, found, err := removeEdit(doc, applied)
	if err != nil {
		t.Fatalf("removeEdit: %v", err)
	}
	if changed || found {
		t.Errorf("removeEdit: changed=%v found=%v, want both false - a modified entry is the user's now", changed, found)
	}
	if got := len(doc["hooks"].(map[string]any)["Stop"].([]any)); got != 3 {
		t.Errorf("len(Stop) = %d, want 3 - the edited entry should survive", got)
	}
}

func TestUpsertMapEntryPreservesSiblings(t *testing.T) {
	doc := mustDecode(t, `{"mcpServers": {"existing": {"command": "keep-me"}}}`)

	applied := mustApply(t, doc, configasset.Edit{
		File:    "config.json",
		KeyPath: []string{"mcpServers"},
		Mode:    configasset.UpsertMapEntry,
		ID:      "github",
		Value:   map[string]any{"command": "npx"},
	})

	servers := doc["mcpServers"].(map[string]any)
	if len(servers) != 2 {
		t.Fatalf("len(mcpServers) = %d, want 2", len(servers))
	}

	if _, _, err := removeEdit(doc, applied); err != nil {
		t.Fatalf("removeEdit: %v", err)
	}

	servers = doc["mcpServers"].(map[string]any)
	if len(servers) != 1 {
		t.Fatalf("len(mcpServers) = %d, want 1 after removal", len(servers))
	}
	if _, ok := servers["existing"]; !ok {
		t.Error("removing tai's server also removed the pre-existing one")
	}
}

func TestRemoveEditPrunesOnlyWhatTaiEmptied(t *testing.T) {
	// tai owns the only entry under an event it also created.
	doc := mustDecode(t, `{"other": true}`)

	applied := mustApply(t, doc, configasset.Edit{
		File:    "settings.json",
		KeyPath: []string{"hooks", "PreToolUse"},
		Mode:    configasset.AppendToArray,
		Value:   map[string]any{"command": "guard.sh"},
	})

	if _, _, err := removeEdit(doc, applied); err != nil {
		t.Fatalf("removeEdit: %v", err)
	}

	// Both the emptied event array and the "hooks" object tai created go away,
	// but unrelated keys stay.
	if _, exists := doc["hooks"]; exists {
		t.Errorf("hooks key survived with no entries left: %v", doc["hooks"])
	}
	if doc["other"] != true {
		t.Error("unrelated key was pruned")
	}
}

func TestRemoveEditKeepsSharedKeyWithForeignEvents(t *testing.T) {
	doc := mustDecode(t, foreignHooks)

	applied := mustApply(t, doc, configasset.Edit{
		File:    "settings.json",
		KeyPath: []string{"hooks", "PreToolUse"},
		Mode:    configasset.AppendToArray,
		Value:   map[string]any{"command": "guard.sh"},
	})

	if _, _, err := removeEdit(doc, applied); err != nil {
		t.Fatalf("removeEdit: %v", err)
	}

	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks key was deleted even though other tools' events remain")
	}
	if _, exists := hooks["PreToolUse"]; exists {
		t.Error("the event array tai emptied should be pruned")
	}
	if _, exists := hooks["Stop"]; !exists {
		t.Error("another tool's Stop hooks were removed")
	}
}

func TestReplaceKeyRoundTrip(t *testing.T) {
	doc := mustDecode(t, `{"statusLine": {"type": "command", "command": "old.sh"}, "keep": 1}`)

	applied := mustApply(t, doc, configasset.Edit{
		File:    "settings.json",
		KeyPath: []string{"statusLine"},
		Mode:    configasset.ReplaceKey,
		Value:   map[string]any{"style": "detailed"},
	})

	if _, _, err := removeEdit(doc, applied); err != nil {
		t.Fatalf("removeEdit: %v", err)
	}
	if _, exists := doc["statusLine"]; exists {
		t.Error("statusLine should be gone after removing the edit that set it")
	}
	if doc["keep"] == nil {
		t.Error("unrelated key was removed")
	}
}

// Numbers larger than 2^53 lose precision if a config file is decoded into
// float64. ~/.claude.json holds values like this next to entries tai manages,
// so a rewrite must leave them byte-identical.
func TestDecodeEncodePreservesLargeNumbers(t *testing.T) {
	const original = `{
  "bigint": 9007199254740993,
  "timestamp": 1759000000000,
  "float": 1.5,
  "nested": {
    "count": 12345678901234567890
  }
}
`
	doc := mustDecode(t, original)

	encoded, err := encodeJSON(doc)
	if err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}

	for _, literal := range []string{"9007199254740993", "1759000000000", "1.5", "12345678901234567890"} {
		if !strings.Contains(string(encoded), literal) {
			t.Errorf("number %s did not survive the round trip:\n%s", literal, encoded)
		}
	}
}

func TestConfigFileSetSkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"a": 1}`+"\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	set := newConfigFileSet()
	doc, err := set.load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Mark dirty without changing anything - a no-op edit.
	_ = doc
	set.markDirty(path)

	written, err := set.flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("flush wrote %v, want no writes for unchanged content", written)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("unchanged config file was rewritten - every rewrite is a chance to race the target")
	}
}

func TestConfigFileSetCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "settings.json")

	set := newConfigFileSet()
	doc, err := set.load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	doc["statusLine"] = map[string]any{"style": "minimal"}
	set.markDirty(path)

	if _, err := set.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	if !strings.Contains(string(data), "statusLine") {
		t.Errorf("created file missing content: %s", data)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("config file should end with a newline")
	}
}
