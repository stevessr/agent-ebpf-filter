package main

/*
#include <stdint.h>
#include <stdlib.h>

static long long ml_c_linear_bench(const double* features, const double* weights, int n, int dim, int classes, int repeat) {
    long long acc = 0;
    if (!features || !weights || n <= 0 || dim <= 0 || classes <= 0 || classes > 16 || repeat <= 0) return 0;
    for (int rep = 0; rep < repeat; rep++) {
        for (int i = 0; i < n; i++) {
            int best = 0;
            double bestScore = weights[dim];
            for (int d = 0; d < dim; d++) bestScore += weights[d] * features[i*dim+d];
            for (int c = 1; c < classes; c++) {
                const double* w = weights + c*(dim+1);
                double score = w[dim];
                for (int d = 0; d < dim; d++) score += w[d] * features[i*dim+d];
                if (score > bestScore) { bestScore = score; best = c; }
            }
            acc += best;
        }
    }
    return acc;
}

static long long ml_c_forest_bench(
    const double* features,
    const uint8_t* featureIdx,
    const float* threshold,
    const int16_t* left,
    const int16_t* right,
    const float* leaf,
    const int32_t* offsets,
    const int32_t* counts,
    int n,
    int dim,
    int numTrees,
    int classes,
    int repeat
) {
    long long acc = 0;
    if (!features || !featureIdx || !threshold || !left || !right || !leaf || !offsets || !counts) return 0;
    if (n <= 0 || dim <= 0 || numTrees <= 0 || classes <= 0 || classes > 16 || repeat <= 0) return 0;
    for (int rep = 0; rep < repeat; rep++) {
        for (int i = 0; i < n; i++) {
            int votes[16] = {0};
            int valid = 0;
            const double* row = features + i*dim;
            for (int t = 0; t < numTrees; t++) {
                int base = offsets[t];
                int idx = 0;
                int guard = counts[t] + 4;
                while (idx >= 0 && idx < counts[t] && guard-- > 0) {
                    int absIdx = base + idx;
                    if (left[absIdx] == -1 && right[absIdx] == -1) {
                        int cls = (int)(leaf[absIdx] + 0.5f);
                        if (cls >= 0 && cls < classes) { votes[cls]++; valid++; }
                        break;
                    }
                    int fi = (int)featureIdx[absIdx];
                    if (fi < 0 || fi >= dim) break;
                    idx = row[fi] < (double)threshold[absIdx] ? (int)left[absIdx] : (int)right[absIdx];
                }
            }
            int best = 0;
            for (int c = 1; c < classes; c++) if (votes[c] > votes[best]) best = c;
            if (valid > 0) acc += best;
        }
    }
    return acc;
}
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"agent-ebpf-filter/cuda"
)

type MLCRuntimeBackend struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Available   bool   `json:"available"`
	Accelerated bool   `json:"accelerated"`
	Detail      string `json:"detail,omitempty"`
}

type MLCRuntimeStatus struct {
	Available        bool                `json:"available"`
	ActiveBackend    string              `json:"activeBackend"`
	BenchmarkBackend string              `json:"benchmarkBackend"`
	Backends         []MLCRuntimeBackend `json:"backends"`
	ModelType        string              `json:"modelType,omitempty"`
	CSupported       bool                `json:"cSupported"`
	SampleCount      int                 `json:"sampleCount"`
	GoMsPerSample    float64             `json:"goMsPerSample,omitempty"`
	CMsPerSample     float64             `json:"cMsPerSample,omitempty"`
	Speedup          float64             `json:"speedup,omitempty"`
	UpdatedAt        string              `json:"updatedAt,omitempty"`
	Note             string              `json:"note,omitempty"`
}

type intelIGPUStatus struct {
	Detected bool
	Name     string
	OpenCL   bool
}

var (
	mlCRuntimeMu     sync.Mutex
	mlCRuntimeCached MLCRuntimeStatus
	mlCRuntimeAt     time.Time
	mlCRuntimeKey    string
	mlCRuntimeSink   int64
)

func buildMLCRuntimeStatus(model Model, store *TrainingDataStore) MLCRuntimeStatus {
	cacheKey := mlCRuntimeCacheKey(model, store)
	mlCRuntimeMu.Lock()
	defer mlCRuntimeMu.Unlock()
	if time.Since(mlCRuntimeAt) < 5*time.Second && mlCRuntimeCached.UpdatedAt != "" && mlCRuntimeKey == cacheKey {
		return mlCRuntimeCached
	}

	intel := detectIntelIGPU()
	backends := []MLCRuntimeBackend{
		{ID: "c_cpu", Label: "Native C CPU", Available: true, Accelerated: false, Detail: "built-in C inference micro-benchmark"},
		{ID: "cuda", Label: "NVIDIA CUDA", Available: cuda.IsAvailable(), Accelerated: cuda.IsAvailable(), Detail: cuda.DeviceInfo()},
		{ID: "intel_igpu", Label: "Intel iGPU", Available: intel.Detected, Accelerated: intel.Detected && intel.OpenCL, Detail: intel.detail()},
	}
	active := "c_cpu"
	if cuda.IsAvailable() {
		active = "cuda"
	} else if intel.Detected && intel.OpenCL {
		active = "intel_igpu"
	}

	status := MLCRuntimeStatus{
		Available:        true,
		ActiveBackend:    active,
		BenchmarkBackend: "c_cpu",
		Backends:         backends,
		UpdatedAt:        time.Now().Format(time.RFC3339),
		Note:             "C runtime benchmark uses native CPU kernels today; CUDA and Intel iGPU are exposed as selectable hardware backends for supported batch kernels.",
	}
	if model != nil {
		status.ModelType = string(model.Type())
	}
	bench := benchmarkModelCInference(model, store)
	if bench != nil {
		status.CSupported = true
		status.SampleCount = bench.SampleCount
		status.GoMsPerSample = bench.GoMsPerSample
		status.CMsPerSample = bench.CMsPerSample
		status.Speedup = bench.Speedup
	} else if model != nil {
		status.Note = "Current model does not yet have a native C inference kernel; runtime backend detection is still available."
	}

	mlCRuntimeCached = status
	mlCRuntimeAt = time.Now()
	mlCRuntimeKey = cacheKey
	return status
}

func mlCRuntimeCacheKey(model Model, store *TrainingDataStore) string {
	modelType := "none"
	if model != nil {
		modelType = fmt.Sprintf("%T:%p", model, model)
	}
	totalSamples := 0
	labeledSamples := 0
	if store != nil {
		totalSamples, labeledSamples = store.Status()
	}
	return modelType + ":" + strconv.Itoa(totalSamples) + ":" + strconv.Itoa(labeledSamples)
}

func (s intelIGPUStatus) detail() string {
	if !s.Detected {
		return "not detected"
	}
	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = "Intel GPU"
	}
	if s.OpenCL {
		return name + " with OpenCL runtime"
	}
	return name + " detected; OpenCL runtime not found"
}

func detectIntelIGPU() intelIGPUStatus {
	status := intelIGPUStatus{OpenCL: hasOpenCLRuntime()}
	cards, _ := filepath.Glob("/sys/class/drm/card*/device/vendor")
	for _, vendorPath := range cards {
		raw, err := os.ReadFile(vendorPath)
		if err != nil || strings.TrimSpace(string(raw)) != "0x8086" {
			continue
		}
		status.Detected = true
		deviceDir := filepath.Dir(vendorPath)
		if rawName, err := os.ReadFile(filepath.Join(deviceDir, "product_name")); err == nil {
			status.Name = strings.TrimSpace(string(rawName))
		} else if rawName, err := os.ReadFile(filepath.Join(deviceDir, "uevent")); err == nil {
			status.Name = firstIntelGPUUeventName(string(rawName))
		}
		if status.Name == "" {
			status.Name = filepath.Base(filepath.Dir(deviceDir))
		}
		return status
	}
	return status
}

func firstIntelGPUUeventName(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "DRIVER=") {
			return "Intel " + strings.TrimPrefix(line, "DRIVER=")
		}
	}
	return ""
}

func hasOpenCLRuntime() bool {
	candidates := []string{
		"/usr/lib/x86_64-linux-gnu/libOpenCL.so.1",
		"/usr/lib/x86_64-linux-gnu/libOpenCL.so",
		"/usr/lib64/libOpenCL.so.1",
		"/usr/lib/libOpenCL.so.1",
		"/opt/intel/oneapi/compiler/latest/linux/lib/libOpenCL.so",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	if entries, err := filepath.Glob("/etc/OpenCL/vendors/*.icd"); err == nil && len(entries) > 0 {
		return true
	}
	return false
}

type mlCInferenceBenchmark struct {
	SampleCount   int
	GoMsPerSample float64
	CMsPerSample  float64
	Speedup       float64
}

func benchmarkModelCInference(model Model, store *TrainingDataStore) *mlCInferenceBenchmark {
	if model == nil || store == nil {
		return nil
	}
	samples := store.LabeledSamples()
	if len(samples) == 0 {
		return nil
	}
	if len(samples) > 128 {
		samples = samples[:128]
	}
	features := flattenRuntimeFeatures(samples)
	if len(features) == 0 {
		return nil
	}
	repeat := 4096 / len(samples)
	if repeat < 1 {
		repeat = 1
	}

	goStart := time.Now()
	var goAcc int64
	for r := 0; r < repeat; r++ {
		for _, sample := range samples {
			goAcc += int64(model.Predict(sample.Features).Action)
		}
	}
	goElapsed := time.Since(goStart)
	mlCRuntimeSink += goAcc

	var cElapsed time.Duration
	var cAcc int64
	inner := unwrapModelType(model)
	switch m := inner.(type) {
	case *DecisionForest:
		cElapsed, cAcc = benchCDecisionForest(m, features, len(samples), repeat)
	case *ExtraTreesModel:
		if m.Forest == nil {
			return nil
		}
		cElapsed, cAcc = benchCDecisionForest(m.Forest, features, len(samples), repeat)
	case *LogisticModel:
		cElapsed, cAcc = benchCLinearWeights(m.Weights, m.NumClasses, features, len(samples), repeat)
	case *SVMModel:
		cElapsed, cAcc = benchCLinearWeights(m.Weights, m.Classes, features, len(samples), repeat)
	case *RidgeModel:
		cElapsed, cAcc = benchCLinearWeights(m.Weights, m.Classes, features, len(samples), repeat)
	case *PerceptronModel:
		cElapsed, cAcc = benchCLinearWeights(m.Weights, m.Classes, features, len(samples), repeat)
	case *PAModel:
		cElapsed, cAcc = benchCLinearWeights(m.Weights, m.Classes, features, len(samples), repeat)
	default:
		return nil
	}
	if cElapsed <= 0 {
		return nil
	}
	mlCRuntimeSink += cAcc

	predictions := float64(len(samples) * repeat)
	goMs := float64(goElapsed.Nanoseconds()) / 1e6 / predictions
	cMs := float64(cElapsed.Nanoseconds()) / 1e6 / predictions
	speedup := 0.0
	if cMs > 0 {
		speedup = goMs / cMs
	}
	return &mlCInferenceBenchmark{SampleCount: len(samples), GoMsPerSample: goMs, CMsPerSample: cMs, Speedup: speedup}
}

func flattenRuntimeFeatures(samples []TrainingSample) []float64 {
	out := make([]float64, 0, len(samples)*FeatureDim)
	for _, sample := range samples {
		for _, v := range sample.Features {
			out = append(out, v)
		}
	}
	return out
}

func benchCLinearWeights(weights [][FeatureDim + 1]float64, classes int, features []float64, n int, repeat int) (time.Duration, int64) {
	if len(weights) == 0 || classes <= 0 || n <= 0 || len(features) == 0 {
		return 0, 0
	}
	flatWeights := make([]float64, 0, classes*(FeatureDim+1))
	for c := 0; c < classes && c < len(weights); c++ {
		for d := 0; d <= FeatureDim; d++ {
			flatWeights = append(flatWeights, weights[c][d])
		}
	}
	if len(flatWeights) == 0 {
		return 0, 0
	}
	start := time.Now()
	acc := C.ml_c_linear_bench(
		(*C.double)(unsafe.Pointer(&features[0])),
		(*C.double)(unsafe.Pointer(&flatWeights[0])),
		C.int(n), C.int(FeatureDim), C.int(classes), C.int(repeat),
	)
	return time.Since(start), int64(acc)
}

func benchCDecisionForest(forest *DecisionForest, features []float64, n int, repeat int) (time.Duration, int64) {
	if forest == nil || len(forest.Trees) == 0 || n <= 0 || len(features) == 0 {
		return 0, 0
	}
	featureIdx := make([]uint8, 0, 1024)
	thresholds := make([]float32, 0, 1024)
	left := make([]int16, 0, 1024)
	right := make([]int16, 0, 1024)
	leaf := make([]float32, 0, 1024)
	offsets := make([]int32, 0, len(forest.Trees))
	counts := make([]int32, 0, len(forest.Trees))
	for _, tree := range forest.Trees {
		offsets = append(offsets, int32(len(featureIdx)))
		counts = append(counts, int32(len(tree.Nodes)))
		for _, node := range tree.Nodes {
			featureIdx = append(featureIdx, node.FeatureIndex)
			thresholds = append(thresholds, node.Threshold)
			left = append(left, node.LeftChild)
			right = append(right, node.RightChild)
			leaf = append(leaf, node.LeafValue)
		}
	}
	if len(featureIdx) == 0 {
		return 0, 0
	}
	classes := forest.NumClasses
	if classes <= 0 {
		classes = 4
	}
	start := time.Now()
	acc := C.ml_c_forest_bench(
		(*C.double)(unsafe.Pointer(&features[0])),
		(*C.uint8_t)(unsafe.Pointer(&featureIdx[0])),
		(*C.float)(unsafe.Pointer(&thresholds[0])),
		(*C.int16_t)(unsafe.Pointer(&left[0])),
		(*C.int16_t)(unsafe.Pointer(&right[0])),
		(*C.float)(unsafe.Pointer(&leaf[0])),
		(*C.int32_t)(unsafe.Pointer(&offsets[0])),
		(*C.int32_t)(unsafe.Pointer(&counts[0])),
		C.int(n), C.int(FeatureDim), C.int(len(forest.Trees)), C.int(classes), C.int(repeat),
	)
	return time.Since(start), int64(acc)
}
