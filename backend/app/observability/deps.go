package observability

import (
	"sync"
	"time"

	"github.com/cilium/ebpf"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
)

// ── Re-export core types ─────────────────────────────────────────────────

type GpuInfo = core.GpuInfo
type VmFaultCounters = core.VmFaultCounters

// ── Dependency interfaces ────────────────────────────────────────────────

type TrackerMapSet interface {
	GetCollectorStats() *ebpf.Map
}

type PersistQueueStatus struct {
	Active         bool
	Stopping       bool
	QueueLen       int
	QueueCap       int
	Pending        uint64
	EnqueuedTotal  uint64
	PersistedTotal uint64
	FailedTotal    uint64
	DroppedTotal   uint64
	LastFlushedAt  string
	LastError      string
}

type SemanticStateStatus struct {
	EntriesByKind                 map[string]int
	Entries                       int
	MaxEntries                    int
	ExpiredEvictionsTotal         uint64
	CapacityEvictionsTotal        uint64
	TruncatedStateValuesTotal     uint64
	IgnoredOversizedMetadataTotal uint64
	LastSweepAt                   string
}

type ToolBaselineStatus struct {
	Tools                     int
	Samples                   int
	MaxTools                  int
	MaxSamples                int
	MaxSamplesPerTool         int
	ObservationsTotal         uint64
	DriftsTotal               uint64
	ExpiredEvictionsTotal     uint64
	CapacityEvictionsTotal    uint64
	TruncatedStateValuesTotal uint64
	LastSweepAt               string
}

type Deps struct {
	TrackerMaps TrackerMapSet

	NvmlInitialized bool

	FdinfoHistory   map[string]uint64
	FdinfoHistoryMu *sync.RWMutex
	FdinfoTime      *time.Time

	LegacyWSClientCount   func() int
	EnvelopeWSClientCount func() int
	PersistQueueStatus    func() PersistQueueStatus
	SemanticStateStatus   func() SemanticStateStatus
	ToolBaselineStatus    func() ToolBaselineStatus
	Broadcast             chan<- *pb.Event
}

var deps Deps

func Init(d Deps) { deps = d }
