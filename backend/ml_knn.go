package main

import (
	"container/heap"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"agent-ebpf-filter/cuda"
)

func init() {
	RegisterModel(ModelKNN, func() Model { return NewKNNModel(5, "euclidean", "uniform") })
}

// KNNModel is a k-nearest neighbors classifier.
// Pure Go — stores training data in memory for lazy inference.
type KNNModel struct {
	K           int      `json:"k"`
	Distance    string   `json:"distance"`   // euclidean, manhattan
	Weight      string   `json:"weight"`     // uniform, distance
	MaxDistance float64  `json:"maxDistance,omitempty"` // skip samples beyond this dist (0=unlimited)
	Samples     [][FeatureDim]float64 `json:"-"`
	Labels      []int32  `json:"-"`
	NumClasses  int      `json:"numClasses"`
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

	for i, sample := range m.Samples {
		d := m.computeDistance(features, sample)
		if m.MaxDistance > 0 && d > m.MaxDistance {
			continue
		}
		if h.Len() < k {
			heap.Push(h, knnHeapItem{idx: i, distance: d})
		} else if d < (*h)[0].distance {
			(*h)[0] = knnHeapItem{idx: i, distance: d}
			heap.Fix(h, 0)
		}
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
		for _, n := range allDists {
			if m.MaxDistance > 0 && n.distance > m.MaxDistance {
				continue
			}
			if h.Len() < k {
				heap.Push(h, n)
			} else if n.distance < (*h)[0].distance {
				(*h)[0] = n
				heap.Fix(h, 0)
			}
		}
		neighbors = make([]knnHeapItem, h.Len())
		for i := len(neighbors) - 1; i >= 0; i-- {
			neighbors[i] = heap.Pop(h).(knnHeapItem)
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
	for i := 0; i < k; i++ {
		n := neighbors[i]
		w := 1.0
		if m.Weight == "distance" && n.distance > 1e-10 {
			w = 1.0 / n.distance
		}
		classVotes[m.Labels[n.idx]] += w
		totalWeight += w
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

	avgDist := 0.0
	for i := 0; i < k; i++ {
		avgDist += neighbors[i].distance
	}
	avgDist /= float64(k)
	anomalyScore := math.Tanh(avgDist)

	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: anomalyScore}
}

func (m *KNNModel) computeDistance(a, b [FeatureDim]float64) float64 {
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
	putU32(2)                          // version (2 = added MaxDistance)
	putU32(uint32(m.K))                // k
	putU32(uint32(len(m.Distance)))    // dist len
	data = append(data, []byte(m.Distance)...)
	putU32(uint32(len(m.Weight)))      // weight len
	data = append(data, []byte(m.Weight)...)
	putU32(uint32(len(m.Samples)))     // sample count
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 24 || string(raw[0:4]) != "KNNN" {
		return nil, fmt.Errorf("invalid KNN model file")
	}
	pos := 4

	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(raw[pos:])
		pos += 4
		return v
	}

	ver := readU32()
	k := int(readU32())
	distLen := int(readU32())
	distance := string(raw[pos : pos+distLen])
	pos += distLen
	weightLen := int(readU32())
	weight := string(raw[pos : pos+weightLen])
	pos += weightLen
	numSamples := int(readU32())
	numClasses := int(readU32())

	m := &KNNModel{K: k, Distance: distance, Weight: weight, NumClasses: numClasses}

	// Read MaxDistance if v2+
	if ver >= 2 {
		m.MaxDistance = math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:]))
		pos += 8
	}

	m.Samples = make([][FeatureDim]float64, numSamples)
	m.Labels = make([]int32, numSamples)

	for i := 0; i < numSamples; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Samples[i][j] = math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:]))
			pos += 8
		}
		m.Labels[i] = int32(binary.LittleEndian.Uint32(raw[pos:]))
		pos += 4
	}

	return m, nil
}
