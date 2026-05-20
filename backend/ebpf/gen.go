package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang AgentTracker agent_tracker.c -- -I. -I/usr/include/x86_64-linux-gnu -I/usr/include/aarch64-linux-gnu -Wno-missing-declarations
