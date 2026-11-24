package ml

import (
	"encoding/binary"
	"math"
)

// ── 7. Linear SVM (SGD with hinge loss) ────────────────────────────

type SVMModel struct {
	Weights [][FeatureDim + 1]float64
	Classes int
	LR      float64
	MaxIter int
	C       float64 // regularization strength
}

func NewSVMModel(lr float64, maxIter int) *SVMModel {
	return &SVMModel{Classes: 4, LR: lr, MaxIter: maxIter, C: 1.0}
}
func (m *SVMModel) Type() ModelType { return ModelSVM }

func (m *SVMModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	scores := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		scores[c] = m.Weights[c][FeatureDim] // bias
		for d := 0; d < FeatureDim; d++ {
			scores[c] += m.Weights[c][d] * features[d]
		}
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] {
			bestClass = int32(c)
		}
	}
	// Platt scaling approximation
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]))
	if confidence > 1 {
		confidence = 1
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *SVMModel) Serialize(path string) error {
	data := []byte("SVM0")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(m.Classes))
	putF64(m.LR)
	putF64(m.C)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			putF64(m.Weights[c][d])
		}
	}
	return writeModelFileAtomic(path, data, 0o644)
}

// ── 8. Ridge Classifier ────────────────────────────────────────────

type RidgeModel struct {
	Weights [][FeatureDim + 1]float64
	Classes int
	Alpha   float64
}

func NewRidgeModel(alpha float64) *RidgeModel { return &RidgeModel{Classes: 4, Alpha: alpha} }
func (m *RidgeModel) Type() ModelType         { return ModelRidge }

func (m *RidgeModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	scores := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		scores[c] = m.Weights[c][FeatureDim]
		for d := 0; d < FeatureDim; d++ {
			scores[c] += m.Weights[c][d] * features[d]
		}
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] {
			bestClass = int32(c)
		}
	}
	confidence := 1.0 / (1.0 + math.Exp(-(scores[bestClass] - 0.5)))
	if confidence > 1 {
		confidence = 1
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *RidgeModel) Serialize(path string) error {
	data := []byte("RIDG")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(m.Classes))
	putF64(m.Alpha)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			putF64(m.Weights[c][d])
		}
	}
	return writeModelFileAtomic(path, data, 0o644)
}

// ── 9. Perceptron ──────────────────────────────────────────────────

type PerceptronModel struct {
	Weights [][FeatureDim + 1]float64
	Classes int
	LR      float64
	MaxIter int
}

func NewPerceptron(lr float64, maxIter int) *PerceptronModel {
	return &PerceptronModel{Classes: 4, LR: lr, MaxIter: maxIter}
}
func (m *PerceptronModel) Type() ModelType { return ModelPerceptron }

func (m *PerceptronModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	scores := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		scores[c] = m.Weights[c][FeatureDim]
		for d := 0; d < FeatureDim; d++ {
			scores[c] += m.Weights[c][d] * features[d]
		}
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] {
			bestClass = int32(c)
		}
	}
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]+scores[0]))
	if confidence > 1 {
		confidence = 1
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *PerceptronModel) Serialize(path string) error {
	data := []byte("PERC")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(m.Classes))
	putF64(m.LR)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			putF64(m.Weights[c][d])
		}
	}
	return writeModelFileAtomic(path, data, 0o644)
}

// ── 10. Passive Aggressive Classifier ──────────────────────────────

type PAModel struct {
	Weights [][FeatureDim + 1]float64
	Classes int
	C       float64
	MaxIter int
}

func NewPAModel(C float64, maxIter int) *PAModel {
	return &PAModel{Classes: 4, C: C, MaxIter: maxIter}
}
func (m *PAModel) Type() ModelType { return ModelPassiveAggressive }

func (m *PAModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	scores := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		scores[c] = m.Weights[c][FeatureDim]
		for d := 0; d < FeatureDim; d++ {
			scores[c] += m.Weights[c][d] * features[d]
		}
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] {
			bestClass = int32(c)
		}
	}
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]+scores[0]))
	if confidence > 1 {
		confidence = 1
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *PAModel) Serialize(path string) error {
	data := []byte("PASG")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(m.Classes))
	putF64(m.C)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			putF64(m.Weights[c][d])
		}
	}
	return writeModelFileAtomic(path, data, 0o644)
}
