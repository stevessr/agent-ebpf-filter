//go:build agentfeat_otlp

package app

func init() {
	enableBuildFeature(FeatureOTLP)
}
