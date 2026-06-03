//go:build agentfeat_hooks

package app

func init() {
	enableBuildFeature(FeatureHooks)
}
