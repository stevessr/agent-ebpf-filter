package sweep

import (
	"agent-ebpf-filter/app/ml"
	"fmt"
	"testing"
	"time"
)

func TestIncrementalSweepUsesRealHoldout(t *testing.T) {
	samples := make([]ml.TrainingSample, 0, 80)
	for i := 0; i < 80; i++ {
		var features [ml.FeatureDim]float64
		features[i%4] = 1
		features[31] = float64(i+1) / 100
		samples = append(samples, ml.TrainingSample{
			Features: features,
			Label: int32(i % 4),
			CommandLine: fmt.Sprintf("sweep-class-%d-example-%d", i%4, i),
			Timestamp: time.Unix(1780000000+int64(i), 0),
			UserLabel: "test",
		})
	}
	store := trainingStoreFromSamples(samples)
	for _, modelType := range []ModelType{ModelExtraTrees, ModelAdaBoost} {
		t.Run(string(modelType), func(t *testing.T) {
			parameter := "numTrees"
			if modelType == ModelAdaBoost {
				parameter = "estimators"
			}
			profile := sweepProfile{
				Name: "holdout-safety", ModelType: modelType,
				Kind: "bar", ParameterKind: "numeric", ParameterName: parameter,
				XValues: []int{5, 12},
				Build: func(x, _ int) MLConfig {
					cfg := DefaultMLConfig()
					cfg.ModelType = modelType
					cfg.NumTrees = x
					cfg.MaxDepth = 4
					cfg.MinSamplesLeaf = 2
					cfg.ValidationSplitRatio = 0.25
					return cfg
				},
				Summary: func(MLConfig) string { return "test" },
			}
			if !canRunIncrementalCountProfile(profile) {
				t.Fatal("supported incremental profile not detected")
			}
			ctx, err := buildIncrementalCountContext(profile, store, nil)
			if err != nil {
				t.Fatal(err)
			}
			if ctx.TrainSamples+ctx.ValSamples != len(samples) || ctx.ValSamples == 0 {
				t.Fatalf("sweep must expose an independent validation partition: train=%d val=%d", ctx.TrainSamples, ctx.ValSamples)
			}
			if len(ctx.TrainSet) != ctx.TrainSamples || len(ctx.ValSet) != ctx.ValSamples || len(ctx.ValRaw) != ctx.ValSamples {
				t.Fatalf("invalid sweep partition accounting: train=%d val=%d", len(ctx.TrainSet), len(ctx.ValSet))
			}
			if model := ctx.ModelForValue(5); model == nil {
				t.Fatal("no trained model for held-out sweep")
			}
		})
	}
}
