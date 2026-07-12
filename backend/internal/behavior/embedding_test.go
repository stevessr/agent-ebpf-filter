package behavior

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"agent-ebpf-filter/pb"
)

func TestBuildInstructionHandlesNilEvent(t *testing.T) {
	t.Parallel()
	if got := BuildInstruction(nil); got != "" {
		t.Fatalf("BuildInstruction(nil) = %q", got)
	}
	event := &pb.Event{Comm: "curl", Type: "connect", Path: "/tmp/socket", Tag: "test"}
	if got := BuildInstruction(event); !strings.Contains(got, "process curl") || !strings.Contains(got, "/tmp/socket") {
		t.Fatalf("BuildInstruction(event) = %q", got)
	}
}

func TestInstructionVocabularyIsBoundedAndOOVTokensRemainVisible(t *testing.T) {
	t.Parallel()
	embedder := newInstructionEmbedderWithLimits(4, 8)
	embedder.RegisterVocab("alpha beta gamma delta epsilon zeta")
	if got := len(embedder.vocab); got != 4 {
		t.Fatalf("vocabulary size = %d, want 4", got)
	}
	for token, index := range embedder.vocab {
		if index < 0 || index >= 4 {
			t.Fatalf("vocabulary token %q index = %d", token, index)
		}
	}
	embedder.RegisterVocab(strings.Repeat("new-token ", maxInstructionTokens+100))
	if got := len(embedder.vocab); got != 4 {
		t.Fatalf("saturated vocabulary size = %d, want 4", got)
	}
	embedding := embedder.EmbedInstruction("epsilon")
	if vectorNorm(embedding.Vector) == 0 {
		t.Fatal("out-of-vocabulary token produced a zero embedding")
	}
}

func TestInstructionTokenizationBoundsInputAndTokenCount(t *testing.T) {
	t.Parallel()
	instruction := strings.Repeat(strings.Repeat("界", maxInstructionTokenBytes)+" ", maxInstructionTokens+100)
	tokens := boundedInstructionTokens(instruction)
	if len(tokens) > maxInstructionTokens {
		t.Fatalf("token count = %d", len(tokens))
	}
	for index, token := range tokens {
		if len(token) > maxInstructionTokenBytes {
			t.Fatalf("token %d length = %d", index, len(token))
		}
	}
}

func TestInstructionClustersSaturateWithoutReplacingBaseline(t *testing.T) {
	t.Parallel()
	embedder := newInstructionEmbedderWithLimits(4, 3)
	ids := make([]ClusterID, 0, 3)
	for index := 0; index < 3; index++ {
		id, created := embedder.AddToCluster(oneHotEmbedding(index))
		if !created {
			t.Fatalf("cluster %d was not created", index)
		}
		ids = append(ids, id)
	}
	if _, created := embedder.AddToCluster(oneHotEmbedding(10)); created {
		t.Fatal("saturated embedder created another cluster")
	}
	if got := len(embedder.clusters); got != 3 {
		t.Fatalf("cluster count = %d, want 3", got)
	}
	for index, cluster := range embedder.clusters {
		if cluster.ID != ids[index] {
			t.Fatalf("cluster %d ID = %d, want %d", index, cluster.ID, ids[index])
		}
	}
	if score := embedder.ComputeAnomalyScore(oneHotEmbedding(10)); score < 0.99 || score > 1 {
		t.Fatalf("saturated anomaly score = %v", score)
	}

	if id, created := embedder.AddToCluster(oneHotEmbedding(0)); created || id != ids[0] {
		t.Fatalf("matching cluster result = id %d created %v", id, created)
	}
	clusters := embedder.GetClusters()
	if len(clusters) != 3 || clusters[0].ID != ids[0] || clusters[0].Count != 2 {
		t.Fatalf("sorted clusters = %+v", clusters)
	}
}

func TestInstructionEmbedderReusesZeroVectorCluster(t *testing.T) {
	t.Parallel()
	embedder := newInstructionEmbedderWithLimits(4, 3)
	firstID, created := embedder.AddToCluster(BehaviorEmbedding{})
	if !created {
		t.Fatal("first zero-vector cluster was not created")
	}
	secondID, created := embedder.AddToCluster(BehaviorEmbedding{})
	if created || secondID != firstID || len(embedder.clusters) != 1 || embedder.clusters[0].Count != 2 {
		t.Fatalf("second zero-vector result = id %d created %v clusters %+v", secondID, created, embedder.clusters)
	}
}

func TestInstructionEmbedderSanitizesNonFiniteVectorsAndSaturatesCount(t *testing.T) {
	t.Parallel()
	embedder := newInstructionEmbedderWithLimits(4, 2)
	var vector [64]float64
	vector[0] = math.NaN()
	vector[1] = math.Inf(1)
	vector[2] = math.MaxFloat64
	id, created := embedder.AddToCluster(BehaviorEmbedding{Vector: vector})
	if !created || id == 0 {
		t.Fatalf("AddToCluster() = id %d created %v", id, created)
	}
	for index, value := range embedder.clusters[0].Centroid {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("centroid %d is non-finite: %v", index, value)
		}
	}
	embedder.clusters[0].Count = maxInstructionClusterCount
	if _, created := embedder.AddToCluster(BehaviorEmbedding{Vector: vector}); created {
		t.Fatal("matching vector unexpectedly created a cluster")
	}
	if got := embedder.clusters[0].Count; got != maxInstructionClusterCount {
		t.Fatalf("saturated cluster count = %d", got)
	}
	if score := embedder.ComputeAnomalyScore(BehaviorEmbedding{Vector: vector}); math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
		t.Fatalf("anomaly score = %v", score)
	}
}

func TestInstructionEmbedderConcurrentUpdatesRemainBounded(t *testing.T) {
	embedder := newInstructionEmbedderWithLimits(16, 8)
	var workers sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				instruction := fmt.Sprintf("worker-%d iteration-%d command", worker, iteration)
				embedder.RegisterVocab(instruction)
				embedding := embedder.EmbedInstruction(instruction)
				embedder.AddToCluster(embedding)
				_ = embedder.ComputeAnomalyScore(embedding)
				_ = embedder.GetClusters()
			}
		}()
	}
	workers.Wait()
	if got := len(embedder.vocab); got > 16 {
		t.Fatalf("vocabulary size = %d", got)
	}
	if got := len(embedder.clusters); got > 8 {
		t.Fatalf("cluster count = %d", got)
	}
}

func oneHotEmbedding(index int) BehaviorEmbedding {
	var vector [64]float64
	vector[index%len(vector)] = 1
	return BehaviorEmbedding{Vector: vector}
}

func vectorNorm(vector [64]float64) float64 {
	norm := 0.0
	for _, value := range vector {
		norm += value * value
	}
	return math.Sqrt(norm)
}
