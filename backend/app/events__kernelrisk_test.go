package app

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unsafe"

	"agent-ebpf-filter/pb"
)

func TestDecodeBPFEventRecordZeroCopy(t *testing.T) {
	raw := make([]byte, bpfEventSampleSize+int(bpfEventSampleAlign))
	base := uintptr(unsafe.Pointer(&raw[0]))
	offset := int((bpfEventSampleAlign - base%bpfEventSampleAlign) % bpfEventSampleAlign)
	sample := raw[offset : offset+bpfEventSampleSize]

	original := (*bpfEvent)(unsafe.Pointer(&sample[0]))
	original.PID = 4242
	original.Type = 4
	copy(original.Comm[:], "rm")
	copy(original.Path[:], "/tmp/target")

	decoded, zeroCopy, err := decodeBPFEventRecord(sample)
	if err != nil {
		t.Fatalf("decodeBPFEventRecord() error = %v", err)
	}
	if nativeLittleEndian && !zeroCopy {
		t.Fatalf("expected zero-copy decode on native little-endian aligned sample")
	}
	if decoded.PID != original.PID || decoded.Type != original.Type {
		t.Fatalf("decoded mismatch: %+v vs %+v", decoded, original)
	}
	if zeroCopy {
		decoded.PID = 7777
		if original.PID != 7777 {
			t.Fatalf("zero-copy decode should point at the raw sample")
		}
	}
}

func TestDecodeBPFEventRecordShortSample(t *testing.T) {
	if _, _, err := decodeBPFEventRecord(make([]byte, bpfEventSampleSize-1)); err == nil {
		t.Fatal("expected short sample error")
	}
}

func TestKernelRiskDecisionAnnotatesSensitiveMutation(t *testing.T) {
	var raw bpfEvent
	raw.PID = 100
	raw.Type = 4 // unlink
	raw.TagID = getTagID("AI Agent")
	copy(raw.Comm[:], "rm")
	copy(raw.Path[:], "/etc/shadow")

	event := buildKernelEventFromRaw(&raw)
	if event == nil {
		t.Fatal("buildKernelEventFromRaw returned nil")
	}
	if event.GetDecision() != "ALERT" {
		t.Fatalf("decision = %q, want ALERT", event.GetDecision())
	}
	if event.GetRiskScore() < 80 {
		t.Fatalf("risk score = %.1f, want >= 80", event.GetRiskScore())
	}
	if !strings.Contains(event.GetExtraInfo(), "kernel_risk") || !strings.Contains(event.GetExtraInfo(), "secret_material_path") {
		t.Fatalf("missing kernel risk reason in ExtraInfo: %q", event.GetExtraInfo())
	}
}

func TestKernelRiskDecisionAnnotatesSuspiciousNetworkTool(t *testing.T) {
	var raw bpfEvent
	raw.PID = 101
	raw.Type = 2 // network_connect
	raw.TagID = getTagID("AI Agent")
	raw.NetFamily = 2
	raw.NetDirection = 1
	raw.NetPort = 4444
	raw.NetAddr = [16]byte{8, 8, 8, 8}
	copy(raw.Comm[:], "curl")

	event := buildKernelEventFromRaw(&raw)
	if event.GetDecision() != "ALERT" {
		t.Fatalf("decision = %q, want ALERT", event.GetDecision())
	}
	if event.GetRiskScore() < 60 {
		t.Fatalf("risk score = %.1f, want >= 60", event.GetRiskScore())
	}
	if !strings.Contains(event.GetExtraInfo(), "high_risk_port_4444") {
		t.Fatalf("missing high-risk port reason: %q", event.GetExtraInfo())
	}
}

func TestCollectorMetricsTrackDecodeAndKernelRisk(t *testing.T) {
	store := newCollectorMetricsState()
	store.RecordRingbufDecode(true)
	store.RecordRingbufDecode(false)
	store.RecordKernelRiskDecision("ALERT", 12)
	store.RecordKernelRiskDecision("BLOCK", 34)
	store.RecordKernelRiskFeedback(true, nil)
	store.RecordKernelRiskFeedback(false, errors.New("boom"))

	snapshot := store.Snapshot()
	if snapshot.RingbufZeroCopyDecodeTotal != 1 || snapshot.RingbufCopyDecodeTotal != 1 {
		t.Fatalf("decode counters = zero-copy %d copy %d", snapshot.RingbufZeroCopyDecodeTotal, snapshot.RingbufCopyDecodeTotal)
	}
	if snapshot.KernelRiskEvaluationsTotal != 2 || snapshot.KernelRiskAlertsTotal != 1 || snapshot.KernelRiskBlocksTotal != 1 {
		t.Fatalf("risk counters = eval %d alert %d block %d", snapshot.KernelRiskEvaluationsTotal, snapshot.KernelRiskAlertsTotal, snapshot.KernelRiskBlocksTotal)
	}
	if snapshot.KernelRiskFeedbackApplied != 1 || snapshot.KernelRiskFeedbackDropped != 1 || snapshot.KernelRiskFeedbackLastError != "boom" {
		t.Fatalf("feedback counters = applied %d dropped %d error %q", snapshot.KernelRiskFeedbackApplied, snapshot.KernelRiskFeedbackDropped, snapshot.KernelRiskFeedbackLastError)
	}
}

func TestKernelRiskFeedbackActionsRespectGatesAndTargets(t *testing.T) {
	settings := RuntimeSettings{
		PolicyManagementEnabled: true,
		KernelRiskFeedback: KernelRiskFeedbackSettings{
			Enabled:          true,
			MinRiskScore:     60,
			EnforceNetwork:   true,
			EnforceFileNames: true,
			EnforceExec:      true,
		},
	}
	decision := kernelRiskDecision{Decision: "ALERT", Score: 96}

	networkActions := kernelRiskFeedbackActions(settings, &pb.Event{
		Type:        "network_connect",
		NetEndpoint: "8.8.8.8:4444",
	}, decision)
	if !hasKernelRiskFeedbackAction(networkActions, kernelRiskFeedbackKindNetworkIP, "8.8.8.8") {
		t.Fatalf("missing network IP feedback action: %+v", networkActions)
	}
	if !hasKernelRiskFeedbackAction(networkActions, kernelRiskFeedbackKindNetworkPort, "4444") {
		t.Fatalf("missing network port feedback action: %+v", networkActions)
	}

	fileActions := kernelRiskFeedbackActions(settings, &pb.Event{
		Type: "unlink",
		Path: "/etc/shadow",
	}, decision)
	if !hasKernelRiskFeedbackAction(fileActions, kernelRiskFeedbackKindLSMFileName, "shadow") {
		t.Fatalf("missing LSM file-name feedback action: %+v", fileActions)
	}

	execActions := kernelRiskFeedbackActions(settings, &pb.Event{
		Type: "execve",
		Path: "/tmp/payload",
		Comm: "payload",
	}, decision)
	if !hasKernelRiskFeedbackAction(execActions, kernelRiskFeedbackKindLSMExecPath, "/tmp/payload") {
		t.Fatalf("missing LSM exec-path feedback action: %+v", execActions)
	}

	settings.PolicyManagementEnabled = false
	if actions := kernelRiskFeedbackActions(settings, &pb.Event{Type: "unlink", Path: "/etc/shadow"}, decision); len(actions) != 0 {
		t.Fatalf("expected policy gate to suppress actions, got %+v", actions)
	}
}

func TestKernelRiskFeedbackStateDedupAndRateLimit(t *testing.T) {
	state := &kernelRiskFeedbackState{seen: make(map[string]time.Time)}
	settings := KernelRiskFeedbackSettings{MaxActionsPerMinute: 1}
	now := time.Unix(100, 0)

	first := kernelRiskFeedbackAction{Kind: kernelRiskFeedbackKindNetworkIP, Target: "8.8.8.8"}
	if !state.Allow(first, settings, now) {
		t.Fatal("first action should be allowed")
	}
	if state.Allow(first, settings, now.Add(time.Second)) {
		t.Fatal("duplicate action should be suppressed")
	}
	second := kernelRiskFeedbackAction{Kind: kernelRiskFeedbackKindNetworkIP, Target: "1.1.1.1"}
	if state.Allow(second, settings, now.Add(2*time.Second)) {
		t.Fatal("rate limit should suppress second distinct action in the same minute")
	}
	if !state.Allow(second, settings, now.Add(2*time.Minute)) {
		t.Fatal("new minute should allow a distinct action")
	}
}

func hasKernelRiskFeedbackAction(actions []kernelRiskFeedbackAction, kind, target string) bool {
	for _, action := range actions {
		if action.Kind == kind && action.Target == target {
			return true
		}
	}
	return false
}
