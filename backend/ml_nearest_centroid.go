package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	RegisterModel(ModelNearestCentroid, func() Model { return NewNearestCentroid("euclidean", false) })
}

// NearestCentroidModel is a simple linear-time tabular baseline.
// It stores one centroid per class and predicts by nearest centroid
// under a selectable metric. A uniform-prior mode helps on imbalanced sets.
type NearestCentroidModel struct {
	Centroids    [][FeatureDim]float64 `json:"-"`
	Priors       []float64             `json:"priors,omitempty"`
	Classes      int                   `json:"classes"`
	Metric       string                `json:"metric"`
	UniformPrior bool                  `json:"uniformPrior"`
}

func NewNearestCentroid(metric string, uniformPrior bool) *NearestCentroidModel {
	if metric == "" {
		metric = "euclidean"
	}
	return &NearestCentroidModel{
		Classes:      4,
		Metric:       strings.ToLower(metric),
		UniformPrior: uniformPrior,
	}
}

func (m *NearestCentroidModel) Type() ModelType { return ModelNearestCentroid }

func (m *NearestCentroidModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Centroids) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	scores := make([]float64, m.Classes)
	bestClass := int32(0)
	bestScore := math.Inf(-1)
	for c := 0; c < m.Classes; c++ {
		if len(m.Priors) == m.Classes && m.Priors[c] <= 0 {
			scores[c] = math.Inf(-1)
			continue
		}
		score := -m.distance(features, m.Centroids[c])
		if !m.UniformPrior && len(m.Priors) == m.Classes && m.Priors[c] > 0 {
			score += math.Log(m.Priors[c])
		}
		scores[c] = score
		if score > bestScore {
			bestScore = score
			bestClass = int32(c)
		}
	}

	maxScore := scores[0]
	for _, s := range scores[1:] {
		if s > maxScore {
			maxScore = s
		}
	}
	sumExp := 0.0
	bestExp := 0.0
	for c, score := range scores {
		e := math.Exp(score - maxScore)
		sumExp += e
		if int32(c) == bestClass {
			bestExp = e
		}
	}
	confidence := 0.0
	if sumExp > 0 {
		confidence = bestExp / sumExp
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *NearestCentroidModel) distance(a, b [FeatureDim]float64) float64 {
	switch m.Metric {
	case "cosine":
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
		return 1.0 - dot/(math.Sqrt(normA)*math.Sqrt(normB))
	case "manhattan":
		sum := 0.0
		for i := 0; i < FeatureDim; i++ {
			sum += math.Abs(a[i] - b[i])
		}
		return sum
	default:
		sum := 0.0
		for i := 0; i < FeatureDim; i++ {
			diff := a[i] - b[i]
			sum += diff * diff
		}
		return math.Sqrt(sum)
	}
}

func (m *NearestCentroidModel) Serialize(path string) error {
	data := make([]byte, 0, 8+len(m.Centroids)*(FeatureDim+1)*8)
	putU32 := func(v uint32) {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		data = append(data, b...)
	}
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	data = append(data, []byte("NCEN")...)
	putU32(1)
	putU32(uint32(m.Classes))
	putU32(uint32(len(m.Metric)))
	data = append(data, []byte(m.Metric)...)
	if m.UniformPrior {
		putU32(1)
	} else {
		putU32(0)
	}
	putU32(uint32(len(m.Priors)))
	for _, p := range m.Priors {
		putF64(p)
	}
	putU32(uint32(len(m.Centroids)))
	for _, centroid := range m.Centroids {
		for _, v := range centroid {
			putF64(v)
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DeserializeNearestCentroid(path string) (*NearestCentroidModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 || string(raw[:4]) != "NCEN" {
		return nil, fmt.Errorf("invalid nearest centroid model file")
	}
	pos := 4
	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(raw[pos:])
		pos += 4
		return v
	}
	readF64 := func() float64 {
		v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:]))
		pos += 8
		return v
	}
	_ = readU32() // version
	classes := int(readU32())
	metricLen := int(readU32())
	if pos+metricLen > len(raw) {
		return nil, fmt.Errorf("invalid nearest centroid model file")
	}
	metric := strings.ToLower(string(raw[pos : pos+metricLen]))
	pos += metricLen
	uniformPrior := readU32() != 0
	priorsLen := int(readU32())
	if priorsLen < 0 || priorsLen > 256 {
		return nil, fmt.Errorf("invalid nearest centroid model file")
	}
	priors := make([]float64, priorsLen)
	for i := range priors {
		priors[i] = readF64()
	}
	centroidCount := int(readU32())
	centroids := make([][FeatureDim]float64, centroidCount)
	for c := 0; c < centroidCount; c++ {
		for d := 0; d < FeatureDim; d++ {
			centroids[c][d] = readF64()
		}
	}
	return &NearestCentroidModel{
		Centroids:    centroids,
		Priors:       priors,
		Classes:      classes,
		Metric:       metric,
		UniformPrior: uniformPrior,
	}, nil
}
