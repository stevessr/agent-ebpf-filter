package app

var compiledFeatureIDs = map[FeatureID]bool{}

func enableBuildFeature(id FeatureID) {
	compiledFeatureIDs[id] = true
}

func enableAllBuildFeatures() {
	for _, id := range optionalFeatureIDs {
		enableBuildFeature(id)
	}
}

func isFeatureCompiledIn(id FeatureID) bool {
	return compiledFeatureIDs[id]
}
