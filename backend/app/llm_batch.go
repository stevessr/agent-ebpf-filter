package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section llm_batch.go ----

const (
	defaultLLMBatchScoreLimit         = 20
	maxLLMBatchScoreLimit             = 100
	maxLLMBatchWorkers                = 4
	maxConcurrentLLMBatches           = 2
	maxLLMBatchRequestBodyBytes int64 = 16 << 10
	maxLLMBatchDuration               = 10 * time.Minute
)

var llmBatchSlots = make(chan struct{}, maxConcurrentLLMBatches)

type llmBatchScoreRequest struct {
	Source        string `json:"source"`
	Limit         int    `json:"limit"`
	OnlyUnlabeled bool   `json:"onlyUnlabeled"`
	ApplyLabels   bool   `json:"applyLabels"`
}

type llmBatchScoreResponse struct {
	Source               string               `json:"source"`
	Model                string               `json:"model"`
	Total                int                  `json:"total"`
	Scored               int                  `json:"scored"`
	Applied              int                  `json:"applied"`
	Skipped              int                  `json:"skipped"`
	AverageRiskScore     float64              `json:"averageRiskScore"`
	Agreement            float64              `json:"agreement"`
	ValidationSplitRatio float64              `json:"validationSplitRatio"`
	Review               *LLMReviewSummary    `json:"review,omitempty"`
	Entries              []llmBatchScoreEntry `json:"entries"`
}

type llmScoreSubject struct {
	Index  int
	Sample TrainingSample
}

type llmBatchScoreEntry struct {
	Index             int      `json:"index"`
	CommandLine       string   `json:"commandLine"`
	Comm              string   `json:"comm"`
	Args              []string `json:"args"`
	CurrentLabel      string   `json:"currentLabel"`
	RiskScore         float64  `json:"riskScore,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	RecommendedAction string   `json:"recommendedAction,omitempty"`
	Reasoning         string   `json:"reasoning,omitempty"`
	Error             string   `json:"error,omitempty"`
	Applied           bool     `json:"applied,omitempty"`
}

type llmBatchWorkItem struct {
	position int
	subject  llmScoreSubject
	request  llmScoreRequest
}

type llmBatchWorkResult struct {
	position   int
	subject    llmScoreSubject
	request    llmScoreRequest
	assessment *llmScoringResult
	err        error
}

func handleMLLLMBatchScorePost(c *gin.Context) {
	req, ok := bindLLMBatchScoreRequest(c)
	if !ok {
		return
	}

	resp, err := scoreLLMBatch(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func bindLLMBatchScoreRequest(c *gin.Context) (llmBatchScoreRequest, bool) {
	var req llmBatchScoreRequest
	if status, err := bindLLMJSON(c, &req, maxLLMBatchRequestBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": "invalid request"})
		return req, false
	}
	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	if req.Source == "" {
		req.Source = "training"
	}
	req.Limit = normalizeLLMBatchLimit(req.Limit)
	return req, true
}

func scoreLLMBatch(ctx context.Context, req llmBatchScoreRequest) (*llmBatchScoreResponse, error) {
	if !llmScoringConfigured() {
		return nil, errors.New("LLM scoring is not configured")
	}
	select {
	case llmBatchSlots <- struct{}{}:
		defer func() { <-llmBatchSlots }()
	default:
		return nil, errors.New("LLM batch scoring concurrency limit reached")
	}

	req.Limit = normalizeLLMBatchLimit(req.Limit)
	subjects, validationRatio, err := llmBatchSubjects(req.Source, req.Limit, req.OnlyUnlabeled)
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		return nil, errors.New("no samples available for LLM scoring")
	}

	return scoreLLMSampleSubjects(ctx, req.Source, subjects, req.Limit, req.OnlyUnlabeled, req.ApplyLabels, validationRatio)
}

func scoreLLMSampleSubjects(ctx context.Context, source string, subjects []llmScoreSubject, limit int, onlyUnlabeled, applyLabels bool, validationRatio float64) (*llmBatchScoreResponse, error) {
	cfg := currentMLConfig()
	if !llmScoringConfigured() {
		return nil, errors.New("LLM scoring is not configured")
	}
	if len(subjects) == 0 {
		return nil, errors.New("no samples available for LLM scoring")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, maxLLMBatchDuration)
	defer cancel()
	limit = normalizeLLMBatchLimit(limit)
	if limit > len(subjects) {
		limit = len(subjects)
	}

	skipped := 0
	work := make([]llmBatchWorkItem, 0, limit)
	for _, subject := range subjects {
		if len(work) >= limit {
			break
		}
		if onlyUnlabeled && subject.Sample.IsLabeled() {
			skipped++
			continue
		}
		request := llmScoreRequest{
			CommandLine:  trainingSampleCommandLine(subject.Sample),
			Comm:         subject.Sample.Comm,
			Args:         append([]string(nil), subject.Sample.Args...),
			Category:     subject.Sample.Category,
			AnomalyScore: subject.Sample.AnomalyScore,
			CurrentLabel: sampleLabelName(subject.Sample.Label),
			Source:       source,
		}
		work = append(work, llmBatchWorkItem{position: len(work), subject: subject, request: request})
	}
	if len(work) == 0 {
		return nil, errors.New("no eligible samples available for LLM scoring")
	}
	results, err := runLLMBatchWorkers(ctx, work)
	if err != nil {
		return nil, err
	}

	entries := make([]llmBatchScoreEntry, 0, len(results))
	scored := 0
	applied := 0
	sumRisk := 0.0
	agreed := 0
	considered := 0
	for _, result := range results {
		subject := result.subject
		scoredReq := result.request
		if result.err != nil {
			entries = append(entries, llmBatchScoreEntry{
				Index:        subject.Index,
				CommandLine:  scoredReq.CommandLine,
				Comm:         subject.Sample.Comm,
				Args:         append([]string(nil), subject.Sample.Args...),
				CurrentLabel: sampleLabelName(subject.Sample.Label),
				Error:        result.err.Error(),
			})
			skipped++
			continue
		}
		assessment := result.assessment

		entry := llmBatchScoreEntry{
			Index:             subject.Index,
			CommandLine:       scoredReq.CommandLine,
			Comm:              subject.Sample.Comm,
			Args:              append([]string(nil), subject.Sample.Args...),
			CurrentLabel:      sampleLabelName(subject.Sample.Label),
			RiskScore:         assessment.RiskScore,
			Confidence:        assessment.Confidence,
			RecommendedAction: assessment.RecommendedAction,
			Reasoning:         assessment.Reasoning,
		}

		scored++
		sumRisk += assessment.RiskScore

		if subject.Sample.IsLabeled() {
			considered++
			if labelFromLLMAction(assessment.RecommendedAction, assessment.RiskScore) == subject.Sample.Label {
				agreed++
			}
		}

		if applyLabels && source == "training" && subject.Index >= 0 && !subject.Sample.IsLabeled() {
			if globalTrainingStore != nil {
				if globalTrainingStore.UpdateSampleLabel(subject.Index, labelFromLLMAction(assessment.RecommendedAction, assessment.RiskScore), "llm-score") {
					entry.Applied = true
					applied++
				}
			}
		}

		entries = append(entries, entry)
	}

	review := &LLMReviewSummary{
		Source:               source,
		Model:                strings.TrimSpace(cfg.LlmModel),
		ScoredSamples:        scored,
		AverageRiskScore:     0,
		Agreement:            0,
		ValidationSplitRatio: validationRatio,
		ReviewedAt:           time.Now().UTC(),
	}
	if scored > 0 {
		review.AverageRiskScore = sumRisk / float64(scored)
	}
	if considered > 0 {
		review.Agreement = float64(agreed) / float64(considered)
	}

	resp := &llmBatchScoreResponse{
		Source:               source,
		Model:                strings.TrimSpace(cfg.LlmModel),
		Total:                len(subjects),
		Scored:               scored,
		Applied:              applied,
		Skipped:              skipped,
		AverageRiskScore:     review.AverageRiskScore,
		Agreement:            review.Agreement,
		ValidationSplitRatio: validationRatio,
		Review:               review,
		Entries:              entries,
	}

	if source == "validation" && globalTrainer != nil {
		globalTrainer.setLastLLMReview(review)
	}

	return resp, nil
}

func normalizeLLMBatchLimit(limit int) int {
	if limit <= 0 {
		return defaultLLMBatchScoreLimit
	}
	if limit > maxLLMBatchScoreLimit {
		return maxLLMBatchScoreLimit
	}
	return limit
}

func runLLMBatchWorkers(ctx context.Context, work []llmBatchWorkItem) ([]llmBatchWorkResult, error) {
	if len(work) == 0 {
		return nil, nil
	}
	workers := min(maxLLMBatchWorkers, len(work))
	jobs := make(chan llmBatchWorkItem)
	results := make(chan llmBatchWorkResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				result := scoreLLMBatchWorkItem(ctx, item)
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, item := range work {
			select {
			case jobs <- item:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]llmBatchWorkResult, len(work))
	completed := 0
	for result := range results {
		ordered[result.position] = result
		completed++
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("LLM batch scoring canceled: %w", err)
	}
	if completed != len(work) {
		return nil, fmt.Errorf("LLM batch scoring completed %d of %d work items", completed, len(work))
	}
	return ordered, nil
}

func scoreLLMBatchWorkItem(ctx context.Context, item llmBatchWorkItem) (result llmBatchWorkResult) {
	result = llmBatchWorkResult{position: item.position, subject: item.subject, request: item.request}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.assessment = nil
			result.err = fmt.Errorf("LLM batch worker panic: %v", recovered)
		}
	}()
	result.assessment, result.err = scoreBehaviorWithLLM(ctx, item.request)
	if result.err == nil && result.assessment == nil {
		result.err = errors.New("LLM scoring returned no assessment")
	}
	if result.assessment != nil {
		result.assessment.RawContent = ""
	}
	return result
}

func llmBatchSubjects(source string, limit int, onlyUnlabeled bool) ([]llmScoreSubject, float64, error) {
	cfg := currentMLConfig()
	limit = normalizeLLMBatchLimit(limit)
	switch source {
	case "", "training":
		if globalTrainingStore == nil {
			return nil, 0, errors.New("ML training store not initialized")
		}
		items := globalTrainingStore.BoundedSamplesWithIndex(limit, onlyUnlabeled)
		subjects := make([]llmScoreSubject, 0, len(items))
		for _, item := range items {
			subjects = append(subjects, llmScoreSubject{Index: item.Index, Sample: item.Sample})
		}
		return subjects, cfg.ValidationSplitRatio, nil
	case "validation":
		if globalTrainer == nil {
			return nil, 0, errors.New("ML trainer not initialized")
		}
		items := globalTrainer.BoundedValidationSamples(limit, onlyUnlabeled)
		subjects := make([]llmScoreSubject, 0, len(items))
		for _, sample := range items {
			subjects = append(subjects, llmScoreSubject{Index: -1, Sample: sample})
		}
		return subjects, cfg.ValidationSplitRatio, nil
	default:
		return nil, 0, fmt.Errorf("unsupported llm score source %q", source)
	}
}

func limitLLMSubjects(subjects []llmScoreSubject, limit int) []llmScoreSubject {
	limit = normalizeLLMBatchLimit(limit)
	if limit > len(subjects) {
		return subjects
	}
	return subjects[:limit]
}

func labelFromLLMAction(action string, riskScore float64) int32 {
	switch normalizeLLMAction(action) {
	case "ALLOW":
		return 0
	case "BLOCK":
		return 1
	case "REWRITE":
		return 2
	case "ALERT":
		return 3
	default:
		switch {
		case riskScore >= 80:
			return 1
		case riskScore >= 60:
			return 3
		case riskScore >= 40:
			return 2
		default:
			return 0
		}
	}
}

func llmAssessmentFromScore(result *llmScoringResult, err error) *llmAssessment {
	if err != nil {
		return &llmAssessment{
			Enabled: true,
			Error:   err.Error(),
		}
	}
	if result == nil {
		return &llmAssessment{Enabled: true, Error: "LLM returned no result"}
	}
	return &llmAssessment{
		Enabled:           true,
		Model:             result.Model,
		RiskScore:         result.RiskScore,
		Confidence:        result.Confidence,
		RecommendedAction: result.RecommendedAction,
		Reasoning:         result.Reasoning,
		Signals:           append([]string(nil), result.Signals...),
		RawContent:        result.RawContent,
	}
}

func (t *ModelTrainer) reviewValidationWithLLM(samples []TrainingSample) (*LLMReviewSummary, error) {
	if !llmScoringConfigured() || len(samples) == 0 {
		return nil, nil
	}
	cfg := currentMLConfig()

	limit := defaultLLMReviewLimit
	if len(samples) < limit {
		limit = len(samples)
	}

	subjects := make([]llmScoreSubject, 0, len(samples))
	for i := 0; i < len(samples); i++ {
		subjects = append(subjects, llmScoreSubject{Index: -1, Sample: samples[i]})
	}
	ctx, stop := trainerCancellationContext(t, context.Background())
	defer stop()
	resp, err := scoreLLMSampleSubjects(ctx, "validation", subjects, limit, false, false, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, err
	}
	return resp.Review, nil
}

func trainerCancellationContext(trainer *ModelTrainer, parent context.Context) (context.Context, func()) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	if trainer == nil {
		return ctx, cancel
	}
	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-trainer.cancellationSignal():
			cancel()
		case <-stopWatch:
		}
	}()
	var stopOnce sync.Once
	return ctx, func() {
		stopOnce.Do(func() { close(stopWatch) })
		cancel()
		<-watchDone
	}
}
