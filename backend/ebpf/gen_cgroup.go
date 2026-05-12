package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang AgentCgroupSandbox cgroup_sandbox.c -- -I.
