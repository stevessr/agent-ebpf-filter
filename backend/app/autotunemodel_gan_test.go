package app

import (
	"fmt"
	"testing"
	"time"

	"agent-ebpf-filter/app/ml"
)

const ganTuneTestClasses = 4

func ganTuneTestFeatures(classIdx, sampleIdx int) [ml.FeatureDim]float64 {
	var features [ml.FeatureDim]float64
	for d := 0; d < ml.FeatureDim; d++ {
		block := (d / 8) % ganTuneTestClasses
		value := 0.12
		if block == classIdx {
			value = 0.82
		}
		if d%ganTuneTestClasses == classIdx {
			value += 0.08
		}
		jitter := float64((sampleIdx*(d+3)+classIdx*11+d)%13) / 200.0
		features[d] = ml.Clamp01(value + jitter)
	}
	return features
}

func newGANAutoTuneTestStore(t *testing.T, perClass int) *ml.TrainingDataStore {
	t.Helper()
	store := ml.NewTrainingDataStore(ganTuneTestClasses*perClass + 16)
	store.SetPersistLocation(t.TempDir())
	for c := 0; c < ganTuneTestClasses; c++ {
		for i := 0; i < perClass; i++ {
			store.Add(ml.TrainingSample{
				Features:    ganTuneTestFeatures(c, i),
				Label:       int32(c),
				Comm:        fmt.Sprintf("gan-class-%d", c),
				CommandLine: fmt.Sprintf("gan-class-%d sample-%d", c, i),
				Timestamp:   time.Now(),
				UserLabel:   "test",
			})
		}
	}
	return store
}

func TestGANTransformerModelAutoTuneCandidate(t *testing.T) {
	store := newGANAutoTuneTestStore(t, 24)
	req := ml.MLModelTuneRequest{
		ModelTypes:           []string{string(ml.ModelGANTransformer)},
		Metric:               "balancedAccuracy",
		ValidationSplitRatio: 0.25,
	}
	resp, err := runModelAutoTune(store, req, []ml.ModelType{ml.ModelGANTransformer}, nil)
	if err != nil {
		t.Fatalf("runModelAutoTune: %v", err)
	}
	if resp == nil || resp.Best == nil {
		t.Fatalf("expected best model tune response: %+v", resp)
	}
	if resp.Best.ModelType != string(ml.ModelGANTransformer) {
		t.Fatalf("best model mismatch: %+v", resp.Best)
	}
	if resp.Best.InferenceThroughput <= 0 || resp.Best.InferenceMsPerSample <= 0 {
		t.Fatalf("missing performance metrics: %+v", resp.Best)
	}
	t.Logf("GAN+Transformer auto-tune candidate: score=%.3f balanced=%.3f allow=%.3f throughput=%.0f/s",
		resp.Best.Score, resp.Best.BalancedAccuracy, resp.Best.AllowRecall, resp.Best.InferenceThroughput)
}
