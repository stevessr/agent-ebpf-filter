package app

import (
	"agent-ebpf-filter/app/ml"
	"testing"
)

func TestNormalizeModelTuneRequestTypesFiltersByFamilyAndFeature(t *testing.T) {
	got := normalizeModelTuneRequestTypes(ml.MLModelTuneRequest{
		Families:        []string{"tree"},
		FeatureProfiles: []string{"sequence"},
	})
	if len(got) == 0 {
		t.Fatal("expected tree models with sequence features")
	}
	for _, modelType := range got {
		meta := ml.ModelTaxonomy(modelType)
		if meta.Family != "tree" {
			t.Fatalf("unexpected family for %s: %+v", modelType, meta)
		}
		matched := false
		for _, profile := range meta.FeatureProfiles {
			if profile == "sequence" {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("expected sequence feature profile for %s: %+v", modelType, meta)
		}
	}
}

func TestNormalizeModelTuneRequestTypesIntersectsExplicitModels(t *testing.T) {
	got := normalizeModelTuneRequestTypes(ml.MLModelTuneRequest{
		ModelTypes: []string{"random_forest", "logistic_mamba"},
		Families:   []string{"linear"},
	})
	if len(got) != 1 || string(got[0]) != "logistic_mamba" {
		t.Fatalf("expected only logistic_mamba after family intersection, got %v", got)
	}
}

func TestNormalizeModelTuneRequestTypesInvalidExplicitListDoesNotExpandFamily(t *testing.T) {
	got := normalizeModelTuneRequestTypes(ml.MLModelTuneRequest{
		ModelTypes: []string{"not_a_model"},
		Families:   []string{"tree"},
	})
	if len(got) != 0 {
		t.Fatalf("invalid explicit list must stay empty after taxonomy filtering, got %v", got)
	}
}

func TestNormalizeModelTuneRequestTypesDoesNotFallbackOutsideTaxonomyFilter(t *testing.T) {
	got := normalizeModelTuneRequestTypes(ml.MLModelTuneRequest{
		Families: []string{"does_not_exist"},
	})
	if len(got) != 0 {
		t.Fatalf("unknown taxonomy filter must not fall back to unrelated models: %v", got)
	}
}

func TestNormalizeModelTuneRequestTypesKeepsLegacyRecommendedFallback(t *testing.T) {
	got := normalizeModelTuneRequestTypes(ml.MLModelTuneRequest{})
	if len(got) == 0 {
		t.Fatal("empty legacy request should still fall back to recommended models")
	}
	for _, modelType := range got {
		if _, ok := ml.ModelRegistry[modelType]; !ok {
			t.Fatalf("fallback returned unregistered model %s", modelType)
		}
	}
}
