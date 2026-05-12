package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -target amd64 -type tls_fragment -type retprobe_ctx AgentTlsCapture agent_tls_capture.c -- -I.
