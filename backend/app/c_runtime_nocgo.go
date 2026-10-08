//go:build !cgo

package app

import (
	"time"

	"agent-ebpf-filter/app/ml"
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

func buildMLCRuntimeStatus(model ml.Model, store *ml.TrainingDataStore) MLCRuntimeStatus {
	status := MLCRuntimeStatus{
		Available:        false,
		ActiveBackend:    "go_cpu",
		BenchmarkBackend: "go_cpu",
		Backends: []MLCRuntimeBackend{
			{
				ID:          "go_cpu",
				Label:       "Go CPU",
				Available:   true,
				Accelerated: false,
				Detail:      "pure-Go embedded backend",
			},
			{
				ID:          "c_cpu",
				Label:       "Native C CPU",
				Available:   false,
				Accelerated: false,
				Detail:      "unavailable because this build has CGO disabled",
			},
		},
		UpdatedAt: time.Now().Format(time.RFC3339),
		Note:      "Native C/CUDA inference micro-benchmarks are disabled in the pure-Go single-file desktop build; Go model inference remains available.",
	}
	if model != nil {
		status.ModelType = string(model.Type())
	}
	if store != nil {
		total, _ := store.Status()
		status.SampleCount = total
	}
	return status
}
