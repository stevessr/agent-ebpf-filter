package ml

import "math"

// SelectBenchmarkSamples picks `target` evenly-spaced samples (by index)
// from the training set; used by sweep evaluation and auto-tune.
func SelectBenchmarkSamples(samples []TrainingSample, target int) []TrainingSample {
	if target <= 0 || len(samples) == 0 {
		return nil
	}
	if target >= len(samples) {
		return append([]TrainingSample(nil), samples...)
	}
	if target == 1 {
		return []TrainingSample{samples[len(samples)/2]}
	}
	out := make([]TrainingSample, 0, target)
	for i := 0; i < target; i++ {
		idx := int(math.Round(float64(i) * float64(len(samples)-1) / float64(target-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(samples) {
			idx = len(samples) - 1
		}
		out = append(out, samples[idx])
	}
	return out
}
