package ml

// ClassifiedBuiltinModelCatalog keeps the existing catalog wire shape while
// normalizing its primary category to the algorithm family. Detailed feature
// taxonomy is exposed separately by BuiltinModelTaxonomy so legacy clients do
// not render internal taxonomy metadata as ordinary tags.
func ClassifiedBuiltinModelCatalog() []BuiltinModelCatalogItem {
	items := BuiltinModelCatalog()
	for i := range items {
		meta := ModelTaxonomy(ModelType(items[i].Value))
		items[i].Category = meta.FamilyLabel
	}
	return items
}
