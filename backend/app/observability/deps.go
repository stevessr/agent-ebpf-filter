package observability

import (
	"sync"
	"time"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"github.com/cilium/ebpf"
)

// ── Re-export core types ─────────────────────────────────────────────────

type GpuInfo = core.GpuInfo
type VmFaultCounters = core.VmFaultCounters

// ── Dependency interfaces ────────────────────────────────────────────────

type TrackerMapSet interface {
	GetCollectorStats() *ebpf.Map
}

type Deps struct {
	TrackerMaps TrackerMapSet

	NvmlInitialized bool

	FdinfoHistory   map[string]uint64
	FdinfoHistoryMu *sync.RWMutex
	FdinfoTime      *time.Time

	LegacyWSClientCount   func() int
	EnvelopeWSClientCount func() int
	Broadcast             chan<- *pb.Event
}

var deps Deps

func Init(d Deps) { deps = d }
