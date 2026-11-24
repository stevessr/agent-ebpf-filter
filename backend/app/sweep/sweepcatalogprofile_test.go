package sweep

import (
	"testing"

	"agent-ebpf-filter/app/ml"
)

func TestModelCatalogProfileCoversEveryBuiltinModel(t *testing.T) {
	profile := modelCatalogProfile()
	if len(profile.XValues) != len(ml.BuiltinModelProfiles) {
		t.Fatalf("catalog profile covers %d models, want %d", len(profile.XValues), len(ml.BuiltinModelProfiles))
	}

	seen := make(map[ModelType]bool, len(ml.BuiltinModelProfiles))
	for i, x := range profile.XValues {
		cfg := profile.Build(x, 0)
		if cfg.ModelType == "" {
			t.Fatalf("profile.Build(%d) produced an empty model type", x)
		}
		if seen[cfg.ModelType] {
			t.Fatalf("model %s appears twice in the catalog profile", cfg.ModelType)
		}
		seen[cfg.ModelType] = true
		if cfg.LlmEnabled {
			t.Fatalf("catalog sweep must not enable the LLM path (%s)", cfg.ModelType)
		}
		if cfg.ValidationSplitRatio != 0.2 {
			t.Fatalf("%s split ratio = %v, want the sweep-wide 0.2", cfg.ModelType, cfg.ValidationSplitRatio)
		}
		label := profile.XLabel(x)
		if label != string(cfg.ModelType) {
			t.Fatalf("x=%d labels %q but built %q", x, label, cfg.ModelType)
		}
		if i != x {
			t.Fatalf("catalog x-values must be dense indices, got %d at position %d", x, i)
		}
	}
	for _, builtin := range ml.BuiltinModelProfiles {
		if !seen[builtin.Type] {
			t.Fatalf("builtin model %s is missing from the catalog profile", builtin.Type)
		}
	}

	full := profilesForModeWithPoints("full", 1000)
	if !containsProfile(full, "model_catalog") {
		t.Fatal("full mode must include the model_catalog profile")
	}
	quick := profilesForModeWithPoints("quick", 1000)
	if containsProfile(quick, "model_catalog") {
		t.Fatal("quick mode must stay small; model_catalog belongs to full/comprehensive sweeps")
	}
	comprehensive := profilesForModeWithPoints("comprehensive", 1000)
	if !containsProfile(comprehensive, "model_catalog") {
		t.Fatal("comprehensive mode must include the model_catalog profile")
	}
}

func containsProfile(profiles []sweepProfile, name string) bool {
	for _, profile := range profiles {
		if profile.Name == name {
			return true
		}
	}
	return false
}
