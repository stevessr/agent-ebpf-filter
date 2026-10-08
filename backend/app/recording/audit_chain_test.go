package recording

import (
	"encoding/json"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
	"google.golang.org/protobuf/proto"
)

func TestAuditChainDetectsTamperAndDeletion(t *testing.T) {
	chain, err := NewAuditChain()
	if err != nil {
		t.Fatal(err)
	}
	records := make([]CapturedEventRecord, 0, 3)
	for i := 1; i <= 3; i++ {
		payload, err := chain.MarshalRecord(CapturedEventRecord{
			ReceivedAt: time.Unix(int64(i), 0).UTC(),
			Event:      &pb.Event{Pid: uint32(i), Type: "openat", Path: "/tmp/test"},
		})
		if err != nil {
			t.Fatal(err)
		}
		var record CapturedEventRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if got := VerifyAuditChain(records); !got.Valid || got.CheckedRecords != 3 || got.Segments != 1 {
		t.Fatalf("valid chain verification = %+v", got)
	}

	tampered := append([]CapturedEventRecord(nil), records...)
	tampered[1].Event = pbCloneEventForAuditTest(tampered[1].Event)
	tampered[1].Event.Path = "/tmp/evil"
	if got := VerifyAuditChain(tampered); got.Valid || got.ErrorIndex != 1 {
		t.Fatalf("tampered chain verification = %+v", got)
	}

	deleted := []CapturedEventRecord{records[0], records[2]}
	if got := VerifyAuditChain(deleted); got.Valid || got.ErrorIndex != 1 {
		t.Fatalf("deleted record verification = %+v", got)
	}
}

func pbCloneEventForAuditTest(in *pb.Event) *pb.Event {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*pb.Event)
}

func TestVerifyAuditChainAllowsLegacyRecordsButMarksThem(t *testing.T) {
	records := []CapturedEventRecord{{ReceivedAt: time.Unix(1, 0).UTC(), Event: &pb.Event{Pid: 1, Type: "execve"}}}
	got := VerifyAuditChain(records)
	if !got.Valid || got.LegacyRecords != 1 || got.CheckedRecords != 0 {
		t.Fatalf("legacy verification = %+v", got)
	}
}
