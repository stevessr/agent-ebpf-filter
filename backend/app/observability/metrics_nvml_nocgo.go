//go:build !cgo

package observability

import "agent-ebpf-filter/pb"

func appendNVIDIAMetrics(procMap map[int32]GpuInfo, globalStats *[]*pb.GPUStatus) {}
