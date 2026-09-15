package ml

import "strings"

// ClassifiedBuiltinModelCatalog keeps the existing catalog wire shape while
// normalizing its primary category to the algorithm family. Feature metadata is
// added as namespaced tags for backward-compatible clients; newer clients can
// also consume BuiltinModelTaxonomy for structured fields.
func ClassifiedBuiltinModelCatalog() []BuiltinModelCatalogItem {
	items := BuiltinModelCatalog()
	for i := range items {
		meta := ModelTaxonomy(ModelType(items[i].Value))
		items[i].Category = meta.FamilyLabel
		items[i].Tags = mergeTaxonomyTags(items[i].Tags, meta)
	}
	return items
}

func mergeTaxonomyTags(existing []string, meta ModelTaxonomyItem) []string {
	out := make([]string, 0, len(existing)+len(meta.FeatureProfiles)+2)
	seen := make(map[string]struct{}, len(existing)+len(meta.FeatureProfiles)+2)
	appendUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, tag := range existing {
		appendUnique(tag)
	}
	appendUnique("family:" + meta.Family)
	appendUnique("feature:" + meta.FeatureClass)
	for _, profile := range meta.FeatureProfiles {
		appendUnique("feature-profile:" + profile)
	}
	return out
}
