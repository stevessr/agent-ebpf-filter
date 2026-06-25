package platform

import "github.com/cilium/ebpf"

// CloseMapHandles closes all non-nil maps in the given collection.
// This is a shared utility used by sandbox and runtime eBPF code.
func CloseMapHandles(maps map[string]*ebpf.Map) {
	for _, m := range maps {
		if m != nil {
			_ = m.Close()
		}
	}
}
