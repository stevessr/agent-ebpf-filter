package tls

import (
	"errors"
	"testing"
)

func TestTLSCaptureControllerInitializesBpfTSModes(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	if controller.bpfTSShadow == nil {
		t.Fatal("expected dormant bpf-ts shadow runtime")
	}
	if controller.bpfTSBridge == nil {
		t.Fatal("expected dormant bpf-ts OpenSSL bridge runtime")
	}

	status := controller.Status()
	if _, ok := status["bpfTsShadow"].(BpfTSTLSShadowStatus); !ok {
		t.Fatalf("expected bpfTsShadow status, got %#v", status["bpfTsShadow"])
	}
	if _, ok := status["bpfTsBridge"].(BpfTSOpenSSLBridgeStatus); !ok {
		t.Fatalf("expected bpfTsBridge status, got %#v", status["bpfTsBridge"])
	}
	if _, ok := status["bpfTsWireEfficiency"].(BpfTSOpenSSLWireEfficiency); !ok {
		t.Fatalf("expected bpfTsWireEfficiency status, got %#v", status["bpfTsWireEfficiency"])
	}
	if _, ok := status["bpfTsBackpressure"].(BpfTSOpenSSLBackpressureStatus); !ok {
		t.Fatalf("expected bpfTsBackpressure status, got %#v", status["bpfTsBackpressure"])
	}
	if active, _ := status["captureActive"].(bool); active {
		t.Fatal("fresh controller must not report an active capture backend")
	}

	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTLSCaptureControllerRejectsShadowWhenBridgeActive(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	bridge := controller.bpfTSBridge
	bridge.mu.Lock()
	bridge.active = true
	bridge.mu.Unlock()

	err := controller.StartBpfTSShadow(BpfTSTLSShadowConfig{
		ObjectPath: "unused.o", ManifestPath: "unused.json", TargetPath: "/unused/libssl.so",
	})
	if !errors.Is(err, ErrBpfTSTLSModeConflict) {
		t.Fatalf("expected mode conflict, got %v", err)
	}

	bridge.mu.Lock()
	bridge.active = false
	bridge.mu.Unlock()
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTLSCaptureControllerRejectsBridgeWhenShadowActive(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	shadow := controller.bpfTSShadow
	shadow.mu.Lock()
	shadow.active = true
	shadow.mu.Unlock()

	err := controller.StartBpfTSOpenSSLBridge(BpfTSOpenSSLBridgeConfig{
		ObjectPath: "unused.o", ManifestPath: "unused.json", TargetPath: "/unused/libssl.so",
	})
	if !errors.Is(err, ErrBpfTSTLSModeConflict) {
		t.Fatalf("expected mode conflict, got %v", err)
	}

	shadow.mu.Lock()
	shadow.active = false
	shadow.mu.Unlock()
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTLSCaptureControllerRejectsBridgeWhenLegacyManagerExists(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	controller.mu.Lock()
	controller.manager = &TLSProbeManager{}
	controller.mu.Unlock()

	err := controller.StartBpfTSOpenSSLBridge(BpfTSOpenSSLBridgeConfig{
		ObjectPath: "unused.o", ManifestPath: "unused.json", TargetPath: "/unused/libssl.so",
	})
	if !errors.Is(err, ErrBpfTSTLSModeConflict) {
		t.Fatalf("expected legacy/bridge mode conflict before file IO, got %v", err)
	}

	controller.mu.Lock()
	controller.manager = nil
	controller.mu.Unlock()
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTLSCaptureControllerRejectsLegacyStartWhenBridgeActive(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	bridge := controller.bpfTSBridge
	bridge.mu.Lock()
	bridge.active = true
	bridge.mu.Unlock()

	if _, err := controller.EnsureStarted(); !errors.Is(err, ErrBpfTSTLSModeConflict) {
		t.Fatalf("expected bridge/legacy mode conflict, got %v", err)
	}

	bridge.mu.Lock()
	bridge.active = false
	bridge.mu.Unlock()
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTLSCaptureControllerBpfTSModesHonorDisabledStateBeforeIO(t *testing.T) {
	controller := NewTLSCaptureController(nil, nil, nil)
	controller.SetEnabledCheck(func() bool { return false })

	shadowErr := controller.StartBpfTSShadow(BpfTSTLSShadowConfig{
		ObjectPath: "missing.o", ManifestPath: "missing.json", TargetPath: "/missing/libssl.so",
	})
	if !errors.Is(shadowErr, ErrTLSCaptureDisabled) {
		t.Fatalf("expected disabled shadow error before file IO, got %v", shadowErr)
	}

	bridgeErr := controller.StartBpfTSOpenSSLBridge(BpfTSOpenSSLBridgeConfig{
		ObjectPath: "missing.o", ManifestPath: "missing.json", TargetPath: "/missing/libssl.so",
	})
	if !errors.Is(bridgeErr, ErrTLSCaptureDisabled) {
		t.Fatalf("expected disabled bridge error before file IO, got %v", bridgeErr)
	}

	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
