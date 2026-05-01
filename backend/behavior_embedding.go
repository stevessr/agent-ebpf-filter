package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"agent-ebpf-filter/pb"
)

// BehaviorEmbedding represents a 64-dimension feature vector for an eBPF event.
// Inspired by Log2Vec / DeepLog approaches: convert behavioral events into
// instruction-level feature vectors suitable for clustering and anomaly detection.
type BehaviorEmbedding struct {
	Vector [64]float64
}

// ClusterID identifies a behavioral cluster
type ClusterID int

// EventCluster holds cluster metadata
type EventCluster struct {
	ID       ClusterID
	Centroid [64]float64
	Count    int
	Label    string
}

// InstructionEmbedder converts eBPF events to text instructions then to vector embeddings.
// Uses a lightweight locality-sensitive hashing approach (no external ML deps) to map
// behavioral patterns into a 64-dim vector space for online clustering.
type InstructionEmbedder struct {
	mu       sync.RWMutex
	clusters []EventCluster
	nextID   ClusterID
	// vocabulary built from seen event patterns
	vocab     map[string]int
	vocabSize int
}

var globalEmbedder = &InstructionEmbedder{
	clusters: make([]EventCluster, 0),
	nextID:   1,
	vocab:    make(map[string]int),
}

// BuildInstruction converts an eBPF event into a natural-language instruction string.
// This mirrors the instruction-embedding paradigm from papers like:
// - "Self-Attentive Classification-Based Anomaly Detection in Unstructured Logs"
// - "Log2Vec: A Heterogeneous Graph Embedding Based Approach"
func BuildInstruction(event *pb.Event) string {
	parts := make([]string, 0, 4)
	parts = append(parts, fmt.Sprintf("process %s", event.Comm))
	parts = append(parts, fmt.Sprintf("performed %s", event.Type))

	if event.Path != "" {
		parts = append(parts, fmt.Sprintf("on path %s", event.Path))
	}
	if event.Tag != "" {
		parts = append(parts, fmt.Sprintf("tagged %s", event.Tag))
	}
	if event.NetEndpoint != "" {
		parts = append(parts, fmt.Sprintf("to %s", event.NetEndpoint))
	}
	if event.NetDirection != "" {
		parts = append(parts, fmt.Sprintf("direction %s", event.NetDirection))
	}
	if event.Retval != 0 {
		parts = append(parts, fmt.Sprintf("retval %d", event.Retval))
	}
	return strings.Join(parts, " ")
}

// EmbedInstruction converts an instruction string into a 64-dim vector using
// locality-sensitive hashing of n-gram features.
func (e *InstructionEmbedder) EmbedInstruction(instruction string) BehaviorEmbedding {
	var vec [64]float64

	// Tokenize into bigrams and trigrams
	tokens := strings.Fields(strings.ToLower(instruction))
	ngrams := make([]string, 0, len(tokens)*2)
	for _, t := range tokens {
		ngrams = append(ngrams, t)
	}
	for i := 0; i < len(tokens)-1; i++ {
		ngrams = append(ngrams, tokens[i]+"_"+tokens[i+1])
	}
	for i := 0; i < len(tokens)-2; i++ {
		ngrams = append(ngrams, tokens[i]+"_"+tokens[i+1]+"_"+tokens[i+2])
	}

	e.mu.RLock()
	vocabSnapshot := e.vocab
	vocabSize := e.vocabSize
	e.mu.RUnlock()

	for _, ng := range ngrams {
		idx, ok := vocabSnapshot[ng]
		if !ok {
			// Assign to a random-feature bucket for OOV n-grams
			h := sha256.Sum256([]byte(ng))
			idx = int(binary.BigEndian.Uint64(h[:8]) % 64)
		}
		// Use normalized TF-IDF-like weighting
		if idx < 64 {
			vec[idx] += 1.0
		}
	}
	_ = vocabSize // reserved for IDF weighting

	// L2 normalize
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] /= norm
		}
	}
	return BehaviorEmbedding{Vector: vec}
}

// cosineSimilarity between two vectors
func cosineSimilarity(a, b [64]float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// ClassifyAndEmbed classifies a wrapper command and produces its embedding
func (e *InstructionEmbedder) ClassifyAndEmbed(comm string, args []string) (*pb.BehaviorClassification, BehaviorEmbedding) {
	classification := ClassifyBehavior(comm, args)
	instruction := fmt.Sprintf("process %s performed wrapper_intercept on %s %s tagged Wrapper",
		comm, comm, strings.Join(args, " "))
	embedding := e.EmbedInstruction(instruction)
	return classification, embedding
}

// AddToCluster assigns an embedding to the nearest cluster or creates a new one.
// Returns the cluster ID and whether a new cluster was created.
func (e *InstructionEmbedder) AddToCluster(emb BehaviorEmbedding) (ClusterID, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	const similarityThreshold = 0.75

	bestID := ClusterID(-1)
	bestSim := 0.0
	for i := range e.clusters {
		sim := cosineSimilarity(emb.Vector, e.clusters[i].Centroid)
		if sim > bestSim {
			bestSim = sim
			bestID = e.clusters[i].ID
		}
	}

	if bestID >= 0 && bestSim >= similarityThreshold {
		// Assign to existing cluster, update centroid
		for i := range e.clusters {
			if e.clusters[i].ID == bestID {
				n := float64(e.clusters[i].Count)
				for j := range e.clusters[i].Centroid {
					e.clusters[i].Centroid[j] = (e.clusters[i].Centroid[j]*n + emb.Vector[j]) / (n + 1)
				}
				e.clusters[i].Count++
				return bestID, false
			}
		}
	}

	// Create new cluster
	newID := e.nextID
	e.nextID++
	e.clusters = append(e.clusters, EventCluster{
		ID:       newID,
		Centroid: emb.Vector,
		Count:    1,
		Label:    fmt.Sprintf("Cluster-%d", newID),
	})
	sort.Slice(e.clusters, func(i, j int) bool {
		return e.clusters[i].Count > e.clusters[j].Count
	})
	return newID, true
}

// GetClusters returns a copy of current clusters (top 20 by count)
func (e *InstructionEmbedder) GetClusters() []EventCluster {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.clusters)
	if n > 20 {
		n = 20
	}
	out := make([]EventCluster, n)
	copy(out, e.clusters[:n])
	return out
}

// RegisterVocab adds instruction tokens to the vocabulary
func (e *InstructionEmbedder) RegisterVocab(instruction string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	tokens := strings.Fields(strings.ToLower(instruction))
	for _, t := range tokens {
		if _, ok := e.vocab[t]; !ok {
			e.vocab[t] = e.vocabSize
			e.vocabSize++
		}
	}
}

// ComputeAnomalyScore returns how far this embedding is from its nearest cluster centroid.
// High score = more anomalous. 0 = perfectly normal (at a centroid).
func (e *InstructionEmbedder) ComputeAnomalyScore(emb BehaviorEmbedding) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.clusters) == 0 {
		return 1.0
	}

	bestSim := 0.0
	for i := range e.clusters {
		sim := cosineSimilarity(emb.Vector, e.clusters[i].Centroid)
		if sim > bestSim {
			bestSim = sim
		}
	}
	return 1.0 - bestSim
}
