package app

import (
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section llm.go ----

const defaultLLMScoringSystemPrompt = "You are a cybersecurity behavior analyst. Score command behavior for security risk and return strict JSON only."
const defaultLLMReviewLimit = 50

const (
	maxLLMRequestBodyBytes    int64 = 128 << 10
	maxLLMResponseBodyBytes   int64 = 256 << 10
	maxLLMSystemPromptBytes         = 16 << 10
	maxLLMCommandLineBytes          = 32 << 10
	maxLLMArgumentCount             = 256
	maxLLMArgumentBytes             = 4 << 10
	maxLLMArgumentsTotalBytes       = 64 << 10
	maxLLMBaseURLBytes              = 4 << 10
	maxLLMModelBytes                = 1024
	maxLLMAPIKeyBytes               = 16 << 10
	maxLLMReasoningBytes            = 8 << 10
	maxLLMSignalCount               = 32
	maxLLMSignalBytes               = 512
	maxLLMTimeoutSeconds            = 120
	maxConcurrentLLMRequests        = 4
)

var (
	llmOutboundSlots = make(chan struct{}, maxConcurrentLLMRequests)
	llmHTTPClient    = &http.Client{}
)

type llmScoreRequest struct {
	CommandLine    string                     `json:"commandLine"`
	Comm           string                     `json:"comm"`
	Args           []string                   `json:"args"`
	Category       string                     `json:"category"`
	AnomalyScore   float64                    `json:"anomalyScore"`
	Classification *pb.BehaviorClassification `json:"classification,omitempty"`
	MlAction       string                     `json:"mlAction,omitempty"`
	MlConfidence   float64                    `json:"mlConfidence,omitempty"`
	NetworkRisk    string                     `json:"networkRisk,omitempty"`
	NetworkScore   float64                    `json:"networkScore,omitempty"`
	SampleEvidence string                     `json:"sampleEvidence,omitempty"`
	CurrentLabel   string                     `json:"currentLabel,omitempty"`
	Source         string                     `json:"source,omitempty"`
}

type llmScoringResult struct {
	Model             string   `json:"model,omitempty"`
	RiskScore         float64  `json:"riskScore"`
	Confidence        float64  `json:"confidence"`
	RecommendedAction string   `json:"recommendedAction"`
	Reasoning         string   `json:"reasoning"`
	Signals           []string `json:"signals,omitempty"`
	RawContent        string   `json:"rawContent,omitempty"`
}

type llmAssessment struct {
	Enabled           bool     `json:"enabled"`
	Model             string   `json:"model,omitempty"`
	RiskScore         float64  `json:"riskScore"`
	Confidence        float64  `json:"confidence"`
	RecommendedAction string   `json:"recommendedAction"`
	Reasoning         string   `json:"reasoning"`
	Signals           []string `json:"signals,omitempty"`
	Error             string   `json:"error,omitempty"`
	RawContent        string   `json:"rawContent,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func bindLLMScoreRequest(c *gin.Context) (llmScoreRequest, bool) {
	var req llmScoreRequest
	if status, err := bindLLMJSON(c, &req, maxLLMRequestBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": "invalid request"})
		return req, false
	}

	req.CommandLine = strings.TrimSpace(req.CommandLine)
	req.Comm = strings.TrimSpace(req.Comm)
	if req.CommandLine == "" && req.Comm == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commandLine or comm is required"})
		return req, false
	}
	if req.Comm == "" && req.CommandLine != "" {
		req.Comm, req.Args = behavior.SplitCommandLine(req.CommandLine)[0], behavior.SplitCommandLine(req.CommandLine)[1:]
	}
	if req.CommandLine == "" {
		req.CommandLine = behavior.JoinCommandLine(req.Comm, req.Args)
	}
	if err := validateLLMCommandInput(req.CommandLine, req.Comm, req.Args); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return req, false
	}
	return req, true
}

func bindLLMJSON(c *gin.Context, target any, maxBytes int64) (int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func bindOptionalLLMJSON(c *gin.Context, target any, maxBytes int64) (int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		if errors.Is(err, io.EOF) {
			return http.StatusOK, nil
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func validateLLMCommandInput(commandLine, comm string, args []string) error {
	if len(commandLine) > maxLLMCommandLineBytes {
		return fmt.Errorf("command line exceeds %d bytes", maxLLMCommandLineBytes)
	}
	if len(comm) > maxLLMArgumentBytes {
		return fmt.Errorf("command name exceeds %d bytes", maxLLMArgumentBytes)
	}
	if len(args) > maxLLMArgumentCount {
		return fmt.Errorf("command arguments exceed %d items", maxLLMArgumentCount)
	}
	total := 0
	for _, arg := range args {
		if len(arg) > maxLLMArgumentBytes {
			return fmt.Errorf("command argument exceeds %d bytes", maxLLMArgumentBytes)
		}
		total += len(arg)
		if total > maxLLMArgumentsTotalBytes {
			return fmt.Errorf("command arguments exceed %d total bytes", maxLLMArgumentsTotalBytes)
		}
	}
	return nil
}

func handleMLLLMScorePost(c *gin.Context) {
	req, ok := bindLLMScoreRequest(c)
	if !ok {
		return
	}

	result, err := scoreBehaviorWithLLM(c.Request.Context(), llmScoreRequest{
		CommandLine:    req.CommandLine,
		Comm:           req.Comm,
		Args:           req.Args,
		Category:       req.Category,
		AnomalyScore:   req.AnomalyScore,
		Classification: req.Classification,
		MlAction:       req.MlAction,
		MlConfidence:   req.MlConfidence,
		NetworkRisk:    req.NetworkRisk,
		NetworkScore:   req.NetworkScore,
		SampleEvidence: req.SampleEvidence,
		CurrentLabel:   req.CurrentLabel,
		Source:         req.Source,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func llmScoringConfigured() bool {
	cfg := currentMLConfig()
	return cfg.LlmEnabled && strings.TrimSpace(cfg.LlmBaseURL) != "" && strings.TrimSpace(cfg.LlmModel) != ""
}

func validateLLMRuntimeConfig(cfg MLConfig) error {
	if len(cfg.LlmBaseURL) > maxLLMBaseURLBytes {
		return errors.New("LLM base URL exceeds size limit")
	}
	if len(cfg.LlmModel) > maxLLMModelBytes {
		return errors.New("LLM model name exceeds size limit")
	}
	if len(cfg.LlmAPIKey) > maxLLMAPIKeyBytes {
		return errors.New("LLM API key exceeds size limit")
	}
	if len(cfg.LlmSystemPrompt) > maxLLMSystemPromptBytes {
		return errors.New("LLM system prompt exceeds size limit")
	}
	return nil
}

func normalizedLLMCompletionURL(rawBaseURL string) (string, error) {
	base := strings.TrimSpace(rawBaseURL)
	if base == "" {
		return "", errors.New("LLM base URL is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported LLM URL scheme: %s", parsed.Scheme)
	}

	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base, nil
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/chat/completions", nil
}

func scoreBehaviorWithLLM(ctx context.Context, req llmScoreRequest) (*llmScoringResult, error) {
	cfg := currentMLConfig()
	if !llmScoringConfigured() {
		return nil, errors.New("LLM scoring is not configured")
	}
	if err := validateLLMRuntimeConfig(cfg); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateLLMCommandInput(req.CommandLine, req.Comm, req.Args); err != nil {
		return nil, err
	}
	timeout := boundedLLMTimeout(cfg.LlmTimeoutSeconds)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := acquireLLMOutboundSlot(ctx); err != nil {
		return nil, err
	}
	defer releaseLLMOutboundSlot()

	endpoint, err := normalizedLLMCompletionURL(cfg.LlmBaseURL)
	if err != nil {
		return nil, err
	}

	sysPrompt := strings.TrimSpace(cfg.LlmSystemPrompt)
	if sysPrompt == "" {
		sysPrompt = defaultLLMScoringSystemPrompt
	}
	if len(sysPrompt) > maxLLMSystemPromptBytes {
		return nil, fmt.Errorf("LLM system prompt exceeds %d bytes", maxLLMSystemPromptBytes)
	}

	contentJSON, err := json.MarshalIndent(buildLLMBehaviorContext(req), "", "  ")
	if err != nil {
		return nil, err
	}

	prompt := buildLLMScoringPrompt(string(contentJSON))
	openAIReq := openAIChatRequest{
		Model:       strings.TrimSpace(cfg.LlmModel),
		Messages:    []openAIChatMessage{{Role: "system", Content: sysPrompt}, {Role: "user", Content: prompt}},
		Temperature: clampFloat64(cfg.LlmTemperature, 0, 2),
		MaxTokens:   clampInt(cfg.LlmMaxTokens, 32, 4096),
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxLLMRequestBodyBytes {
		return nil, errors.New("LLM API request exceeds size limit")
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	reqHTTP.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(cfg.LlmAPIKey); key != "" {
		reqHTTP.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := llmHTTPClient.Do(reqHTTP)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := ioReadLimit(resp.Body, 4096)
		msg := strings.TrimSpace(string(payload))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("LLM API request failed: %s: %s", resp.Status, msg)
	}

	var openAIResp openAIChatResponse
	if err := decodeBoundedLLMResponse(resp.Body, &openAIResp); err != nil {
		return nil, err
	}
	if openAIResp.Error != nil && strings.TrimSpace(openAIResp.Error.Message) != "" {
		return nil, errors.New(truncateLLMText(openAIResp.Error.Message, maxLLMReasoningBytes))
	}
	if len(openAIResp.Choices) == 0 {
		return nil, errors.New("LLM API returned no choices")
	}

	content := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	result, err := parseLLMScoreContent(content)
	if err != nil {
		return nil, err
	}
	result.Model = openAIReq.Model
	result.RawContent = content
	return result, nil
}

func boundedLLMTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = 45
	}
	return time.Duration(clampInt(seconds, 5, maxLLMTimeoutSeconds)) * time.Second
}

func acquireLLMOutboundSlot(ctx context.Context) error {
	select {
	case llmOutboundSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for LLM request slot: %w", ctx.Err())
	}
}

func releaseLLMOutboundSlot() {
	<-llmOutboundSlots
}

func decodeBoundedLLMResponse(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maxLLMResponseBodyBytes+1))
	if err != nil {
		return err
	}
	if int64(len(payload)) > maxLLMResponseBodyBytes {
		return errors.New("LLM API response exceeds size limit")
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return err
	}
	return nil
}

func buildLLMBehaviorContext(req llmScoreRequest) llmScoreContext {
	return llmScoreContext{
		CommandLine:    strings.TrimSpace(req.CommandLine),
		Comm:           strings.TrimSpace(req.Comm),
		Args:           append([]string(nil), req.Args...),
		Category:       strings.TrimSpace(req.Category),
		AnomalyScore:   req.AnomalyScore,
		Classification: req.Classification,
		MlAction:       strings.TrimSpace(req.MlAction),
		MlConfidence:   req.MlConfidence,
		NetworkRisk:    strings.TrimSpace(req.NetworkRisk),
		NetworkScore:   req.NetworkScore,
		SampleEvidence: strings.TrimSpace(req.SampleEvidence),
		CurrentLabel:   strings.TrimSpace(req.CurrentLabel),
		Source:         strings.TrimSpace(req.Source),
	}
}

type llmScoreContext struct {
	CommandLine    string                     `json:"commandLine"`
	Comm           string                     `json:"comm"`
	Args           []string                   `json:"args"`
	Category       string                     `json:"category,omitempty"`
	AnomalyScore   float64                    `json:"anomalyScore,omitempty"`
	Classification *pb.BehaviorClassification `json:"classification,omitempty"`
	MlAction       string                     `json:"mlAction,omitempty"`
	MlConfidence   float64                    `json:"mlConfidence,omitempty"`
	NetworkRisk    string                     `json:"networkRisk,omitempty"`
	NetworkScore   float64                    `json:"networkScore,omitempty"`
	SampleEvidence string                     `json:"sampleEvidence,omitempty"`
	CurrentLabel   string                     `json:"currentLabel,omitempty"`
	Source         string                     `json:"source,omitempty"`
}

func buildLLMScoringPrompt(contextJSON string) string {
	return fmt.Sprintf(`Score the following behavior for security risk.

Return STRICT JSON only with these keys:
- riskScore: number from 0 to 100
- recommendedAction: one of ALLOW, ALERT, BLOCK, REWRITE
- confidence: number from 0 to 1
- reasoning: short explanation
- signals: array of concise strings

Behavior context:
%s
`, contextJSON)
}

func parseLLMScoreContent(content string) (*llmScoringResult, error) {
	jsonPayload := extractJSONObject(content)
	if jsonPayload == "" {
		return nil, errors.New("LLM response did not contain JSON")
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return nil, err
	}

	result := &llmScoringResult{
		RiskScore:         clampFloat64(extractLLMFloat(raw, "riskScore", "risk_score", "score", "risk"), 0, 100),
		Confidence:        clampFloat64(extractLLMFloat(raw, "confidence", "conf"), 0, 1),
		RecommendedAction: normalizeLLMAction(extractLLMString(raw, "recommendedAction", "recommended_action", "action", "label")),
		Reasoning:         truncateLLMText(strings.TrimSpace(extractLLMString(raw, "reasoning", "explanation", "analysis")), maxLLMReasoningBytes),
		Signals:           extractLLMStrings(raw, "signals", "evidence", "reasons"),
	}
	if result.RecommendedAction == "" {
		result.RecommendedAction = llmActionFromRiskScore(result.RiskScore)
	}
	if result.Confidence == 0 && result.RiskScore > 0 {
		result.Confidence = clampFloat64(result.RiskScore/100.0, 0.1, 0.99)
	}
	if result.Reasoning == "" {
		result.Reasoning = "LLM returned a risk score without reasoning"
	}
	return result, nil
}

func extractJSONObject(content string) string {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func extractLLMString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func extractLLMFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			switch v := val.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			case int:
				return float64(v)
			case int64:
				return float64(v)
			case json.Number:
				if f, err := v.Float64(); err == nil {
					return f
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func extractLLMStrings(raw map[string]any, keys ...string) []string {
	for _, key := range keys {
		val, ok := raw[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case []any:
			out := make([]string, 0, min(len(v), maxLLMSignalCount))
			for _, item := range v {
				if s, ok := item.(string); ok {
					if trimmed := strings.TrimSpace(s); trimmed != "" {
						out = append(out, truncateLLMText(trimmed, maxLLMSignalBytes))
						if len(out) >= maxLLMSignalCount {
							break
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case []string:
			out := make([]string, 0, min(len(v), maxLLMSignalCount))
			for _, s := range v {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					out = append(out, truncateLLMText(trimmed, maxLLMSignalBytes))
					if len(out) >= maxLLMSignalCount {
						break
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return []string{truncateLLMText(trimmed, maxLLMSignalBytes)}
			}
		}
	}
	return nil
}

func truncateLLMText(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return strings.ToValidUTF8(value[:maxBytes], "")
}

func normalizeLLMAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ALLOW", "BLOCK", "ALERT", "REWRITE":
		return strings.ToUpper(strings.TrimSpace(action))
	default:
		return ""
	}
}

func llmActionFromRiskScore(score float64) string {
	switch {
	case score >= 80:
		return "BLOCK"
	case score >= 60:
		return "ALERT"
	case score >= 40:
		return "REWRITE"
	default:
		return "ALLOW"
	}
}

func clampFloat64(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ioReadLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 4096
	}
	return io.ReadAll(io.LimitReader(r, limit))
}
