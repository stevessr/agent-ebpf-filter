package tls

import (
	"math"
	"testing"
)

func TestBpfTSOpenSSLWireEfficiencyCompactRecords(t *testing.T) {
	status := BpfTSOpenSSLBridgeStatus{
		Records: 2,
		Bytes:   uint64((bpfTSOpenSSLMetadataSize + 32) + (bpfTSOpenSSLMetadataSize + 128)),
	}
	efficiency := bpfTSOpenSSLWireEfficiency(status)
	wantLegacy := uint64(2 * bpfTSOpenSSLEventSize)
	wantSaved := wantLegacy - status.Bytes

	if efficiency.LegacyEquivalentBytes != wantLegacy {
		t.Fatalf("legacy bytes = %d, want %d", efficiency.LegacyEquivalentBytes, wantLegacy)
	}
	if efficiency.SavedBytes != wantSaved {
		t.Fatalf("saved bytes = %d, want %d", efficiency.SavedBytes, wantSaved)
	}
	if efficiency.AverageBytesPerRecord != float64(status.Bytes)/2 {
		t.Fatalf("average bytes/record = %f", efficiency.AverageBytesPerRecord)
	}
	wantPercent := float64(wantSaved) * 100 / float64(wantLegacy)
	if math.Abs(efficiency.SavingsPercent-wantPercent) > 0.000001 {
		t.Fatalf("savings = %f%%, want %f%%", efficiency.SavingsPercent, wantPercent)
	}
}

func TestBpfTSOpenSSLWireEfficiencyDoesNotReportNegativeSavings(t *testing.T) {
	status := BpfTSOpenSSLBridgeStatus{Records: 1, Bytes: uint64(bpfTSOpenSSLEventSize + 100)}
	efficiency := bpfTSOpenSSLWireEfficiency(status)
	if efficiency.SavedBytes != 0 || efficiency.SavingsPercent != 0 {
		t.Fatalf("unexpected negative savings projection: %#v", efficiency)
	}
}

func TestBpfTSOpenSSLWireEfficiencyHandlesEmptyStatus(t *testing.T) {
	efficiency := bpfTSOpenSSLWireEfficiency(BpfTSOpenSSLBridgeStatus{})
	if efficiency.Records != 0 || efficiency.ActualWireBytes != 0 || efficiency.AverageBytesPerRecord != 0 {
		t.Fatalf("unexpected empty efficiency: %#v", efficiency)
	}
}
