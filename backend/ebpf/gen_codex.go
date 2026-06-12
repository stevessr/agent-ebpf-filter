package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64,arm64 -type codex_syscall_event CodexSyscallTracker codex_syscall_tracker.c -- -I../
