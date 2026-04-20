package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang AgentTracker agent_tracker.c -- -I.
