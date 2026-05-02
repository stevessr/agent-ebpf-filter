package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultLLMProductionDatasetLimit = 500

type llmProductionDatasetRequest struct {
	Limit          int  `json:"limit"`
	AllowHeuristic bool `json:"allowHeuristic"`
	Deduplicate    bool `json:"deduplicate"`
}

type llmProductionDatasetRow struct {
	Index            int                 `json:"index"`
	CommandLine      string              `json:"commandLine"`
	Comm             string              `json:"comm"`
	Args             []string            `json:"args"`
	Label            string              `json:"label"`
	Category         string              `json:"category"`
	AnomalyScore     float64             `json:"anomalyScore"`
	Timestamp        string              `json:"timestamp"`
	UserLabel        string              `json:"userLabel"`
	TargetRiskScore  float64             `json:"targetRiskScore"`
	TargetConfidence float64             `json:"targetConfidence"`
	Reasoning        string              `json:"reasoning"`
	Signals          []string            `json:"signals"`
	Prompt           string              `json:"prompt"`
	Completion       string              `json:"completion"`
	Messages         []openAIChatMessage `json:"messages"`
}

type llmProductionDatasetResponse struct {
	Source            string                    `json:"source"`
	Format            string                    `json:"format"`
	ContentType       string                    `json:"contentType"`
	Total             int                       `json:"total"`
	Limit             int                       `json:"limit"`
	Truncated         bool                      `json:"truncated"`
	Included          int                       `json:"included"`
	SkippedUnlabeled  int                       `json:"skippedUnlabeled"`
	SkippedHeuristic  int                       `json:"skippedHeuristic"`
	SkippedDuplicates int                       `json:"skippedDuplicates"`
	SystemPrompt      string                    `json:"systemPrompt"`
	Rows              []llmProductionDatasetRow `json:"rows"`
}

func bindLLMProductionDatasetRequest(c *gin.Context) (llmProductionDatasetRequest, bool) {
	var req llmProductionDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return req, false
	}
	req.Limit = parseLLMProductionDatasetLimit(req.Limit)
	return req, true
}

func handleMLLLMProductionDatasetPullPost(c *gin.Context) {
	req, ok := bindLLMProductionDatasetRequest(c)
	if !ok {
		return
	}

	resp, err := buildLLMProductionDataset(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func buildLLMProductionDataset(req llmProductionDatasetRequest) (*llmProductionDatasetResponse, error) {
	if globalTrainingStore == nil {
		return nil, errors.New("ML training store not initialized")
	}

	items := globalTrainingStore.AllSamplesWithIndex()
	total := len(items)
	limit := parseLLMProductionDatasetLimit(req.Limit)
	truncated := false
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		truncated = true
	}

	systemPrompt := strings.TrimSpace(mlConfig.LlmSystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultLLMScoringSystemPrompt
	}

	rows := make([]llmProductionDatasetRow, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	skippedUnlabeled := 0
	skippedHeuristic := 0
	skippedDuplicates := 0

	for _, item := range items {
		sample := item.Sample
		if !sample.IsLabeled() {
			skippedUnlabeled++
			continue
		}
		if !req.AllowHeuristic && isLLMProductionHeuristicSource(sample.UserLabel) {
			skippedHeuristic++
			continue
		}

		key := commandKey(sample.Comm, sample.Args) + "\x00" + sampleLabelName(sample.Label)
		if req.Deduplicate {
			if _, exists := seen[key]; exists {
				skippedDuplicates++
				continue
			}
			seen[key] = struct{}{}
		}

		rows = append(rows, buildLLMProductionDatasetRow(item.Index, sample, systemPrompt))
	}

	return &llmProductionDatasetResponse{
		Source:            "local-training-store",
		Format:            "jsonl",
		ContentType:       "application/x-ndjson",
		Total:             total,
		Limit:             limit,
		Truncated:         truncated,
		Included:          len(rows),
		SkippedUnlabeled:  skippedUnlabeled,
		SkippedHeuristic:  skippedHeuristic,
		SkippedDuplicates: skippedDuplicates,
		SystemPrompt:      systemPrompt,
		Rows:              rows,
	}, nil
}

func buildLLMProductionDatasetRow(index int, sample TrainingSample, systemPrompt string) llmProductionDatasetRow {
	comm, args := normalizeCommandInput("", sample.Comm, sample.Args)
	commandLine := trainingSampleCommandLine(sample)
	if strings.TrimSpace(commandLine) == "" {
		commandLine = joinCommandLine(comm, args)
	}

	category := strings.TrimSpace(sample.Category)
	if category == "" {
		category = ClassifyBehavior(comm, args).PrimaryCategory
	}

	label := sampleLabelName(sample.Label)
	if label == "" {
		label = "-"
	}

	context := buildLLMBehaviorContext(llmScoreRequest{
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         append([]string(nil), args...),
		Category:     category,
		AnomalyScore: sample.AnomalyScore,
		CurrentLabel: label,
		Source:       sample.UserLabel,
	})
	contextJSON, err := json.MarshalIndent(context, "", "  ")
	if err != nil {
		contextJSON = []byte("{}")
	}

	prompt := buildLLMScoringPrompt(string(contextJSON))
	targetRiskScore := llmProductionTargetRiskScore(label)
	targetConfidence := llmProductionTargetConfidence(sample.UserLabel)
	reasoning := llmProductionReasoning(label, sample.UserLabel, category, sample.AnomalyScore)
	signals := llmProductionSignals(label, sample.UserLabel, category, sample.AnomalyScore)

	completionPayload := map[string]any{
		"riskScore":         targetRiskScore,
		"recommendedAction": label,
		"confidence":        targetConfidence,
		"reasoning":         reasoning,
		"signals":           signals,
	}
	completionJSON, err := json.Marshal(completionPayload)
	if err != nil {
		completionJSON = []byte("{}")
	}

	timestamp := sample.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return llmProductionDatasetRow{
		Index:            index,
		CommandLine:      commandLine,
		Comm:             comm,
		Args:             append([]string(nil), args...),
		Label:            label,
		Category:         category,
		AnomalyScore:     sample.AnomalyScore,
		Timestamp:        timestamp.Format(time.RFC3339),
		UserLabel:        sample.UserLabel,
		TargetRiskScore:  targetRiskScore,
		TargetConfidence: targetConfidence,
		Reasoning:        reasoning,
		Signals:          signals,
		Prompt:           prompt,
		Completion:       string(completionJSON),
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
			{Role: "assistant", Content: string(completionJSON)},
		},
	}
}

func llmProductionTargetRiskScore(label string) float64 {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "BLOCK":
		return 95
	case "ALERT":
		return 70
	case "REWRITE":
		return 50
	default:
		return 10
	}
}

func llmProductionTargetConfidence(userLabel string) float64 {
	switch strings.ToLower(strings.TrimSpace(userLabel)) {
	case "manual", "manual-index", "accepted", "rejected", "alerted":
		return 0.99
	case "remote-source-label", "loaded":
		return 0.96
	case "llm-score":
		return 0.86
	case "remote-heuristic", "heuristic", "auto", "automatic":
		return 0.78
	default:
		return 0.92
	}
}

func llmProductionReasoning(label, userLabel, category string, anomalyScore float64) string {
	parts := []string{}
	if userLabel != "" {
		parts = append(parts, "source="+userLabel)
	}
	if category != "" {
		parts = append(parts, "category="+category)
	}
	parts = append(parts, fmt.Sprintf("anomaly=%.2f", anomalyScore))

	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "BLOCK":
		parts = append(parts, "destructive or privilege-sensitive pattern")
	case "ALERT":
		parts = append(parts, "suspicious pattern that deserves review")
	case "REWRITE":
		parts = append(parts, "safer rewrite preferred")
	default:
		parts = append(parts, "benign command")
	}
	return strings.Join(parts, "; ")
}

func llmProductionSignals(label, userLabel, category string, anomalyScore float64) []string {
	signals := []string{
		"label:" + strings.ToUpper(strings.TrimSpace(label)),
	}
	if userLabel != "" {
		signals = append(signals, "source:"+userLabel)
	}
	if category != "" {
		signals = append(signals, "category:"+category)
	}
	signals = append(signals, fmt.Sprintf("anomaly:%.2f", anomalyScore))
	return signals
}

func isLLMProductionHeuristicSource(userLabel string) bool {
	switch strings.ToLower(strings.TrimSpace(userLabel)) {
	case "remote-heuristic", "llm-score", "heuristic", "auto", "automatic":
		return true
	default:
		return false
	}
}

func parseLLMProductionDatasetLimit(limit int) int {
	if limit <= 0 {
		return defaultLLMProductionDatasetLimit
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}
