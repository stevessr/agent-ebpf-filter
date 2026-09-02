package tls

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-ebpf-filter/app/bpfts"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

type fakeBpfTSShadowLoadedRuntime struct {
	maps   map[string]*ebpf.Map
	closed atomic.Bool
}

func (runtime *fakeBpfTSShadowLoadedRuntime) Close() error {
	runtime.closed.Store(true)
	return nil
}

func (runtime *fakeBpfTSShadowLoadedRuntime) Map(name string) *ebpf.Map {
	return runtime.maps[name]
}

type fakeBpfTSShadowReader struct {
	records chan ringbuf.Record
	closed  chan struct{}
	once    sync.Once
}

func newFakeBpfTSShadowReader() *fakeBpfTSShadowReader {
	return &fakeBpfTSShadowReader{
		records: make(chan ringbuf.Record, 4),
		closed:  make(chan struct{}),
	}
}

func (reader *fakeBpfTSShadowReader) Read() (ringbuf.Record, error) {
	select {
	case record := <-reader.records:
		return record, nil
	case <-reader.closed:
		return ringbuf.Record{}, errors.New("reader closed")
	}
}

func (reader *fakeBpfTSShadowReader) Close() error {
	reader.once.Do(func() { close(reader.closed) })
	return nil
}

func writeBpfTSShadowManifest(t *testing.T, manifest bpfts.Manifest) string {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "tls-shadow.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}

func testBpfTSShadowManifest() bpfts.Manifest {
	return bpfts.Manifest{
		Version: bpfts.ManifestVersion,
		Source:  "tls-read.ts",
		Probes: []bpfts.ManifestProbe{
			{Name: "sslReadEnter", Section: "uprobe", Kind: "uprobe", Target: "SSL_read"},
			{Name: "sslReadReturn", Section: "uretprobe", Kind: "uretprobe", Target: "SSL_read"},
		},
		Maps: []bpfts.ManifestMap{
			{Name: "pendingReadBuffers", Kind: "hash", MaxEntries: 1024},
			{Name: "tlsReads", Kind: "ringbuf", MaxEntries: 65536},
		},
	}
}

func TestBpfTSTLSShadowRejectsKernelProbeManifest(t *testing.T) {
	manifest := bpfts.Manifest{
		Version: bpfts.ManifestVersion,
		Source:  "bad.ts",
		Probes: []bpfts.ManifestProbe{
			{Name: "kernel", Section: "kprobe/do_sys_open", Kind: "kprobe", Target: "do_sys_open"},
		},
		Maps: []bpfts.ManifestMap{{Name: "events", Kind: "ringbuf", MaxEntries: 65536}},
	}
	if err := validateBpfTSTLSShadowManifest(manifest); err == nil {
		t.Fatal("kernel probe manifest was accepted by TLS shadow runtime")
	}
}

func TestBpfTSTLSShadowStartCountsRingRecordsAndStopsCleanly(t *testing.T) {
	loaded := &fakeBpfTSShadowLoadedRuntime{
		maps: map[string]*ebpf.Map{"tlsReads": {}},
	}
	reader := newFakeBpfTSShadowReader()
	shadow := newBpfTSTLSShadowRuntime(
		func(_ string, _ bpfts.Manifest, options bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			if options.ResolveUprobe == nil {
				t.Fatal("shadow loader did not receive TLS uprobe resolver")
			}
			return loaded, nil
		},
		func(_ *ebpf.Map) (bpfTSTLSShadowRingReader, error) { return reader, nil },
	)
	manifestPath := writeBpfTSShadowManifest(t, testBpfTSShadowManifest())
	config := BpfTSTLSShadowConfig{
		ObjectPath:   "fake-tls-read.bpf.o",
		ManifestPath: manifestPath,
		TargetPath:   "/bin/false",
		PID:          1234,
	}
	if err := shadow.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := shadow.Start(config); !errors.Is(err, ErrBpfTSTLSShadowAlreadyActive) {
		t.Fatalf("second Start() error = %v, want ErrBpfTSTLSShadowAlreadyActive", err)
	}

	reader.records <- ringbuf.Record{RawSample: []byte{1, 2, 3, 4, 5}}
	deadline := time.Now().Add(time.Second)
	for {
		stats := shadow.Status().Ringbufs["tlsReads"]
		if stats.Records == 1 {
			if stats.Bytes != 5 {
				t.Fatalf("record bytes = %d, want 5", stats.Bytes)
			}
			if stats.ReadErrors != 0 {
				t.Fatalf("read errors before stop = %d, want 0", stats.ReadErrors)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("shadow reader did not observe record: %+v", shadow.Status())
		}
		time.Sleep(time.Millisecond)
	}

	if err := shadow.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !loaded.closed.Load() {
		t.Fatal("Stop() did not close loaded bpf-ts runtime")
	}
	status := shadow.Status()
	if status.Active {
		t.Fatalf("status remained active after Stop(): %+v", status)
	}
	if stats := status.Ringbufs["tlsReads"]; stats.ReadErrors != 0 || stats.Records != 1 || stats.Bytes != 5 {
		t.Fatalf("unexpected final ring stats: %+v", stats)
	}
}

func TestBpfTSTLSShadowMissingRingbufRollsBackLoadedRuntime(t *testing.T) {
	loaded := &fakeBpfTSShadowLoadedRuntime{maps: map[string]*ebpf.Map{}}
	shadow := newBpfTSTLSShadowRuntime(
		func(_ string, _ bpfts.Manifest, _ bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			return loaded, nil
		},
		func(_ *ebpf.Map) (bpfTSTLSShadowRingReader, error) {
			t.Fatal("reader factory must not be called for missing ringbuf")
			return nil, nil
		},
	)
	manifestPath := writeBpfTSShadowManifest(t, testBpfTSShadowManifest())
	err := shadow.Start(BpfTSTLSShadowConfig{
		ObjectPath:   "fake-tls-read.bpf.o",
		ManifestPath: manifestPath,
		TargetPath:   "/bin/false",
	})
	if err == nil {
		t.Fatal("Start() succeeded with missing loaded ringbuf")
	}
	if !loaded.closed.Load() {
		t.Fatal("failed Start() did not roll back loaded runtime")
	}
	if shadow.Status().Active {
		t.Fatal("failed Start() left shadow runtime active")
	}
}
