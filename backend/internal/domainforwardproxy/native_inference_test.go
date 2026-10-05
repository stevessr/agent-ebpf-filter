package domainforwardproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNativeInferenceFixture(t testing.TB, replacement string) string {
	return writeNativeInferenceFixtureLabels(t, replacement, 1)
}

func writeNativeInferenceFixtureLabels(t testing.TB, replacement string, labelCount int) string {
	t.Helper()
	labels := make([]nativeInferenceLabel, labelCount)
	for i := range labels {
		labels[i] = nativeInferenceLabel{
			Name:        fmt.Sprintf("sensitive-%d", i),
			Threshold:   1,
			Replacement: replacement,
			Weights:     []int8{1, 1, 1, 1, 1, 1, 1, 1},
		}
	}
	model := nativeInferenceModelFile{
		Version:   nativeInferenceModelVersion,
		Dimension: 8,
		Seed:      7,
		Labels:    labels,
	}
	payload, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rewrite-int8.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNativeInferenceRewritesJSONValuesButNotKeys(t *testing.T) {
	modelFile := writeNativeInferenceFixture(t, "<PRIVATE>")
	kernel, err := LoadNativeInferenceKernel(NativeInferenceSettings{
		Enabled:       true,
		ModelFile:     modelFile,
		Direction:     "request",
		Host:          "api.openai.com",
		PathPrefix:    "/v1/responses",
		ContentType:   "application/json",
		MinTokenBytes: 3,
		MaxTokenBytes: 256,
	})
	if err != nil {
		t.Fatalf("load inference model: %v", err)
	}
	if !kernel.Matches("request", "api.openai.com", "/v1/responses", "application/json; charset=utf-8") {
		t.Fatal("expected inference scope to match")
	}
	if kernel.Matches("response", "api.openai.com", "/v1/responses", "application/json") {
		t.Fatal("response escaped request-only inference scope")
	}

	input := []byte(`{"email":"alice@example.com","short":"ok","escaped":"prefix\\\"quote"}`)
	got, changed := kernel.Rewrite("application/json", input)
	if !changed {
		t.Fatalf("expected rewrite: %s", got)
	}
	text := string(got)
	if !strings.Contains(text, `"email":"<PRIVATE>"`) {
		t.Fatalf("email value not rewritten: %s", text)
	}
	if !strings.Contains(text, `"email":`) || !strings.Contains(text, `"short":"ok"`) {
		t.Fatalf("JSON keys or short values changed unexpectedly: %s", text)
	}
	if !json.Valid(got) {
		t.Fatalf("rewrite produced invalid JSON: %s", got)
	}
}

func TestNativeInferenceRejectsUnsafeReplacement(t *testing.T) {
	modelFile := writeNativeInferenceFixture(t, `bad"token`)
	_, err := LoadNativeInferenceKernel(NativeInferenceSettings{
		Enabled:   true,
		ModelFile: modelFile,
	})
	if err == nil || !strings.Contains(err.Error(), "JSON-safe token") {
		t.Fatalf("expected replacement validation error, got %v", err)
	}
}

func TestRewriteKernelAppliesNativeInference(t *testing.T) {
	modelFile := writeNativeInferenceFixture(t, "<MASK>")
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		Inference: NativeInferenceSettings{
			Enabled:       true,
			ModelFile:     modelFile,
			Direction:     "request",
			ContentType:   "application/json",
			MinTokenBytes: 3,
			MaxTokenBytes: 256,
		},
	})
	if err := kernel.InferenceError(); err != nil {
		t.Fatalf("inference error: %v", err)
	}
	got, _, changed := kernel.RewriteRequest(
		"api.openai.com",
		"/v1/responses",
		"application/json",
		[]byte(`{"input":"secret@example.com"}`),
	)
	if !changed || !strings.Contains(string(got), `"input":"<MASK>"`) {
		t.Fatalf("native inference was not applied: changed=%v body=%s", changed, got)
	}
}

func BenchmarkNativeInferenceJSON(b *testing.B) {
	body := []byte(`{"model":"gpt-5.6","input":[{"role":"user","content":[{"type":"input_text","text":"secret@example.com"}]}]}`)
	for _, labelCount := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("labels_%d", labelCount), func(b *testing.B) {
			modelFile := writeNativeInferenceFixtureLabels(b, "<MASK>", labelCount)
			kernel, err := LoadNativeInferenceKernel(NativeInferenceSettings{
				Enabled:       true,
				ModelFile:     modelFile,
				MinTokenBytes: 3,
				MaxTokenBytes: 256,
			})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				_, _ = kernel.Rewrite("application/json", body)
			}
		})
	}
}
