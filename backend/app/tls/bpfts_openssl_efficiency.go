package tls

// BpfTSOpenSSLWireEfficiency compares the observed compact ringbuf traffic with
// the legacy fixed-size OpenSSL record format. It is derived entirely from
// existing bridge counters, so collecting it adds no work to the hot path.
type BpfTSOpenSSLWireEfficiency struct {
	Records               uint64  `json:"records"`
	ActualWireBytes       uint64  `json:"actualWireBytes"`
	LegacyEquivalentBytes uint64  `json:"legacyEquivalentBytes"`
	SavedBytes            uint64  `json:"savedBytes"`
	SavingsPercent        float64 `json:"savingsPercent"`
	AverageBytesPerRecord float64 `json:"averageBytesPerRecord"`
}

func bpfTSOpenSSLWireEfficiency(status BpfTSOpenSSLBridgeStatus) BpfTSOpenSSLWireEfficiency {
	efficiency := BpfTSOpenSSLWireEfficiency{
		Records:         status.Records,
		ActualWireBytes: status.Bytes,
	}
	if status.Records == 0 {
		return efficiency
	}

	// Saturate instead of wrapping if a corrupt or synthetic counter reaches the
	// uint64 multiplication boundary. Real bridge counters are far below this.
	maxRecords := ^uint64(0) / uint64(bpfTSOpenSSLEventSize)
	if status.Records > maxRecords {
		efficiency.LegacyEquivalentBytes = ^uint64(0)
	} else {
		efficiency.LegacyEquivalentBytes = status.Records * uint64(bpfTSOpenSSLEventSize)
	}
	if efficiency.LegacyEquivalentBytes > status.Bytes {
		efficiency.SavedBytes = efficiency.LegacyEquivalentBytes - status.Bytes
		efficiency.SavingsPercent = float64(efficiency.SavedBytes) * 100 / float64(efficiency.LegacyEquivalentBytes)
	}
	efficiency.AverageBytesPerRecord = float64(status.Bytes) / float64(status.Records)
	return efficiency
}

func (bridge *BpfTSOpenSSLBridgeRuntime) WireEfficiency() BpfTSOpenSSLWireEfficiency {
	if bridge == nil {
		return BpfTSOpenSSLWireEfficiency{}
	}
	return bpfTSOpenSSLWireEfficiency(bridge.Status())
}

func (c *TLSCaptureController) BpfTSOpenSSLWireEfficiency() BpfTSOpenSSLWireEfficiency {
	if c == nil {
		return BpfTSOpenSSLWireEfficiency{}
	}
	return bpfTSOpenSSLWireEfficiency(c.BpfTSOpenSSLBridgeStatus())
}
