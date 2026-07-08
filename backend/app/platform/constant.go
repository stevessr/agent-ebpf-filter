package platform

// eBPF pin root constants (shared across subpackages)
const (
	EBPFPinRoot     = "/sys/fs/bpf/agent-ebpf"
	EBPFPinMapsDir  = EBPFPinRoot + "/maps"
	EBPFPinLinksDir = EBPFPinRoot + "/links"
)