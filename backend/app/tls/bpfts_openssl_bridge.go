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

var ErrBpfTSOpenSSLBridgeAlreadyActive = errors.New("bpf-ts OpenSSL bridge is already active")

type BpfTSOpenSSLBridgeConfig struct {
	ObjectPath   string `json:"objectPath"`
	ManifestPath string `json:"manifestPath"`
	TargetPath   string `json:"targetPath"`
	PID          int    `json:"pid,omitempty"`
}

type BpfTSOpenSSLBridgeStatus struct {
	Active       bool   `json:"active"`
	Healthy      bool   `json:"healthy"`
	ReaderActive bool   `json:"readerActive"`
	StartedAt    string `json:"startedAt,omitempty"`
	TargetPath   string `json:"targetPath,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Records      uint64 `json:"records"`
	Bytes        uint64 `json:"bytes"`
	Decoded      uint64 `json:"decoded"`
	DecodeErrors uint64 `json:"decodeErrors"`
	HTTPEvents   uint64 `json:"httpEvents"`
	RawEvents    uint64 `json:"rawEvents"`
	ReadErrors   uint64 `json:"readErrors"`
	LastRecordNS int64  `json:"lastRecordNs,omitempty"`
	LastError    string `json:"error,omitempty"`
}

type bpfTSOpenSSLBridgeSink func(CompletedTLSFragment) tlsCompletedProcessResult

type BpfTSOpenSSLBridgeRuntime struct {
	transitionMu sync.Mutex
	mu           sync.RWMutex
	wg           sync.WaitGroup

	loader        bpfTSTLSShadowLoader
	readerFactory bpfTSTLSShadowRingFactory
	sink          bpfTSOpenSSLBridgeSink

	runtime   bpfTSTLSShadowLoadedRuntime
	reader    bpfTSTLSShadowRingReader
	config    BpfTSOpenSSLBridgeConfig
	startedAt time.Time
	active    bool
	lastError string

	records      atomic.Uint64
	bytes        atomic.Uint64
	decoded      atomic.Uint64
	decodeErrors atomic.Uint64
	httpEvents   atomic.Uint64
	rawEvents    atomic.Uint64
	readErrors   atomic.Uint64
	readerActive atomic.Bool
	lastRecordNS atomic.Int64
}

func NewBpfTSOpenSSLBridgeRuntime(
	store *TLSCaptureStore,
	rules *TLSCaptureRuleStore,
	broadcaster *TLSBroadcaster,
) *BpfTSOpenSSLBridgeRuntime {
	processor := newTLSCompletedEventProcessor(store, rules, broadcaster)
	return newBpfTSOpenSSLBridgeRuntime(
		func(objectPath string, manifest bpfts.Manifest, options bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			return bpfts.LoadAndAttach(objectPath, manifest, options)
		},
		func(m *ebpf.Map) (bpfTSTLSShadowRingReader, error) {
			return ringbuf.NewReader(m)
		},
		processor.Process,
	)
}

func newBpfTSOpenSSLBridgeRuntime(
	loader bpfTSTLSShadowLoader,
	readerFactory bpfTSTLSShadowRingFactory,
	sink bpfTSOpenSSLBridgeSink,
) *BpfTSOpenSSLBridgeRuntime {
	return &BpfTSOpenSSLBridgeRuntime{
		loader:        loader,
		readerFactory: readerFactory,
		sink:          sink,
	}
}

func (bridge *BpfTSOpenSSLBridgeRuntime) Start(config BpfTSOpenSSLBridgeConfig) error {
	if bridge == nil {
		return fmt.Errorf("bpf-ts OpenSSL bridge runtime is unavailable")
	}
	bridge.transitionMu.Lock()
	defer bridge.transitionMu.Unlock()

	bridge.mu.RLock()
	alreadyActive := bridge.active || bridge.runtime != nil
	bridge.mu.RUnlock()
	if alreadyActive {
		return ErrBpfTSOpenSSLBridgeAlreadyActive
	}
	if config.ObjectPath == "" || config.ManifestPath == "" || config.TargetPath == "" {
		return bridge.fail(fmt.Errorf("bpf-ts OpenSSL bridge requires objectPath, manifestPath, and targetPath"))
	}
	if bridge.loader == nil || bridge.readerFactory == nil || bridge.sink == nil {
		return bridge.fail(fmt.Errorf("bpf-ts OpenSSL bridge runtime dependencies are unavailable"))
	}

	manifest, err := bpfts.LoadManifest(config.ManifestPath)
	if err != nil {
		return bridge.fail(err)
	}
	if err := validateBpfTSOpenSSLManifest(manifest); err != nil {
		return bridge.fail(err)
	}

	loaded, err := bridge.loader(config.ObjectPath, manifest, bpfts.LoadOptions{
		ResolveUprobe: NewBpfTSUprobeResolver(config.TargetPath, config.PID),
	})
	if err != nil {
		return bridge.fail(err)
	}
	cleanupLoaded := func(cause error) error {
		if closeErr := loaded.Close(); closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("cleanup bpf-ts OpenSSL bridge runtime: %w", closeErr))
		}
		return bridge.fail(cause)
	}

	eventsMap := loaded.Map(bpfTSOpenSSLRingName)
	if eventsMap == nil {
		return cleanupLoaded(fmt.Errorf("bpf-ts OpenSSL bridge ringbuf %q is missing from loaded runtime", bpfTSOpenSSLRingName))
	}
	reader, err := bridge.readerFactory(eventsMap)
	if err != nil {
		return cleanupLoaded(fmt.Errorf("open bpf-ts OpenSSL bridge ringbuf %q: %w", bpfTSOpenSSLRingName, err))
	}

	bridge.records.Store(0)
	bridge.bytes.Store(0)
	bridge.decoded.Store(0)
	bridge.decodeErrors.Store(0)
	bridge.httpEvents.Store(0)
	bridge.rawEvents.Store(0)
	bridge.readErrors.Store(0)
	bridge.readerActive.Store(true)
	bridge.lastRecordNS.Store(0)

	bridge.mu.Lock()
	bridge.runtime = loaded
	bridge.reader = reader
	bridge.config = config
	bridge.startedAt = time.Now()
	bridge.active = true
	bridge.lastError = ""
	bridge.mu.Unlock()

	bridge.wg.Add(1)
	go bridge.readLoop(reader)
	return nil
}

func (bridge *BpfTSOpenSSLBridgeRuntime) readLoop(reader bpfTSTLSShadowRingReader) {
	defer bridge.wg.Done()
	for {
		record, err := reader.Read()
		if err != nil {
			bridge.mu.RLock()
			active := bridge.active && bridge.reader == reader
			bridge.mu.RUnlock()
			if !active {
				return
			}
			bridge.readErrors.Add(1)
			bridge.readerActive.Store(false)
			bridge.setError(fmt.Errorf("bpf-ts OpenSSL bridge ringbuf read: %w", err))
			return
		}

		bridge.records.Add(1)
		bridge.bytes.Add(uint64(len(record.RawSample)))
		bridge.lastRecordNS.Store(time.Now().UnixNano())

		event, err := decodeBpfTSOpenSSLEvent(record.RawSample)
		if err != nil {
			bridge.decodeErrors.Add(1)
			bridge.setError(err)
			continue
		}
		bridge.decoded.Add(1)
		completed := bpfTSOpenSSLToCompleted(event)
		result := bridge.sink(completed)
		bridge.httpEvents.Add(uint64(result.HTTPEvents))
		bridge.rawEvents.Add(uint64(result.RawEvents))
	}
}

func (bridge *BpfTSOpenSSLBridgeRuntime) Stop() error {
	if bridge == nil {
		return nil
	}
	bridge.transitionMu.Lock()
	defer bridge.transitionMu.Unlock()

	bridge.mu.Lock()
	loaded := bridge.runtime
	reader := bridge.reader
	bridge.active = false
	bridge.runtime = nil
	bridge.reader = nil
	bridge.readerActive.Store(false)
	bridge.mu.Unlock()

	var errs []error
	if reader != nil {
		errs = append(errs, reader.Close())
	}
	bridge.wg.Wait()
	if loaded != nil {
		errs = append(errs, loaded.Close())
	}
	closeErr := errors.Join(errs...)
	if closeErr != nil {
		bridge.setError(closeErr)
	}
	return closeErr
}

func (bridge *BpfTSOpenSSLBridgeRuntime) Close() error {
	return bridge.Stop()
}

func (bridge *BpfTSOpenSSLBridgeRuntime) Status() BpfTSOpenSSLBridgeStatus {
	if bridge == nil {
		return BpfTSOpenSSLBridgeStatus{LastError: "bpf-ts OpenSSL bridge runtime is unavailable"}
	}
	bridge.mu.RLock()
	active := bridge.active
	config := bridge.config
	startedAt := bridge.startedAt
	lastError := bridge.lastError
	bridge.mu.RUnlock()
	readerActive := bridge.readerActive.Load()

	status := BpfTSOpenSSLBridgeStatus{
		Active:       active,
		Healthy:      active && readerActive,
		ReaderActive: readerActive,
		TargetPath:   config.TargetPath,
		PID:          config.PID,
		Records:      bridge.records.Load(),
		Bytes:        bridge.bytes.Load(),
		Decoded:      bridge.decoded.Load(),
		DecodeErrors: bridge.decodeErrors.Load(),
		HTTPEvents:   bridge.httpEvents.Load(),
		RawEvents:    bridge.rawEvents.Load(),
		ReadErrors:   bridge.readErrors.Load(),
		LastRecordNS: bridge.lastRecordNS.Load(),
		LastError:    lastError,
	}
	if !startedAt.IsZero() {
		status.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (bridge *BpfTSOpenSSLBridgeRuntime) setError(err error) {
	if bridge == nil || err == nil {
		return
	}
	bridge.mu.Lock()
	bridge.lastError = err.Error()
	bridge.mu.Unlock()
}

func (bridge *BpfTSOpenSSLBridgeRuntime) fail(err error) error {
	bridge.setError(err)
	return err
}
