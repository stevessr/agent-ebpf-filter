package app

import (
	"testing"

	"agent-ebpf-filter/app/ml"
)

// The offline candidate must not depend on when CI happened to process the
// command. Keep non-clock behavioral and network evidence untouched.
func TestDecisionOfflineClockFeatureNeutralization(t *testing.T) {
	var sample ml.TrainingSample
	for i := range sample.Features {
		sample.Features[i] = float64(i+1) / 128.0
	}
	original := sample.Features
	neutralizeOfflineDecisionClockFeatures(&sample)
	for i := range sample.Features {
		if i >= 114 && i <= 118 {
			if sample.Features[i] != 0 {
				t.Fatalf("untrusted offline clock feature %d was retained", i)
			}
		} else if sample.Features[i] != original[i] {
			t.Fatalf("non-clock feature %d was modified", i)
		}
	}
}
