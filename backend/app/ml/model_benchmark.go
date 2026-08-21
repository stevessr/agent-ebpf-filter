package ml

import "time"

// BenchmarkModelInference measures predict throughput/latency for the given samples.
func BenchmarkModelInference(model Model, samples []TrainingSample) (float64, float64, float64, int) {
	if model == nil || len(samples) == 0 {
		return 0, 0, 0, 0
	}
	warmup := 8
	if warmup > len(samples) {
		warmup = len(samples)
	}
	for i := 0; i < warmup; i++ {
		_ = model.Predict(samples[i].Features)
	}

	const targetPredictions = 256
	rounds := targetPredictions / len(samples)
	if targetPredictions%len(samples) != 0 {
		rounds++
	}
	if rounds < 1 {
		rounds = 1
	}

	totalPredictions := 0
	start := time.Now()
	for r := 0; r < rounds; r++ {
		for _, sample := range samples {
			_ = model.Predict(sample.Features)
			totalPredictions++
		}
	}
	duration := time.Since(start).Seconds()
	if duration <= 0 {
		duration = 1e-9
	}
	throughput := float64(totalPredictions) / duration
	latencyMs := duration * 1000 / float64(totalPredictions)
	return duration, throughput, latencyMs, totalPredictions
}
