package tls

import (
	"errors"
	"testing"

	"agent-ebpf-filter/app/bpfts"
	"github.com/cilium/ebpf"
)

func TestTLSCaptureControllerExposesDormantBpfTSShadowStatus(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	defer controller.Close()

	status := controller.Status()
	value, ok := status["bpfTsShadow"]
	if !ok {
		t.Fatal("controller status omitted bpfTsShadow")
	}
	shadow, ok := value.(BpfTSTLSShadowStatus)
	if !ok {
		t.Fatalf("bpfTsShadow status type = %T, want BpfTSTLSShadowStatus", value)
	}
	if shadow.Active {
		t.Fatalf("new controller unexpectedly started bpf-ts shadow: %+v", shadow)
	}
}

func TestTLSCaptureControllerRejectsShadowWhenCaptureDisabled(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	defer controller.Close()
	controller.SetEnabledCheck(func() bool { return false })

	err := controller.StartBpfTSShadow(BpfTSTLSShadowConfig{
		ObjectPath:   "not-used.o",
		ManifestPath: "not-used.json",
		TargetPath:   "/bin/false",
	})
	if !errors.Is(err, ErrTLSCaptureDisabled) {
		t.Fatalf("StartBpfTSShadow() error = %v, want ErrTLSCaptureDisabled", err)
	}
	if controller.BpfTSShadowStatus().Active {
		t.Fatal("disabled controller started bpf-ts shadow")
	}
}

func TestTLSCaptureControllerStartsAndStopsInjectedShadow(t *testing.T) {
	loaded := &fakeBpfTSShadowLoadedRuntime{maps: map[string]*ebpf.Map{"tlsReads": &ebpf.Map{}}}
	reader := newFakeBpfTSShadowReader()
	shadow := newBpfTSTLSShadowRuntime(
		func(_ string, _ bpfts.Manifest, _ bpfts.LoadOptions) (bpfTSTLSShadowLoadedRuntime, error) {
			return loaded, nil
		},
		func(_ *ebpf.Map) (bpfTSTLSShadowRingReader, error) { return reader, nil },
	)
	controller := NewTLSCaptureController(nil, nil, nil)
	controller.bpfTSShadow = shadow
	manifestPath := writeBpfTSShadowManifest(t, testBpfTSShadowManifest())

	if err := controller.StartBpfTSShadow(BpfTSTLSShadowConfig{
		ObjectPath:   "fake-tls-read.bpf.o",
		ManifestPath: manifestPath,
		TargetPath:   "/bin/false",
		PID:          777,
	}); err != nil {
		t.Fatalf("StartBpfTSShadow() error = %v", err)
	}
	if status := controller.BpfTSShadowStatus(); !status.Active {
		t.Fatalf("shadow not active after start: %+v", status)
	}
	if err := controller.StopBpfTSShadow(); err != nil {
		t.Fatalf("StopBpfTSShadow() error = %v", err)
	}
	if status := controller.BpfTSShadowStatus(); status.Active {
		t.Fatalf("shadow remained active after stop: %+v", status)
	}
	if !loaded.closed.Load() {
		t.Fatal("controller stop did not close bpf-ts runtime")
	}
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() after stopped shadow error = %v", err)
	}
}
