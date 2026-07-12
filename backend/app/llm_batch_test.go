package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLLMBatchLimit(t *testing.T) {
	if got := normalizeLLMBatchLimit(0); got != defaultLLMBatchScoreLimit {
		t.Fatalf("default limit=%d", got)
	}
	if got := normalizeLLMBatchLimit(5000); got != maxLLMBatchScoreLimit {
		t.Fatalf("maximum limit=%d", got)
	}
	if got := len(limitLLMSubjects(make([]llmScoreSubject, 5000), 5000)); got != maxLLMBatchScoreLimit {
		t.Fatalf("bounded subjects=%d", got)
	}
}

func TestScoreLLMSampleSubjectsUsesBoundedWorkersAndPreservesOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testLLMResponse)
	}))
	defer server.Close()
	withTestLLMConfig(t, server.URL)

	subjects := make([]llmScoreSubject, 12)
	for i := range subjects {
		subjects[i] = llmScoreSubject{
			Index: i,
			Sample: TrainingSample{
				CommandLine: fmt.Sprintf("echo %d", i),
				Comm:        "echo",
				Args:        []string{"echo", fmt.Sprint(i)},
				Label:       -1,
			},
		}
	}
	response, err := scoreLLMSampleSubjects(context.Background(), "test", subjects, len(subjects), false, false, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	if response.Scored != len(subjects) || len(response.Entries) != len(subjects) {
		t.Fatalf("scored=%d entries=%d", response.Scored, len(response.Entries))
	}
	for i, entry := range response.Entries {
		if entry.Index != i {
			t.Fatalf("entry %d index=%d", i, entry.Index)
		}
	}
}

func TestScoreLLMSampleSubjectsStopsOnCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, testLLMResponse)
	}))
	defer server.Close()
	withTestLLMConfig(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := scoreLLMSampleSubjects(ctx, "test", []llmScoreSubject{{
		Index:  1,
		Sample: TrainingSample{CommandLine: "echo", Comm: "echo", Args: []string{"echo"}, Label: -1},
	}}, 1, false, false, 0.2)
	if err == nil {
		t.Fatal("canceled batch was accepted")
	}
}

func TestTrainerCancellationContextStopsLLMReview(t *testing.T) {
	trainer := &ModelTrainer{cancelCh: make(chan struct{})}
	ctx, stop := trainerCancellationContext(trainer, context.Background())
	defer stop()
	trainer.requestCancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("trainer cancellation did not reach LLM review context")
	}
}

func TestScoreLLMBatchRejectsExcessConcurrentBatch(t *testing.T) {
	withTestLLMConfig(t, "http://127.0.0.1:1")
	oldSlots := llmBatchSlots
	llmBatchSlots = make(chan struct{}, maxConcurrentLLMBatches)
	for i := 0; i < maxConcurrentLLMBatches; i++ {
		llmBatchSlots <- struct{}{}
	}
	t.Cleanup(func() { llmBatchSlots = oldSlots })
	_, err := scoreLLMBatch(context.Background(), llmBatchScoreRequest{Source: "training", Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "concurrency limit") {
		t.Fatalf("batch concurrency error = %v", err)
	}
}

func TestBoundedLLMSubjectSnapshotsFilterBeforeLimit(t *testing.T) {
	store := newTrainingDataStore(8)
	for i := 0; i < 6; i++ {
		sample := TrainingSample{
			Timestamp:   time.Now(),
			CommandLine: fmt.Sprintf("echo %d", i),
			Comm:        "echo",
			Args:        []string{"echo", fmt.Sprint(i)},
			Label:       -1,
		}
		if i < 3 {
			sample.Label = 0
			sample.UserLabel = "manual"
		}
		store.Add(sample)
	}
	items := store.BoundedSamplesWithIndex(2, true)
	if len(items) != 2 || items[0].Sample.CommandLine != "echo 3" || items[1].Sample.CommandLine != "echo 4" {
		t.Fatalf("bounded unlabeled samples = %+v", items)
	}
	items[0].Sample.Args[0] = "mutated"
	again := store.BoundedSamplesWithIndex(1, true)
	if again[0].Sample.Args[0] != "echo" {
		t.Fatal("bounded sample snapshot aliases store arguments")
	}
}
