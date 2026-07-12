package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const testLLMResponse = `{"choices":[{"message":{"content":"{\"riskScore\":10,\"confidence\":0.9,\"recommendedAction\":\"ALLOW\",\"reasoning\":\"safe\"}"}}]}`

func withTestLLMConfig(t *testing.T, baseURL string) {
	t.Helper()
	runtimeSettingsStore.mu.Lock()
	old := runtimeSettingsStore.settings
	runtimeSettingsStore.settings.MLConfig = MLConfig{
		LlmEnabled:        true,
		LlmBaseURL:        baseURL,
		LlmModel:          "test-model",
		LlmTimeoutSeconds: 5,
		LlmMaxTokens:      256,
		LlmSystemPrompt:   defaultLLMScoringSystemPrompt,
	}
	runtimeSettingsStore.mu.Unlock()
	t.Cleanup(func() {
		runtimeSettingsStore.mu.Lock()
		runtimeSettingsStore.settings = old
		runtimeSettingsStore.mu.Unlock()
	})
}

func testLLMScoreRequest() llmScoreRequest {
	return llmScoreRequest{CommandLine: "git status", Comm: "git", Args: []string{"git", "status"}}
}

func TestHandleMLLLMScoreRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/score", handleMLLLMScorePost)
	body := append([]byte(`{"commandLine":"`), bytes.Repeat([]byte("x"), int(maxLLMRequestBodyBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/score", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScoreBehaviorWithLLMBoundsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), int(maxLLMResponseBodyBytes+1)))
	}))
	defer server.Close()
	withTestLLMConfig(t, server.URL)
	_, err := scoreBehaviorWithLLM(context.Background(), testLLMScoreRequest())
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestScoreBehaviorWithLLMBoundsGlobalConcurrency(t *testing.T) {
	var active atomic.Int32
	var peak atomic.Int32
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testLLMResponse)
	}))
	defer server.Close()
	withTestLLMConfig(t, server.URL)

	const requests = maxConcurrentLLMRequests * 2
	errs := make(chan error, requests)
	for i := 0; i < requests; i++ {
		go func() {
			_, err := scoreBehaviorWithLLM(context.Background(), testLLMScoreRequest())
			errs <- err
		}()
	}
	for i := 0; i < maxConcurrentLLMRequests; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("timed out waiting for bounded LLM requests")
		}
	}
	select {
	case <-entered:
		close(release)
		t.Fatal("more LLM requests entered than the global concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for i := 0; i < requests; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("score request error = %v", err)
		}
	}
	if got := peak.Load(); got != maxConcurrentLLMRequests {
		t.Fatalf("peak concurrency=%d want=%d", got, maxConcurrentLLMRequests)
	}
}

func TestLLMInputAndTimeoutLimits(t *testing.T) {
	if got := boundedLLMTimeout(999); got != maxLLMTimeoutSeconds*time.Second {
		t.Fatalf("bounded timeout=%s", got)
	}
	if got := boundedLLMTimeout(5); got != 5*time.Second {
		t.Fatalf("configured timeout=%s", got)
	}
	args := make([]string, maxLLMArgumentCount+1)
	if err := validateLLMCommandInput("command", "cmd", args); err == nil {
		t.Fatal("oversized argument list accepted")
	}
}

func TestNormalizeRuntimeSettingsClampsLLMResources(t *testing.T) {
	settings := RuntimeSettings{
		AccessToken: "test-token",
		MLConfig: MLConfig{
			LlmTimeoutSeconds: 999,
			LlmMaxTokens:      999999,
			LlmTemperature:    99,
			LlmSystemPrompt:   "bounded",
		},
	}
	if err := normalizeRuntimeSettings(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.MLConfig.LlmTimeoutSeconds != maxLLMTimeoutSeconds || settings.MLConfig.LlmMaxTokens != 4096 || settings.MLConfig.LlmTemperature != 2 {
		t.Fatalf("normalized LLM config = %+v", settings.MLConfig)
	}
}

func TestHeuristicAssessmentDoesNotCallExternalLLM(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, testLLMResponse)
	}))
	defer server.Close()
	withTestLLMConfig(t, server.URL)
	_ = assessCommandSafetyWithOptions(context.Background(), "git", []string{"git", "status"}, "", 0, commandSafetyAssessmentOptions{IncludeLLM: false})
	if got := calls.Load(); got != 0 {
		t.Fatalf("heuristic assessment made %d external LLM calls", got)
	}
}

func TestParseLLMScoreContentBoundsTextFields(t *testing.T) {
	signals := make([]string, maxLLMSignalCount+10)
	for i := range signals {
		signals[i] = strings.Repeat("s", maxLLMSignalBytes+10)
	}
	payload := map[string]any{
		"riskScore":         50,
		"recommendedAction": "ALERT",
		"reasoning":         strings.Repeat("r", maxLLMReasoningBytes+10),
		"signals":           signals,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseLLMScoreContent(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reasoning) > maxLLMReasoningBytes || len(result.Signals) != maxLLMSignalCount {
		t.Fatalf("bounded result reasoning=%d signals=%d", len(result.Reasoning), len(result.Signals))
	}
	for _, signal := range result.Signals {
		if len(signal) > maxLLMSignalBytes {
			t.Fatalf("signal length=%d", len(signal))
		}
	}
}
