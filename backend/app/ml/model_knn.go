package ml

import (
	"agent-ebpf-filter/cuda"
	"container/heap"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func init() {
	RegisterModel(ModelKNN, func() Model { return NewKNNModel(5, "euclidean", "uniform") })
}

// KNNModel is a k-nearest neighbors classifier.
// Pure Go — stores training data in memory for lazy inference.
type KNNModel struct {
	K           int                   `json:"k"`
	Distance    string                `json:"distance"`              // euclidean, manhattan
	Weight      string                `json:"weight"`                // uniform, distance
	MaxDistance float64               `json:"maxDistance,omitempty"` // skip samples beyond this dist (0=unlimited)
	Samples     [][FeatureDim]float64 `json:"-"`
	Labels      []int32               `json:"-"`
	NumClasses  int                   `json:"numClasses"`
}

func NewKNNModel(k int, distance, weight string) *KNNModel {
	if k < 1 {
		k = 1
	}
	return &KNNModel{K: k, Distance: distance, Weight: weight, NumClasses: 4}
}

func (m *KNNModel) Type() ModelType { return ModelKNN }

// ── Heap-based KNN Search ──────────────────────────────────────────

type knnHeapItem struct {
	idx      int
	distance float64
}

// max-heap of knnHeapItem (sorted by distance descending)
type knnMaxHeap []knnHeapItem

func (h knnMaxHeap) Len() int           { return len(h) }
func (h knnMaxHeap) Less(i, j int) bool { return h[i].distance > h[j].distance }
func (h knnMaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *knnMaxHeap) Push(x any)        { *h = append(*h, x.(knnHeapItem)) }
func (h *knnMaxHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// knnSearch finds the K nearest neighbors using a max-heap (O(N log K)).
// When MaxDistance > 0, skips samples beyond the threshold.
func (m *KNNModel) knnSearch(features [FeatureDim]float64) []knnHeapItem {
	k := m.K
	if k <= 0 {
		k = 1
	}
	if k > len(m.Samples) {
		k = len(m.Samples)
	}

	h := &knnMaxHeap{}
	heap.Init(h)
	var exact []knnHeapItem

	for i, sample := range m.Samples {
		d := m.computeDistance(features, sample)
		if math.IsNaN(d) || math.IsInf(d, 0) || d < 0 {
			continue
		}
		if m.MaxDistance > 0 && d > m.MaxDistance {
			continue
		}
		if m.Weight == "distance" && d <= 1e-10 {
			// Do not arbitrarily discard duplicate exact observations due
			// to K or their insertion order. All exact labels must vote.
			exact = append(exact, knnHeapItem{idx: i, distance: d})
		}
		if h.Len() < k {
			heap.Push(h, knnHeapItem{idx: i, distance: d})
		} else if d < (*h)[0].distance {
			(*h)[0] = knnHeapItem{idx: i, distance: d}
			heap.Fix(h, 0)
		}
	}

	if len(exact) > 0 {
		return exact
	}
	// Convert to sorted slice (ascending)
	result := make([]knnHeapItem, h.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(knnHeapItem)
	}
	return result
}

func (m *KNNModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Samples) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	var neighbors []knnHeapItem

	if cuda.IsAvailable() && len(m.Samples) >= 1000 {
		// GPU-accelerated batch distance computation
		q := make([]float32, FeatureDim)
		r := make([]float32, len(m.Samples)*FeatureDim)
		for d := 0; d < FeatureDim; d++ {
			q[d] = float32(features[d])
		}
		for i, s := range m.Samples {
			for d := 0; d < FeatureDim; d++ {
				r[i*FeatureDim+d] = float32(s[d])
			}
		}
		dists := cuda.KNNDistances(q, r, 1, len(m.Samples), FeatureDim, m.Distance)
		allDists := make([]knnHeapItem, len(m.Samples))
		for i := range allDists {
			allDists[i] = knnHeapItem{idx: i, distance: float64(dists[i])}
		}
		// Use same heap-based selection for GPU path
		k := m.K
		if k <= 0 {
			k = 1
		}
		if k > len(allDists) {
			k = len(allDists)
		}
		h := &knnMaxHeap{}
		heap.Init(h)
		var exact []knnHeapItem
		for _, n := range allDists {
			if math.IsNaN(n.distance) || math.IsInf(n.distance, 0) || n.distance < 0 {
				continue
			}
			if m.MaxDistance > 0 && n.distance > m.MaxDistance {
				continue
			}
			if m.Weight == "distance" && n.distance <= 1e-10 {
				exact = append(exact, n)
			}
			if h.Len() < k {
				heap.Push(h, n)
			} else if n.distance < (*h)[0].distance {
				(*h)[0] = n
				heap.Fix(h, 0)
			}
		}
		if len(exact) > 0 {
			neighbors = exact
		} else {
			neighbors = make([]knnHeapItem, h.Len())
			for i := len(neighbors) - 1; i >= 0; i-- {
				neighbors[i] = heap.Pop(h).(knnHeapItem)
			}
		}
	} else {
		// CPU path: heap-based O(N log K)
		neighbors = m.knnSearch(features)
	}

	if len(neighbors) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	k := len(neighbors)
	classVotes := make([]float64, m.NumClasses)
	totalWeight := 0.0
	weightedDistanceSum := 0.0
	votedNeighbors := 0
	// Inverse-distance weighting must never make a merely nearby neighbor
	// outweigh an exact match. With exact matches, vote only among those
	// matches; otherwise retain normal inverse-distance behavior.
	exactMatch := m.Weight == "distance" && neighbors[0].distance <= 1e-10
	for i := 0; i < k; i++ {
		n := neighbors[i]
		if n.idx < 0 || n.idx >= len(m.Labels) {
			continue
		}
		label := m.Labels[n.idx]
		if label < 0 || int(label) >= m.NumClasses {
			continue
		}
		if exactMatch && n.distance > 1e-10 {
			continue
		}
		w := 1.0
		if !exactMatch && m.Weight == "distance" && n.distance > 1e-10 {
			w = 1.0 / n.distance
		}
		classVotes[label] += w
		totalWeight += w
		weightedDistanceSum += n.distance
		votedNeighbors++
	}
	if totalWeight == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	bestClass := int32(0)
	bestVotes := classVotes[0]
	for i := 1; i < m.NumClasses; i++ {
		if classVotes[i] > bestVotes {
			bestVotes = classVotes[i]
			bestClass = int32(i)
		}
	}

	confidence := 0.0
	if totalWeight > 0 {
		confidence = bestVotes / totalWeight
	}

	// The anomaly score must use the same neighbors that actually cast
	// votes. Otherwise a perfect match can still appear anomalous because
	// discarded, more distant neighbors inflate the reported distance.
	avgDist := weightedDistanceSum / float64(votedNeighbors)
	anomalyScore := math.Tanh(avgDist)

	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: anomalyScore}
}

func (m *KNNModel) computeDistance(a, b [FeatureDim]float64) float64 {
	if m.Distance == "cosine" {
		dot := 0.0
		normA := 0.0
		normB := 0.0
		for i := 0; i < FeatureDim; i++ {
			dot += a[i] * b[i]
			normA += a[i] * a[i]
			normB += b[i] * b[i]
		}
		if normA <= 0 || normB <= 0 {
			return 1.0
		}
		// Floating-point rounding can push cosine similarity just over 1
		// and yield a negative distance, especially for large vectors.
		return math.Max(0, 1.0-dot/(math.Sqrt(normA)*math.Sqrt(normB)))
	}
	var sum float64
	for i := 0; i < FeatureDim; i++ {
		diff := a[i] - b[i]
		if m.Distance == "manhattan" {
			sum += math.Abs(diff)
		} else {
			sum += diff * diff
		}
	}
	if m.Distance == "manhattan" {
		return sum
	}
	return math.Sqrt(sum)
}

func (m *KNNModel) Serialize(path string) error {
	size := 4*7 + len(m.Samples)*(FeatureDim*8+4)
	data := make([]byte, 0, size)

	putU32 := func(v uint32) {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		data = append(data, b...)
	}

	data = append(data, []byte("KNNN")...)
	putU32(2)                       // version (2 = added MaxDistance)
	putU32(uint32(m.K))             // k
	putU32(uint32(len(m.Distance))) // dist len
	data = append(data, []byte(m.Distance)...)
	putU32(uint32(len(m.Weight))) // weight len
	data = append(data, []byte(m.Weight)...)
	putU32(uint32(len(m.Samples))) // sample count
	putU32(uint32(m.NumClasses))
	// MaxDistance (new in v2)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(m.MaxDistance))
	data = append(data, b...)

	// Each sample: features (FeatureDim * 8 bytes) + label (4 bytes)
	for i, feat := range m.Samples {
		for _, v := range feat {
			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, math.Float64bits(v))
			data = append(data, b...)
		}
		label := make([]byte, 4)
		binary.LittleEndian.PutUint32(label, uint32(m.Labels[i]))
		data = append(data, label...)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func DeserializeKNN(path string) (*KNNModel, error) {
	r, err := newMLBinaryModelReaderWithLimit(path, "KNNN", mlKNNMaxModelFileBytes)
	if err != nil {
		return nil, err
	}
	version := r.readSupportedVersion("KNN", 1, 2)
	k := r.readBoundedCount("KNN neighbor count", 1, mlMaxTrainingSamples)
	distance := r.readBoundedString("KNN distance", 32)
	weight := r.readBoundedString("KNN weight", 32)
	numSamples := r.readBoundedCount("KNN sample count", 0, mlMaxTrainingSamples)
	numClasses := r.readBoundedCount("KNN class count", 1, mlBinaryMaxClasses)
	if distance != "euclidean" && distance != "manhattan" && distance != "cosine" {
		return nil, fmt.Errorf("invalid KNN distance %q", distance)
	}
	if weight != "uniform" && weight != "distance" {
		return nil, fmt.Errorf("invalid KNN weight %q", weight)
	}

	m := &KNNModel{K: k, Distance: distance, Weight: weight, NumClasses: numClasses}

	// Read MaxDistance if v2+
	if version >= 2 {
		m.MaxDistance = r.readF64()
		if math.IsNaN(m.MaxDistance) || math.IsInf(m.MaxDistance, 0) || m.MaxDistance < 0 {
			return nil, fmt.Errorf("invalid KNN max distance %v", m.MaxDistance)
		}
	}
	r.requireItems("KNN samples", numSamples, FeatureDim*mlBinaryFloatBytes+4, 0)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}

	m.Samples = make([][FeatureDim]float64, numSamples)
	m.Labels = make([]int32, numSamples)

	for i := 0; i < numSamples; i++ {
		for j := 0; j < FeatureDim; j++ {
			value := r.readF64()
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("invalid KNN sample %d feature %d", i, j)
			}
			m.Samples[i][j] = value
		}
		label := r.readU32()
		if label >= uint32(numClasses) {
			return nil, fmt.Errorf("invalid KNN sample %d label %d", i, label)
		}
		m.Labels[i] = int32(label)
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}
