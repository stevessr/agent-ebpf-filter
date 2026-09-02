package tls

import (
	"errors"
	"testing"
	"time"

	"agent-ebpf-filter/app/bpfts"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

func TestBpfTSOpenSSLBridgeDecodesAndForwardsCompletedFragment(t *testing.T) {
	loaded := &fakeBpfTSShadowLoadedRuntime{
		maps: map[string]*ebpf.Map{bpfTSOpenSSLRingName: {}},
	}
	reader := newFakeBpfTSShadowReader()
	completedCh := make(chan CompletedTLSFragment, 1)
	bridge := newBpfTSOpenSSLBridgeRuntime(
		func(_ string, manifest bpfts.Manifest, options bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			if err := validateBpfTSOpenSSLManifest(manifest); err != nil {
				t.Fatalf("loader received non-canonical manifest: %v", err)
			}
			if options.ResolveUprobe == nil {
				t.Fatal("bridge loader did not receive TLS resolver")
			}
			return loaded, nil
		},
		func(_ *ebpf.Map) (bpfTSTLSShadowRingReader, error) { return reader, nil },
		func(completed CompletedTLSFragment) tlsCompletedProcessResult {
			completedCh <- completed
			return tlsCompletedProcessResult{RawEvents: 1}
		},
	)
	manifestPath := writeBpfTSShadowManifest(t, testBpfTSOpenSSLManifest())
	config := BpfTSOpenSSLBridgeConfig{
		ObjectPath:   "fake-tls-openssl.bpf.o",
		ManifestPath: manifestPath,
		TargetPath:   "/bin/false",
		PID:          987,
	}
	if err := bridge.Start(config); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := bridge.Start(config); !errors.Is(err, ErrBpfTSOpenSSLBridgeAlreadyActive) {
		t.Fatalf("second Start() error = %v", err)
	}

	reader.records <- ringbuf.Record{RawSample: encodeBpfTSOpenSSLFixture(t, 96, bpfTSOpenSSLDirectionRecv, tlsFuncSSLRead)}
	select {
	case completed := <-completedCh:
		if completed.TGID != 4242 || completed.PID != 4243 || completed.OriginalLen != 96 {
			t.Fatalf("unexpected completed fragment: %+v", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("bridge did not forward decoded record")
	}

	deadline := time.Now().Add(time.Second)
	for bridge.Status().Decoded != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("bridge counters did not settle: %+v", bridge.Status())
		}
		time.Sleep(time.Millisecond)
	}
	status := bridge.Status()
	if status.Records != 1 || status.DecodeErrors != 0 || status.RawEvents != 1 {
		t.Fatalf("unexpected bridge status: %+v", status)
	}
	if err := bridge.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !loaded.closed.Load() {
		t.Fatal("bridge stop did not close loaded runtime")
	}
	if bridge.Status().Active {
		t.Fatal("bridge remained active after stop")
	}
}

func TestBpfTSOpenSSLBridgeCountsDecodeErrorsWithoutCallingSink(t *testing.T) {
	loaded := &fakeBpfTSShadowLoadedRuntime{maps: map[string]*ebpf.Map{bpfTSOpenSSLRingName: {}}}
	reader := newFakeBpfTSShadowReader()
	called := make(chan struct{}, 1)
	bridge := newBpfTSOpenSSLBridgeRuntime(
		func(_ string, _ bpfts.Manifest, _ bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			return loaded, nil
		},
		func(_ *ebpf.Map) (bpfTSTLSShadowRingReader, error) { return reader, nil },
		func(CompletedTLSFragment) tlsCompletedProcessResult {
			called <- struct{}{}
			return tlsCompletedProcessResult{}
		},
	)
	manifestPath := writeBpfTSShadowManifest(t, testBpfTSOpenSSLManifest())
	if err := bridge.Start(BpfTSOpenSSLBridgeConfig{
		ObjectPath: "fake.o", ManifestPath: manifestPath, TargetPath: "/bin/false",
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer bridge.Stop()

	reader.records <- ringbuf.Record{RawSample: []byte{1, 2, 3}}
	deadline := time.Now().Add(time.Second)
	for bridge.Status().DecodeErrors != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("decode error counter did not update: %+v", bridge.Status())
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-called:
		t.Fatal("decode error reached completed-fragment sink")
	default:
	}
}
