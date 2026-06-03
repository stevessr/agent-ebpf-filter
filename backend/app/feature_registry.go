package app

import "agent-ebpf-filter/core"

type FeatureRegistry struct{}

func newFeatureRegistry() *FeatureRegistry {
	return &FeatureRegistry{}
}

func (r *FeatureRegistry) CompiledIn(id FeatureID) bool {
	return isFeatureCompiledIn(id)
}

func (r *FeatureRegistry) Manifest(settings core.RuntimeSettings) FeatureManifestResponse {
	return buildFeatureManifest(settings)
}
