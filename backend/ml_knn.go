package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"agent-ebpf-filter/cuda"
)

func init() {
	RegisterModel(ModelKNN, func() Model { return NewKNNModel(5, "euclidean", "uniform") })
}

// KNNModel is a k-nearest neighbors classifier.
// Pure Go — stores training data in memory for lazy inference.
type KNNModel struct {
	K          int      `json:"k"`
	Distance   string   `json:"distance"`   // euclidean, manhattan
	Weight     string   `json:"weight"`     // uniform, distance
	Samples    [][FeatureDim]float64 `json:"-"`
	Labels     []int32  `json:"-"`
	NumClasses int      `json:"numClasses"`
}

func NewKNNModel(k int, distance, weight string) *KNNModel {
	if k < 1 {
		k = 1
	}
	return &KNNModel{K: k, Distance: distance, Weight: weight, NumClasses: 4}
}

func (m *KNNModel) Type() ModelType { return ModelKNN }

func (m *KNNModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Samples) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	type neighbor struct {
		idx      int
		distance float64
	}

	neighbors := make([]neighbor, len(m.Samples))
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
		for i := range neighbors {
			neighbors[i] = neighbor{idx: i, distance: float64(dists[i])}
		}
	} else {
		// CPU fallback
		for i, sample := range m.Samples {
			neighbors[i] = neighbor{idx: i, distance: m.computeDistance(features, sample)}
		}
	}
	sort.Slice(neighbors, func(i, j int) bool {
		return neighbors[i].distance < neighbors[j].distance
	})

	k := m.K
	if k > len(neighbors) {
		k = len(neighbors)
	}

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

	// Anomaly: average distance to k nearest neighbors
	avgDist := 0.0
	for i := 0; i < k; i++ {
		avgDist += neighbors[i].distance
	}
	avgDist /= float64(k)
	anomalyScore := math.Tanh(avgDist) // normalize to [0, ~0.76]

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
	size := 4*6 + len(m.Samples)*(FeatureDim*8+4)
	data := make([]byte, 0, size)

	putU32 := func(v uint32) {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		data = append(data, b...)
	}

	data = append(data, []byte("KNNN")...)
	putU32(1)                          // version
	putU32(uint32(m.K))                // k
	putU32(uint32(len(m.Distance)))    // dist len
	data = append(data, []byte(m.Distance)...)
	putU32(uint32(len(m.Weight)))      // weight len
	data = append(data, []byte(m.Weight)...)
	putU32(uint32(len(m.Samples)))     // sample count
	putU32(uint32(m.NumClasses))

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
	pos := 0
	if len(raw) < 24 || string(raw[0:4]) != "KNNN" {
		return nil, fmt.Errorf("invalid KNN model file")
	}
	pos = 4

	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(raw[pos:])
		pos += 4
		return v
	}

	_ = readU32 // version
	pos += 4
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

func putString16(buf []byte, s string) {
	copy(buf, s)
}
