package tls

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"agent-ebpf-filter/app/bpfts"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

var ErrBpfTSTLSShadowAlreadyActive = errors.New("bpf-ts TLS shadow runtime is already active")

type BpfTSTLSShadowConfig struct {
	ObjectPath   string `json:"objectPath"`
	ManifestPath string `json:"manifestPath"`
	TargetPath   string `json:"targetPath"`
	PID          int    `json:"pid,omitempty"`
}

type BpfTSTLSShadowRingStats struct {
	ReaderActive bool   `json:"readerActive"`
	Records      uint64 `json:"records"`
	Bytes        uint64 `json:"bytes"`
	ReadErrors   uint64 `json:"readErrors"`
	LastRecordNS int64  `json:"lastRecordNs,omitempty"`
}

type BpfTSTLSShadowStatus struct {
	Active       bool                               `json:"active"`
	Healthy      bool                               `json:"healthy"`
	Source       string                             `json:"source,omitempty"`
	ProbeCount   int                                `json:"probeCount"`
	MapCount     int                                `json:"mapCount"`
	RingbufCount int                                `json:"ringbufCount"`
	StartedAt    string                             `json:"startedAt,omitempty"`
	LastError    string                             `json:"error,omitempty"`
	Ringbufs     map[string]BpfTSTLSShadowRingStats `json:"ringbufs,omitempty"`
}

type bpfTSTLSShadowLoadedRuntime interface {
	Close() error
	Map(name string) *ebpf.Map
}

type bpfTSTLSShadowLoader func(string, bpfts.Manifest, bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error)

type bpfTSTLSShadowRingReader interface {
	Read() (ringbuf.Record, error)
	Close() error
}

type bpfTSTLSShadowRingFactory func(*ebpf.Map) (bpfTSTLSShadowRingReader, error)

type bpfTSTLSShadowCounters struct {
	readerActive atomic.Bool
	records      atomic.Uint64
	bytes        atomic.Uint64
	readErrors   atomic.Uint64
	lastRecordNS atomic.Int64
}

type BpfTSTLSShadowRuntime struct {
	transitionMu sync.Mutex
	mu           sync.RWMutex
	wg           sync.WaitGroup

	loader        bpfTSTLSShadowLoader
	readerFactory bpfTSTLSShadowRingFactory

	runtime   bpfTSTLSShadowLoadedRuntime
	readers   map[string]bpfTSTLSShadowRingReader
	counters  map[string]*bpfTSTLSShadowCounters
	manifest  bpfts.Manifest
	config    BpfTSTLSShadowConfig
	startedAt time.Time
	active    bool
	lastError string
}

func NewBpfTSTLSShadowRuntime() *BpfTSTLSShadowRuntime {
	return newBpfTSTLSShadowRuntime(
		func(objectPath string, manifest bpfts.Manifest, options bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			return bpfts.LoadAndAttach(objectPath, manifest, options)
		},
		func(m *ebpf.Map) (bpfTSTLSShadowRingReader, error) {
			return ringbuf.NewReader(m)
		},
	)
}

func newBpfTSTLSShadowRuntime(loader bpfTSTLSShadowLoader, readerFactory bpfTSTLSShadowRingFactory) *BpfTSTLSShadowRuntime {
	return &BpfTSTLSShadowRuntime{loader: loader, readerFactory: readerFactory}
}

func isBpfTSTLSShadowTarget(target string) bool {
	if target == "SSL_write_ex2" {
		return true
	}
	for _, library := range staticTLSLibraries {
		for _, symbol := range library.sendSymbols {
			if target == symbol {
				return true
			}
		}
		for _, symbol := range library.recvSymbols {
			if target == symbol {
				return true
			}
		}
	}
	return false
}

func validateBpfTSTLSShadowManifest(manifest bpfts.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	for _, probe := range manifest.Probes {
		if probe.Kind != "uprobe" && probe.Kind != "uretprobe" {
			return fmt.Errorf("bpf-ts TLS shadow rejects %s probe %q; only userspace probes are allowed", probe.Kind, probe.Name)
		}
		if !isBpfTSTLSShadowTarget(probe.Target) {
			return fmt.Errorf("bpf-ts TLS shadow probe %q targets unsupported TLS symbol %q", probe.Name, probe.Target)
		}
	}
	for _, item := range manifest.Maps {
		if item.Kind == "ringbuf" {
			return nil
		}
	}
	return fmt.Errorf("bpf-ts TLS shadow manifest must expose at least one ringbuf for observation")
}

func (shadow *BpfTSTLSShadowRuntime) Start(config BpfTSTLSShadowConfig) error {
	if shadow == nil {
		return fmt.Errorf("bpf-ts TLS shadow runtime is unavailable")
	}
	shadow.transitionMu.Lock()
	defer shadow.transitionMu.Unlock()

	shadow.mu.RLock()
	alreadyActive := shadow.active || shadow.runtime != nil
	shadow.mu.RUnlock()
	if alreadyActive {
		return ErrBpfTSTLSShadowAlreadyActive
	}
	if config.ObjectPath == "" || config.ManifestPath == "" || config.TargetPath == "" {
		return shadow.fail(fmt.Errorf("bpf-ts TLS shadow requires objectPath, manifestPath, and targetPath"))
	}

	manifest, err := bpfts.LoadManifest(config.ManifestPath)
	if err != nil {
		return shadow.fail(err)
	}
	if err := validateBpfTSTLSShadowManifest(manifest); err != nil {
		return shadow.fail(err)
	}
	if shadow.loader == nil || shadow.readerFactory == nil {
		return shadow.fail(fmt.Errorf("bpf-ts TLS shadow runtime dependencies are unavailable"))
	}

	loaded, err := shadow.loader(config.ObjectPath, manifest, bpfts.LoadOptions{
		ResolveUprobe: NewBpfTSUprobeResolver(config.TargetPath, config.PID),
	})
	if err != nil {
		return shadow.fail(err)
	}

	readers := make(map[string]bpfTSTLSShadowRingReader)
	counters := make(map[string]*bpfTSTLSShadowCounters)
	cleanup := func(cause error) error {
		var closeErrs []error
		for _, reader := range readers {
			closeErrs = append(closeErrs, reader.Close())
		}
		closeErrs = append(closeErrs, loaded.Close())
		if closeErr := errors.Join(closeErrs...); closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("cleanup bpf-ts TLS shadow runtime: %w", closeErr))
		}
		return shadow.fail(cause)
	}

	for _, item := range manifest.Maps {
		if item.Kind != "ringbuf" {
			continue
		}
		loadedMap := loaded.Map(item.Name)
		if loadedMap == nil {
			return cleanup(fmt.Errorf("bpf-ts TLS shadow ringbuf %q is missing from loaded runtime", item.Name))
		}
		reader, err := shadow.readerFactory(loadedMap)
		if err != nil {
			return cleanup(fmt.Errorf("open bpf-ts TLS shadow ringbuf %q: %w", item.Name, err))
		}
		readers[item.Name] = reader
		counter := &bpfTSTLSShadowCounters{}
		counter.readerActive.Store(true)
		counters[item.Name] = counter
	}

	shadow.mu.Lock()
	shadow.runtime = loaded
	shadow.readers = readers
	shadow.counters = counters
	shadow.manifest = manifest
	shadow.config = config
	shadow.startedAt = time.Now()
	shadow.active = true
	shadow.lastError = ""
	shadow.mu.Unlock()

	for name, reader := range readers {
		shadow.wg.Add(1)
		go shadow.readRing(name, reader, counters[name])
	}
	return nil
}

func (shadow *BpfTSTLSShadowRuntime) readRing(name string, reader bpfTSTLSShadowRingReader, counters *bpfTSTLSShadowCounters) {
	defer shadow.wg.Done()
	for {
		record, err := reader.Read()
		if err != nil {
			shadow.mu.RLock()
			active := shadow.active && shadow.readers != nil && shadow.readers[name] == reader
			shadow.mu.RUnlock()
			if !active {
				return
			}
			counters.readErrors.Add(1)
			counters.readerActive.Store(false)
			shadow.mu.Lock()
			if shadow.active && shadow.readers[name] == reader {
				shadow.lastError = fmt.Sprintf("bpf-ts TLS shadow ringbuf %s read: %v", name, err)
			}
			shadow.mu.Unlock()
			return
		}
		counters.records.Add(1)
		counters.bytes.Add(uint64(len(record.RawSample)))
		counters.lastRecordNS.Store(time.Now().UnixNano())
	}
}

func (shadow *BpfTSTLSShadowRuntime) Stop() error {
	if shadow == nil {
		return nil
	}
	shadow.transitionMu.Lock()
	defer shadow.transitionMu.Unlock()

	shadow.mu.Lock()
	loaded := shadow.runtime
	readers := shadow.readers
	shadow.active = false
	shadow.runtime = nil
	shadow.readers = nil
	for _, counter := range shadow.counters {
		counter.readerActive.Store(false)
	}
	shadow.mu.Unlock()

	var errs []error
	for _, reader := range readers {
		errs = append(errs, reader.Close())
	}
	shadow.wg.Wait()
	if loaded != nil {
		errs = append(errs, loaded.Close())
	}
	closeErr := errors.Join(errs...)
	if closeErr != nil {
		shadow.mu.Lock()
		shadow.lastError = closeErr.Error()
		shadow.mu.Unlock()
	}
	return closeErr
}

func (shadow *BpfTSTLSShadowRuntime) Close() error {
	return shadow.Stop()
}

func (shadow *BpfTSTLSShadowRuntime) Status() BpfTSTLSShadowStatus {
	if shadow == nil {
		return BpfTSTLSShadowStatus{LastError: "bpf-ts TLS shadow runtime is unavailable"}
	}
	shadow.mu.RLock()
	active := shadow.active
	manifest := shadow.manifest
	startedAt := shadow.startedAt
	lastError := shadow.lastError
	counters := make(map[string]*bpfTSTLSShadowCounters, len(shadow.counters))
	for name, counter := range shadow.counters {
		counters[name] = counter
	}
	shadow.mu.RUnlock()

	status := BpfTSTLSShadowStatus{
		Active:     active,
		Healthy:    active && len(counters) > 0,
		Source:     manifest.Source,
		ProbeCount: len(manifest.Probes),
		MapCount:   len(manifest.Maps),
		LastError:  lastError,
	}
	if !startedAt.IsZero() {
		status.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	}
	if len(counters) > 0 {
		status.Ringbufs = make(map[string]BpfTSTLSShadowRingStats, len(counters))
		for name, counter := range counters {
			readerActive := counter.readerActive.Load()
			if !readerActive {
				status.Healthy = false
			}
			status.RingbufCount++
			status.Ringbufs[name] = BpfTSTLSShadowRingStats{
				ReaderActive: readerActive,
				Records:      counter.records.Load(),
				Bytes:        counter.bytes.Load(),
				ReadErrors:   counter.readErrors.Load(),
				LastRecordNS: counter.lastRecordNS.Load(),
			}
		}
	}
	return status
}

func (shadow *BpfTSTLSShadowRuntime) fail(err error) error {
	if err == nil || shadow == nil {
		return err
	}
	shadow.mu.Lock()
	shadow.lastError = err.Error()
	shadow.mu.Unlock()
	return err
}
