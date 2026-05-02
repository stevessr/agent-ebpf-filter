package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

func init() {
	RegisterModel(ModelLogisticRegression, func() Model { return NewLogisticModel(0.01, "l2", 1000) })
}

// LogisticModel is a one-vs-rest multinomial logistic regression classifier.
// Pure Go SGD implementation — no external ML dependencies.
type LogisticModel struct {
	Weights      [][FeatureDim + 1]float64 `json:"-"` // +1 for bias, one per class
	NumClasses   int                        `json:"numClasses"`
	LearningRate float64                    `json:"learningRate"`
	Regularization string                   `json:"regularization"` // l1, l2, none
	MaxIterations int                       `json:"maxIterations"`
}

func NewLogisticModel(learningRate float64, reg string, maxIter int) *LogisticModel {
	if learningRate <= 0 {
		learningRate = 0.01
	}
	if maxIter <= 0 {
		maxIter = 1000
	}
	return &LogisticModel{
		NumClasses:    4,
		LearningRate:  learningRate,
		Regularization: reg,
		MaxIterations: maxIter,
	}
}

func (m *LogisticModel) Type() ModelType { return ModelLogisticRegression }

func sigmoid(x float64) float64 {
	if x > 20 {
		return 1.0
	}
	if x < -20 {
		return 0.0
	}
	return 1.0 / (1.0 + math.Exp(-x))
}

func (m *LogisticModel) dot(features [FeatureDim]float64, classIdx int) float64 {
	sum := m.Weights[classIdx][FeatureDim] // bias
	for i := 0; i < FeatureDim; i++ {
		sum += m.Weights[classIdx][i] * features[i]
	}
	return sum
}

func (m *LogisticModel) softmax(features [FeatureDim]float64) []float64 {
	probs := make([]float64, m.NumClasses)
	maxLogit := math.Inf(-1)
	logits := make([]float64, m.NumClasses)
	for c := 0; c < m.NumClasses; c++ {
		logits[c] = m.dot(features, c)
		if logits[c] > maxLogit {
			maxLogit = logits[c]
		}
	}
	sum := 0.0
	for c := 0; c < m.NumClasses; c++ {
		probs[c] = math.Exp(logits[c] - maxLogit)
		sum += probs[c]
	}
	for c := 0; c < m.NumClasses; c++ {
		probs[c] /= sum
	}
	return probs
}

func (m *LogisticModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	probs := m.softmax(features)

	bestClass := int32(0)
	bestProb := probs[0]
	for c := 1; c < m.NumClasses; c++ {
		if probs[c] > bestProb {
			bestProb = probs[c]
			bestClass = int32(c)
		}
	}

	// Anomaly: 1 - max_prob (uncertain prediction = anomalous)
	anomalyScore := 1.0 - bestProb

	return Prediction{Action: bestClass, Confidence: bestProb, AnomalyScore: anomalyScore}
}

// Train runs SGD on the provided samples and labels.
func (m *LogisticModel) Train(samples [][FeatureDim]float64, labels []int32) {
	nSamples := len(samples)
	if nSamples == 0 {
		return
	}

	// Initialize weights with small random values
	m.Weights = make([][FeatureDim + 1]float64, m.NumClasses)
	rng := rand.New(rand.NewSource(42))
	for c := 0; c < m.NumClasses; c++ {
		for i := 0; i <= FeatureDim; i++ {
			m.Weights[c][i] = (rng.Float64() - 0.5) * 0.01
		}
	}

	// SGD loop
	for iter := 0; iter < m.MaxIterations; iter++ {
		// Learning rate decay
		lr := m.LearningRate * (1.0 - float64(iter)/float64(m.MaxIterations)*0.95)

		// Shuffle
		order := rng.Perm(nSamples)
		totalLoss := 0.0

		for _, idx := range order {
			features := samples[idx]
			trueLabel := int(labels[idx])
			if trueLabel < 0 || trueLabel >= m.NumClasses {
				continue
			}

			probs := m.softmax(features)

			// Cross-entropy loss gradient
			for c := 0; c < m.NumClasses; c++ {
				target := 0.0
				if c == trueLabel {
					target = 1.0
				}
				error := probs[c] - target

				// Update weights with regularization
				for i := 0; i < FeatureDim; i++ {
					grad := error * features[i]
					// L2 regularization
					if m.Regularization == "l2" {
						grad += 0.001 * m.Weights[c][i]
					} else if m.Regularization == "l1" {
						if m.Weights[c][i] > 0 {
							grad += 0.001
						} else if m.Weights[c][i] < 0 {
							grad -= 0.001
						}
					}
					m.Weights[c][i] -= lr * grad
				}
				// Bias update
				m.Weights[c][FeatureDim] -= lr * error
			}

			if probs[trueLabel] > 0 {
				totalLoss += -math.Log(probs[trueLabel])
			}
		}

		// Early stopping if loss is very small
		avgLoss := totalLoss / float64(nSamples)
		if avgLoss < 0.01 && iter > 100 {
			break
		}
	}
}

// Serialize writes the logistic model to a binary file
func (m *LogisticModel) Serialize(path string) error {
	size := 4*5 + m.NumClasses*(FeatureDim+1)*8
	data := make([]byte, 0, size)

	putU32 := func(v uint32) {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		data = append(data, b...)
	}

	data = append(data[:0], []byte("LOGR")...)
	putU32(1) // version
	putU32(uint32(m.NumClasses))
	putU32(uint32(math.Float64bits(m.LearningRate)))
	putU32(uint32(len(m.Regularization)))
	data = append(data, []byte(m.Regularization)...)

	putU32F := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}

	for c := 0; c < m.NumClasses; c++ {
		for i := 0; i <= FeatureDim; i++ {
			putU32F(m.Weights[c][i])
		}
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

// DeserializeLogistic loads a logistic model from disk
func DeserializeLogistic(path string) (*LogisticModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 || string(raw[0:4]) != "LOGR" {
		return nil, fmt.Errorf("invalid logistic model file")
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
	pos += 4
	numClasses := int(readU32())
	learningRate := math.Float64frombits(uint64(readU32()))
	regLen := int(readU32())
	regularization := string(raw[pos : pos+regLen])
	pos += regLen

	m := &LogisticModel{
		NumClasses:    numClasses,
		LearningRate:  learningRate,
		Regularization: regularization,
		Weights:       make([][FeatureDim + 1]float64, numClasses),
	}

	for c := 0; c < numClasses; c++ {
		for i := 0; i <= FeatureDim; i++ {
			m.Weights[c][i] = readF64()
		}
	}

	return m, nil
}
