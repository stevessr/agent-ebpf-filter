package research

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeResearchProcessingSettingsCapsSessionEvents(t *testing.T) {
	settings := ResearchProcessingSettings{MaxSessionEvents: 1_000_000}
	normalizeResearchProcessingSettings(&settings)
	if settings.MaxSessionEvents != 100_000 {
		t.Fatalf("MaxSessionEvents = %d, want 100000", settings.MaxSessionEvents)
	}
}

func TestResearchLimitedBufferRejectsLimitPlusOne(t *testing.T) {
	buffer := &researchLimitedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("abcd")); err != nil {
		t.Fatalf("exact-limit write failed: %v", err)
	}
	if _, err := buffer.Write([]byte("e")); err == nil {
		t.Fatal("limit+1 write succeeded")
	}
	if got := buffer.buf.String(); got != "abcd" {
		t.Fatalf("buffer changed after rejected write: %q", got)
	}
}

func TestResearchAddBundlePayloadEnforcesIndividualAndAggregateLimits(t *testing.T) {
	limits := researchExportLimits{payloadBytes: 4, aggregateBytes: 6, archiveBytes: 8}
	payloads := map[string][]byte{}
	var aggregate int64
	if err := researchAddBundlePayload(payloads, "a", []byte("abcd"), limits, &aggregate); err != nil {
		t.Fatalf("add exact-limit payload: %v", err)
	}
	if err := researchAddBundlePayload(payloads, "too-large", []byte("abcde"), limits, &aggregate); err == nil {
		t.Fatal("oversized individual payload accepted")
	}
	if err := researchAddBundlePayload(payloads, "aggregate", []byte("xyz"), limits, &aggregate); err == nil {
		t.Fatal("oversized aggregate payload accepted")
	}
	if aggregate != 4 || !bytes.Equal(payloads["a"], []byte("abcd")) {
		t.Fatalf("payload state changed after rejection: aggregate=%d payloads=%v", aggregate, payloads)
	}
}

func TestResearchBundleBoundsEventPayloadAndArchive(t *testing.T) {
	session := ResearchSession{ID: "rs-test", Name: "bounded"}
	settings := ResearchProcessingSettings{MaxSessionEvents: 1}
	limits := researchExportLimits{payloadBytes: 256, aggregateBytes: 4096, archiveBytes: 4096}

	if _, err := researchBundleZipBytesWithLimits(session, []ResearchEvent{{ID: "one"}, {ID: "two"}}, ResearchResults{}, settings, limits); err == nil || !strings.Contains(err.Error(), "2 events") {
		t.Fatalf("event cardinality error = %v", err)
	}

	largeEvent := ResearchEvent{ID: "one", Target: strings.Repeat("x", 1024)}
	if _, err := researchBundleZipBytesWithLimits(session, []ResearchEvent{largeEvent}, ResearchResults{}, settings, limits); err == nil || !strings.Contains(err.Error(), "events.jsonl") {
		t.Fatalf("payload size error = %v", err)
	}

	archiveLimits := researchExportLimits{payloadBytes: 1 << 20, aggregateBytes: 4 << 20, archiveBytes: 1}
	if _, err := researchBundleZipBytesWithLimits(session, nil, ResearchResults{}, settings, archiveLimits); err == nil || !strings.Contains(err.Error(), "research export exceeds") {
		t.Fatalf("archive size error = %v", err)
	}
}
