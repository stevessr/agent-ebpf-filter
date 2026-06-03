//go:build !agentfeat_all && !agentfeat_core && !agentfeat_shell_sessions && !agentfeat_system_run && !agentfeat_hooks && !agentfeat_policy_management && !agentfeat_tls_capture && !agentfeat_otlp && !agentfeat_domain_forward && !agentfeat_ml && !agentfeat_plugins && !agentfeat_sandbox_cgroup && !agentfeat_sandbox_lsm && !agentfeat_network_export && !agentfeat_agentsight

package app

func init() {
	enableAllBuildFeatures()
}
