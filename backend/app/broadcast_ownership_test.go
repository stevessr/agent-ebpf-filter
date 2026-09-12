package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/internal/wsfanout"
	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/redaction"
)

// The broadcaster owns events once they leave the queue: the archived record
// is the very object that was enqueued (no clone), redacted in place, while
// semantic alerts are derived from the unredacted content beforehand.
func TestEventBroadcasterRecordsEventInPlaceAfterDerivingAlerts(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	oldRuntime := runtimeSettingsStore
	oldArchive := capturedEventArchive
	oldEngine := globalRedactionEngine
	originalContext := AppCtx
	runtimeSettingsStore = &runtimeState{}
	capturedEventArchive = newEventArchive(10)
	globalRedactionEngine = redaction.NewRedactionEngine(redaction.RedactionPolicy{
		Level:              redaction.RedactionLevelStrict,
		DefaultPlaceholder: "[REDACTED]",
	})
	appContext := newAppContext()
	appContext.Broadcast = make(chan *pb.Event, 4)
	appContext.EventClientHub = wsfanout.New(wsfanout.Options{})
	appContext.EnvelopeClientHub = wsfanout.New(wsfanout.Options{})
	AppCtx = appContext
	t.Cleanup(func() {
		appContext.Network.Close()
		AppCtx = originalContext
		runtimeSettingsStore = oldRuntime
		capturedEventArchive = oldArchive
		globalRedactionEngine = oldEngine
	})

	event := &pb.Event{
		Type:      "openat",
		Comm:      "codex",
		Pid:       4242,
		Tag:       "AI Agent",
		Path:      "/home/user/.ssh/id_rsa",
		ExtraInfo: "authorization=" + secret,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runEventBroadcaster(ctx)
		close(done)
	}()
	appContext.Broadcast <- event
	deadline := time.Now().Add(2 * time.Second)
	for len(capturedEventArchive.Snapshot(10)) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	records := capturedEventArchive.Snapshot(10)
	if len(records) < 2 {
		t.Fatalf("archived records = %d, want the event plus a semantic alert", len(records))
	}
	if records[0].Event != event {
		t.Fatal("archived record does not share the enqueued event object")
	}
	if strings.Contains(event.GetExtraInfo(), secret) {
		t.Fatalf("event was not redacted in place: %q", event.GetExtraInfo())
	}
	if records[0].Envelope == nil || records[0].Envelope.GetLegacyEvent() != event {
		t.Fatal("envelope legacy event does not share the archived event")
	}
	// The alert could only have been derived from the unredacted path; the
	// strict engine masks the alert's own identifier fields afterwards, so
	// assert on the fields it leaves alone.
	alert := records[1].Event
	if alert.GetType() != "semantic_alert" || alert.GetTag() != "Security" || alert.GetDecision() != "ALERT" {
		t.Fatalf("second record = type:%q tag:%q decision:%q, want a semantic alert", alert.GetType(), alert.GetTag(), alert.GetDecision())
	}
	if strings.Contains(alert.GetExtraInfo(), secret) {
		t.Fatalf("alert leaked the raw secret: %q", alert.GetExtraInfo())
	}
}
