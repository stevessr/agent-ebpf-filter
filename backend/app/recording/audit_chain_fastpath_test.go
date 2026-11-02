package recording

import (
	"encoding/json"
	"testing"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

func TestAuditChainSingleNormalizationPreservesHashes(t *testing.T) {
	cases := []struct {
		name   string
		record CapturedEventRecord
	}{
		{
			name: "derived envelope",
			record: CapturedEventRecord{
				ReceivedAt: time.Unix(1700000000, 42).UTC(),
				Event:      &pb.Event{Pid: 15, Type: "openat", Path: "/tmp/a"},
			},
		},
		{
			name: "preexisting envelope",
			// Existing envelopes in the normal pipeline are complete,
			// derived protocol envelopes, not an otherwise empty placeholder.
			// Build one first to validate the compatibility path.
			record: events.NormalizeCapturedEventRecord(CapturedEventRecord{
				ReceivedAt: time.Unix(1700000001, 42).UTC(),
				Event:      &pb.Event{Pid: 16, Type: "network_connect", NetEndpoint: "127.0.0.1:1234"},
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalized := events.NormalizeCapturedEventRecord(tc.record)
			// The original path re-normalizes before hashing; both paths
			// must generate identical hashes for an already normalized event.
			legacy, err := auditRecordHash(normalized, "fixture-id", 3, "fixture-previous")
			if err != nil {
				t.Fatal(err)
			}
			fast, err := auditRecordHashNormalized(normalized, "fixture-id", 3, "fixture-previous")
			if err != nil {
				t.Fatal(err)
			}
			if legacy != fast {
				t.Fatalf("single-pass digest differs from legacy hash")
			}

			chain, err := NewAuditChain()
			if err != nil {
				t.Fatal(err)
			}
			records := make([]CapturedEventRecord, 0, 4)
			for i := 0; i < 4; i++ {
				payload, err := chain.MarshalRecord(tc.record)
				if err != nil {
					t.Fatal(err)
				}
				var decoded CapturedEventRecord
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatal(err)
				}
				records = append(records, decoded)
			}
			result := VerifyAuditChain(records)
			if !result.Valid || result.CheckedRecords != len(records) {
				t.Fatalf("audit chain verification failed: %+v", result)
			}
		})
	}
}

func BenchmarkAuditHashNormalization(b *testing.B) {
	record := events.NormalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		Event: &pb.Event{
			Pid: 4242, Ppid: 1, Type: "openat", Comm: "claude",
			Path: "/home/runner/project/main.go", ExtraInfo: "agent_context",
		},
	})
	b.Run("legacy-renormalized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := auditRecordHash(record, "benchmark-chain", 100, "previous"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("single-pass", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := auditRecordHashNormalized(record, "benchmark-chain", 100, "previous"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkAuditChainMarshalRecord(b *testing.B) {
	record := CapturedEventRecord{
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		Event:      &pb.Event{Pid: 4242, Type: "openat", Path: "/tmp/demo", Comm: "agent"},
	}
	chain, err := NewAuditChain()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := chain.MarshalRecord(record); err != nil {
			b.Fatal(err)
		}
	}
}
