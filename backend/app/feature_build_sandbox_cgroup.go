//go:build agentfeat_sandbox_cgroup

package app

func init() {
	enableBuildFeature(FeatureSandboxCgroup)
}
