package observability

import (
	"sync"
	"time"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"github.com/cilium/ebpf"
	"github.com/gorilla/websocket"
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

	Clients           map[*websocket.Conn]bool
	ClientsMu         *sync.Mutex
	EnvelopeClients   map[*websocket.Conn]bool
	EnvelopeClientsMu *sync.Mutex
	Broadcast         chan<- *pb.Event
}

var deps Deps

func Init(d Deps) { deps = d }