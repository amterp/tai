package install

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	"github.com/amterp/tai/internal/testutil"
)

// minimalConfigAsset is a valid, installable asset of each config type, so the
// invariant below exercises a real Plan rather than a validation failure.
var minimalConfigAsset = map[string]string{
	config.AssetTypeStatusLine: "level: custom\ncommand: echo hi\n",
	config.AssetTypeHook:       "on: session_start\nrun: echo hi\n",
	config.AssetTypeMCP:        "transport: stdio\ncommand: npx\n",
	config.AssetTypeSettings:   "native: {%s: {demo: true}}\n",
	config.AssetTypeApproval:   "## allow: Probe\nAllowed.\n",
}

// An adapter declares which config asset types it supports, and separately
// implements an optional translator interface per type. Nothing makes those two
// agree at compile time, so a target that claims a type but forgets its
// translator fails at install time with "does not support" - confusing, and
// only caught today by each adapter happening to have a hand-written test.
//
// This checks the invariant directly, for every adapter now and later.
func TestAdaptersImplementWhatTheyClaimToSupport(t *testing.T) {
	for _, name := range adapter.DefaultRegistry.List() {
		t.Run(name, func(t *testing.T) {
			env := testutil.NewTestEnv(t)
			adap, err := adapter.DefaultRegistry.Get(name)
			if err != nil {
				t.Fatalf("building adapter: %v", err)
			}

			for _, assetType := range adap.SupportedConfigAssetTypes() {
				handler, ok := configasset.Lookup(assetType)
				if !ok {
					t.Errorf("claims support for %q, which no handler is registered for", assetType)
					continue
				}

				content, ok := minimalConfigAsset[assetType]
				if !ok {
					t.Fatalf("no minimal asset defined for config type %q - add one", assetType)
				}

				if assetType == config.AssetTypeSettings {
					content = fmt.Sprintf(content, name)
				}
				plan, err := handler.Plan(configasset.Request{
					Adapter: adap,
					Asset: &config.Asset{
						Type:       assetType,
						Name:       "probe.yaml",
						SourcePath: filepath.Join(env.ConfigRepo, "probe.yaml"),
					},
					Content: []byte(content),
					Scope:   config.DefaultScope,
					Name:    "probe",
				})
				if err != nil {
					t.Errorf("cannot plan supported asset type %q: %v", assetType, err)
				} else if len(plan.Edits) == 0 {
					t.Errorf("supported asset type %q produced no edits", assetType)
				}
			}
		})
	}
}

func TestAdaptersMapLooseFiles(t *testing.T) {
	for _, name := range adapter.DefaultRegistry.List() {
		t.Run(name, func(t *testing.T) {
			adap, err := adapter.DefaultRegistry.Get(name)
			if err != nil {
				t.Fatal(err)
			}
			vars := adap.ProvideVariables()
			if vars["target_name"] != name {
				t.Errorf("target_name = %q, want %q", vars["target_name"], name)
			}
			instructions := vars["target_agents_file"]
			if instructions == "" {
				t.Fatalf("missing instructions filename: %v", vars)
			}
			asset := &config.Asset{Type: config.AssetTypeFile, Name: "instructions.md", Dest: instructions}
			if path := adap.MapAssetPath(asset); path != instructions {
				t.Errorf("instructions install to %q, want %q", path, instructions)
			}
		})
	}
}

// The mirror of the above: a target that quietly implements a translator it
// never declares would install nothing, with the capability invisible.
func TestAdaptersDeclareEveryTranslatorTheyImplement(t *testing.T) {
	implements := map[string]func(adapter.Adapter) bool{
		config.AssetTypeStatusLine: func(a adapter.Adapter) bool {
			_, ok := a.(configasset.StatusLineTranslator)
			return ok
		},
		config.AssetTypeHook: func(a adapter.Adapter) bool {
			_, ok := a.(configasset.HookTranslator)
			return ok
		},
		config.AssetTypeMCP: func(a adapter.Adapter) bool {
			_, ok := a.(configasset.MCPTranslator)
			return ok
		},
	}

	for _, name := range adapter.DefaultRegistry.List() {
		t.Run(name, func(t *testing.T) {
			adap, err := adapter.DefaultRegistry.Get(name)
			if err != nil {
				t.Fatalf("building adapter: %v", err)
			}

			declared := make(map[string]bool)
			for _, assetType := range adap.SupportedConfigAssetTypes() {
				declared[assetType] = true
			}

			for assetType, hasTranslator := range implements {
				if hasTranslator(adap) && !declared[assetType] {
					t.Errorf("implements the %q translator but omits it from SupportedConfigAssetTypes", assetType)
				}
			}
		})
	}
}
