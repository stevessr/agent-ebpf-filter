package ml

import "time"

// LLMReviewSummary aggregates the outcome of an LLM-assisted dataset review.
type LLMReviewSummary struct {
	Source               string    `json:"source"`
	Model                string    `json:"model"`
	ScoredSamples        int       `json:"scoredSamples"`
	AverageRiskScore     float64   `json:"averageRiskScore"`
	Agreement            float64   `json:"agreement"`
	ValidationSplitRatio float64   `json:"validationSplitRatio,omitempty"`
	ReviewedAt           time.Time `json:"reviewedAt"`
}
