package ml

import (
	"agent-ebpf-filter/core"
	"testing"
)

func TestBuiltinModelTaxonomyCoversAllRegisteredModelTypes(t *testing.T) {
	items := BuiltinModelTaxonomy()
	types := AllModelTypes()
	if len(items) != len(types) {
		t.Fatalf("taxonomy size=%d model types=%d", len(items), len(types))
	}

	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ModelType == "" || item.Family == "" || item.FamilyLabel == "" || item.FeatureClass == "" || item.FeatureClassLabel == "" {
			t.Fatalf("incomplete taxonomy item: %+v", item)
		}
		if item.Family == string(ModelFamilyOther) {
			t.Fatalf("built-in model left uncategorized: %+v", item)
		}
		if len(item.FeatureProfiles) == 0 {
			t.Fatalf("missing feature profiles: %+v", item)
		}
		if len(item.FeatureSources) != len(baseFeatureSources) {
			t.Fatalf("unexpected feature source count for %s: %v", item.ModelType, item.FeatureSources)
		}
		if seen[item.ModelType] {
			t.Fatalf("duplicate taxonomy entry for %s", item.ModelType)
		}
		seen[item.ModelType] = true
	}
}

func TestModelTaxonomySeparatesClassifierFamilyFromFeatureRepresentation(t *testing.T) {
	rfMamba := ModelTaxonomy(ModelRandomForestMamba)
	if rfMamba.Family != string(ModelFamilyTree) {
		t.Fatalf("RF + Mamba must remain in tree family: %+v", rfMamba)
	}
	if rfMamba.FeatureClass != string(ModelFeatureSequenceState) {
		t.Fatalf("RF + Mamba must expose state-space feature class: %+v", rfMamba)
	}

	ngramLogistic := ModelTaxonomy(core.ModelNGramLogistic)
	if ngramLogistic.Family != string(ModelFamilyLinear) {
		t.Fatalf("N-Gram Logistic must remain in linear family: %+v", ngramLogistic)
	}
	if ngramLogistic.FeatureClass != string(ModelFeatureSequenceNGram) {
		t.Fatalf("N-Gram Logistic must expose N-Gram feature class: %+v", ngramLogistic)
	}

	graph := ModelTaxonomy(core.ModelGraphLearning)
	if graph.Family != string(ModelFamilyGraph) || graph.FeatureClass != string(ModelFeatureGraphContext) {
		t.Fatalf("graph model taxonomy mismatch: %+v", graph)
	}
}

func TestModelTypesByTaxonomyFiltersFamilyAndFeature(t *testing.T) {
	treeSequence := ModelTypesByTaxonomy(
		[]string{string(ModelFamilyTree)},
		[]string{string(ModelFeatureSequenceState)},
	)
	if len(treeSequence) == 0 {
		t.Fatal("expected tree + sequence models")
	}
	for _, modelType := range treeSequence {
		meta := ModelTaxonomy(modelType)
		if meta.Family != string(ModelFamilyTree) || meta.FeatureClass != string(ModelFeatureSequenceState) {
			t.Fatalf("unexpected tree-sequence match %s: %+v", modelType, meta)
		}
	}

	ngram := ModelTypesByTaxonomy(nil, []string{"ngram"})
	if len(ngram) != 3 {
		t.Fatalf("expected three N-Gram models, got %d: %v", len(ngram), ngram)
	}
}
