package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -type lsm_enforcer_stats AgentLsmEnforcer lsm_enforcer.c -- -I.
