package behavior

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"agent-ebpf-filter/pb"
)

const (
	behaviorEmbeddingDimensions      = 64
	defaultInstructionVocabLimit     = behaviorEmbeddingDimensions
	defaultInstructionClusterLimit   = 128
	maxInstructionClusterLimit       = 4096
	maxInstructionInputBytes         = 64 << 10
	maxInstructionTokens             = 2048
	maxInstructionTokenBytes         = 256
	maxInstructionClusterCount       = 1_000_000
	instructionClusterMatchThreshold = 0.75
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
	mu          sync.RWMutex
	clusters    []EventCluster
	nextID      ClusterID
	maxClusters int
	// vocabulary built from seen event patterns
	vocab    map[string]int
	maxVocab int
}

func NewInstructionEmbedder() *InstructionEmbedder {
	return newInstructionEmbedderWithLimits(defaultInstructionVocabLimit, defaultInstructionClusterLimit)
}

func newInstructionEmbedderWithLimits(maxVocab, maxClusters int) *InstructionEmbedder {
	if maxVocab <= 0 || maxVocab > behaviorEmbeddingDimensions {
		maxVocab = defaultInstructionVocabLimit
	}
	if maxClusters <= 0 || maxClusters > maxInstructionClusterLimit {
		maxClusters = defaultInstructionClusterLimit
	}
	return &InstructionEmbedder{
		clusters:    make([]EventCluster, 0, maxClusters),
		nextID:      1,
		maxClusters: maxClusters,
		vocab:       make(map[string]int, maxVocab),
		maxVocab:    maxVocab,
	}
}

// BuildInstruction converts an eBPF event into a natural-language instruction string.
// This mirrors the instruction-embedding paradigm from papers like:
// - "Self-Attentive Classification-Based Anomaly Detection in Unstructured Logs"
// - "Log2Vec: A Heterogeneous Graph Embedding Based Approach"
func BuildInstruction(event *pb.Event) string {
	if event == nil {
		return ""
	}
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
	tokens := boundedInstructionTokens(instruction)
	if len(tokens) == 0 {
		return BehaviorEmbedding{Vector: vec}
	}

	// Keep the read lock while consulting the map. Copying the map header and
	// unlocking, as the old implementation did, races with RegisterVocab.
	e.mu.RLock()
	addFeature := func(feature string) {
		idx, ok := e.vocab[feature]
		if !ok {
			h := sha256.Sum256([]byte(feature))
			idx = int(binary.BigEndian.Uint64(h[:8]) % behaviorEmbeddingDimensions)
		}
		if idx >= 0 && idx < behaviorEmbeddingDimensions {
			vec[idx] += 1.0
		}
	}
	for _, token := range tokens {
		addFeature(token)
	}
	for index := 1; index < len(tokens); index++ {
		addFeature(tokens[index-1] + "_" + tokens[index])
	}
	for index := 2; index < len(tokens); index++ {
		addFeature(tokens[index-2] + "_" + tokens[index-1] + "_" + tokens[index])
	}
	e.mu.RUnlock()

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

func boundedInstructionTokens(instruction string) []string {
	instruction = truncateInstructionUTF8(instruction, maxInstructionInputBytes)
	if instruction == "" {
		return nil
	}
	tokens := strings.Fields(strings.ToLower(instruction))
	if len(tokens) > maxInstructionTokens {
		tokens = tokens[:maxInstructionTokens]
	}
	for index, token := range tokens {
		tokens[index] = truncateInstructionUTF8(token, maxInstructionTokenBytes)
	}
	return tokens
}

func truncateInstructionUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

// cosineSimilarity between two vectors
func cosineSimilarity(a, b [64]float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 && nb == 0 {
		return 1
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
	emb.Vector = normalizeBehaviorVector(emb.Vector)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureLocked()

	bestIndex := -1
	bestSim := -1.0
	for i := range e.clusters {
		sim := cosineSimilarity(emb.Vector, e.clusters[i].Centroid)
		if sim > bestSim {
			bestSim = sim
			bestIndex = i
		}
	}

	if bestIndex >= 0 && bestSim >= instructionClusterMatchThreshold {
		cluster := &e.clusters[bestIndex]
		updateInstructionCluster(cluster, emb.Vector)
		return cluster.ID, false
	}

	if len(e.clusters) >= e.maxClusters {
		// Preserve the learned baseline when saturated. Dissimilar samples still
		// receive a high anomaly score, but cannot grow or poison cluster state.
		if bestIndex >= 0 {
			return e.clusters[bestIndex].ID, false
		}
		return 0, false
	}

	newID := e.nextID
	e.nextID++
	e.clusters = append(e.clusters, EventCluster{
		ID:       newID,
		Centroid: emb.Vector,
		Count:    1,
		Label:    fmt.Sprintf("Cluster-%d", newID),
	})
	return newID, true
}

func updateInstructionCluster(cluster *EventCluster, vector [64]float64) {
	if cluster == nil {
		return
	}
	count := cluster.Count
	if count < 1 {
		count = 1
	}
	weight := count
	if weight >= maxInstructionClusterCount {
		weight = maxInstructionClusterCount - 1
	}
	for index := range cluster.Centroid {
		cluster.Centroid[index] = (cluster.Centroid[index]*float64(weight) + vector[index]) / float64(weight+1)
	}
	if cluster.Count < maxInstructionClusterCount {
		cluster.Count = count + 1
	}
}

func normalizeBehaviorVector(vector [64]float64) [64]float64 {
	maxAbs := 0.0
	for index, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			vector[index] = 0
			continue
		}
		if abs := math.Abs(value); abs > maxAbs {
			maxAbs = abs
		}
	}
	if maxAbs == 0 {
		return vector
	}
	norm := 0.0
	for _, value := range vector {
		scaled := value / maxAbs
		norm += scaled * scaled
	}
	norm = math.Sqrt(norm)
	for index := range vector {
		vector[index] = (vector[index] / maxAbs) / norm
	}
	return vector
}

func (e *InstructionEmbedder) ensureLocked() {
	if e.maxVocab <= 0 || e.maxVocab > behaviorEmbeddingDimensions {
		e.maxVocab = defaultInstructionVocabLimit
	}
	if e.maxClusters <= 0 || e.maxClusters > maxInstructionClusterLimit {
		e.maxClusters = defaultInstructionClusterLimit
	}
	if e.vocab == nil {
		e.vocab = make(map[string]int, e.maxVocab)
	}
	if e.clusters == nil {
		e.clusters = make([]EventCluster, 0, e.maxClusters)
	}
	if e.nextID <= 0 {
		e.nextID = 1
	}
}

// GetClusters returns a copy of current clusters (top 20 by count)
func (e *InstructionEmbedder) GetClusters() []EventCluster {
	e.mu.RLock()
	out := make([]EventCluster, len(e.clusters))
	copy(out, e.clusters)
	e.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].ID < out[j].ID
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// RegisterVocab adds instruction tokens to the vocabulary
func (e *InstructionEmbedder) RegisterVocab(instruction string) {
	e.mu.RLock()
	full := e.maxVocab > 0 && len(e.vocab) >= e.maxVocab
	e.mu.RUnlock()
	if full {
		return
	}
	tokens := boundedInstructionTokens(instruction)
	if len(tokens) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureLocked()
	for _, t := range tokens {
		if len(e.vocab) >= e.maxVocab {
			return
		}
		if _, ok := e.vocab[t]; !ok {
			e.vocab[strings.Clone(t)] = len(e.vocab)
		}
	}
}

// ComputeAnomalyScore returns how far this embedding is from its nearest cluster centroid.
// High score = more anomalous. 0 = perfectly normal (at a centroid).
func (e *InstructionEmbedder) ComputeAnomalyScore(emb BehaviorEmbedding) float64 {
	emb.Vector = normalizeBehaviorVector(emb.Vector)
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
	if bestSim < 0 {
		bestSim = 0
	}
	if bestSim > 1 {
		bestSim = 1
	}
	return 1.0 - bestSim
}
