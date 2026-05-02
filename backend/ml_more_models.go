package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

// ── Model Type Registration ────────────────────────────────────────

func init() {
	RegisterModel(ModelNaiveBayes, func() Model { return NewNaiveBayes() })
	RegisterModel(ModelExtraTrees, func() Model { return NewExtraTrees(31, 8) })
	RegisterModel(ModelAdaBoost, func() Model { return NewAdaBoost(50) })
	RegisterModel(ModelSVM, func() Model { return NewSVMModel(0.01, 1000) })
	RegisterModel(ModelRidge, func() Model { return NewRidgeModel(1.0) })
	RegisterModel(ModelPerceptron, func() Model { return NewPerceptron(0.01, 1000) })
	RegisterModel(ModelPassiveAggressive, func() Model { return NewPAModel(1.0, 1000) })
}

// ── 4. Gaussian Naive Bayes ────────────────────────────────────────

type NaiveBayesModel struct {
	Means    [][FeatureDim]float64
	Vars     [][FeatureDim]float64
	Priors   []float64
	Classes  int
}

func NewNaiveBayes() *NaiveBayesModel { return &NaiveBayesModel{Classes: 4} }
func (m *NaiveBayesModel) Type() ModelType { return ModelNaiveBayes }

func (m *NaiveBayesModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Means) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	var bestClass int32
	bestLogProb := math.Inf(-1)
	logProbs := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		logProb := math.Log(m.Priors[c] + 1e-10)
		for d := 0; d < FeatureDim; d++ {
			diff := features[d] - m.Means[c][d]
			logProb += -0.5 * (diff*diff/(m.Vars[c][d]+1e-8) + math.Log(2*math.Pi*(m.Vars[c][d]+1e-8)))
		}
		logProbs[c] = logProb
		if logProb > bestLogProb {
			bestLogProb = logProb
			bestClass = int32(c)
		}
	}
	sumExp := 0.0
	for _, lp := range logProbs {
		sumExp += math.Exp(lp - bestLogProb)
	}
	confidence := 1.0 / (sumExp + 1e-10)
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *NaiveBayesModel) Serialize(path string) error {
	data := []byte("NBAY")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1)
	putU32(uint32(m.Classes))
	for c := 0; c < m.Classes; c++ {
		putF64(m.Priors[c])
		for d := 0; d < FeatureDim; d++ {
			putF64(m.Means[c][d])
			putF64(m.Vars[c][d])
		}
	}
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}

func DeserializeNaiveBayes(path string) (*NaiveBayesModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 8 || string(raw[0:4]) != "NBAY" {
		return nil, fmt.Errorf("invalid NB model")
	}
	pos := 4
	readU32 := func() uint32 { v := binary.LittleEndian.Uint32(raw[pos:]); pos += 4; return v }
	readF64 := func() float64 { v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:])); pos += 8; return v }
	_ = readU32() // version
	nc := int(readU32())
	m := &NaiveBayesModel{Classes: nc}
	m.Means = make([][FeatureDim]float64, nc)
	m.Vars = make([][FeatureDim]float64, nc)
	m.Priors = make([]float64, nc)
	for c := 0; c < nc; c++ {
		m.Priors[c] = readF64()
		for d := 0; d < FeatureDim; d++ {
			m.Means[c][d] = readF64()
			m.Vars[c][d] = readF64()
		}
	}
	return m, nil
}

// ── 5. Extra Trees ─────────────────────────────────────────────────

type ExtraTreesModel struct {
	Forest     *DecisionForest
	MaxDepth   int
	NumTrees   int
}

func NewExtraTrees(numTrees, maxDepth int) *ExtraTreesModel {
	return &ExtraTreesModel{NumTrees: numTrees, MaxDepth: maxDepth}
}
func (m *ExtraTreesModel) Type() ModelType { return ModelExtraTrees }

func (m *ExtraTreesModel) Predict(features [FeatureDim]float64) Prediction {
	if m.Forest == nil {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	return m.Forest.Predict(features)
}

func (m *ExtraTreesModel) Serialize(path string) error {
	if m.Forest != nil {
		return m.Forest.Serialize(path)
	}
	return nil
}

func buildExtraTrees(trainSet []trainSample, numTrees, maxDepth, minLeaf int, seed int64) *DecisionForest {
	rng := rand.New(rand.NewSource(seed))
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	fCount := int(math.Sqrt(float64(FeatureDim)))
	if fCount < 1 { fCount = 1 }
	for ti := 0; ti < numTrees; ti++ {
		bootstrap := make([]trainSample, len(trainSet))
		for i := range bootstrap {
			bootstrap[i] = trainSet[rng.Intn(len(trainSet))]
		}
		nodes := buildExtraTree(bootstrap, 0, maxDepth, minLeaf, fCount, rng)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
	}
	forest.IsTrained = true
	return forest
}

func buildExtraTree(samples []trainSample, depth, maxDepth, minLeaf, fCount int, rng *rand.Rand) []DecisionNode {
	if depth >= maxDepth || len(samples) < minLeaf*2 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}
	allSame := true
	for _, s := range samples[1:] {
		if s.label != samples[0].label { allSame = false; break }
	}
	if allSame {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: float32(samples[0].label)}}
	}

	// Extra Trees: pick random feature AND random threshold
	fi := rng.Intn(FeatureDim)
	minV, maxV := samples[0].features[fi], samples[0].features[fi]
	for _, s := range samples {
		if s.features[fi] < minV { minV = s.features[fi] }
		if s.features[fi] > maxV { maxV = s.features[fi] }
	}
	threshold := minV + rng.Float64()*(maxV-minV)
	if minV >= maxV {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	var left, right []trainSample
	for _, s := range samples {
		if s.features[fi] < threshold {
			left = append(left, s)
		} else {
			right = append(right, s)
		}
	}
	if len(left) == 0 || len(right) == 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	leftNodes := buildExtraTree(left, depth+1, maxDepth, minLeaf, fCount, rng)
	rightNodes := buildExtraTree(right, depth+1, maxDepth, minLeaf, fCount, rng)

	leftOff, rightOff := 1, 1+len(leftNodes)
	for i := range leftNodes {
		if n := &leftNodes[i]; !n.IsLeaf() { n.LeftChild += int16(leftOff); n.RightChild += int16(leftOff) }
	}
	for i := range rightNodes {
		if n := &rightNodes[i]; !n.IsLeaf() { n.LeftChild += int16(rightOff); n.RightChild += int16(rightOff) }
	}

	nodes := []DecisionNode{{FeatureIndex: uint8(fi), Threshold: float32(threshold), LeftChild: int16(leftOff), RightChild: int16(rightOff)}}
	nodes = append(nodes, leftNodes...)
	nodes = append(nodes, rightNodes...)
	return nodes
}

// ── 6. AdaBoost with Decision Stumps ───────────────────────────────

type AdaBoostModel struct {
	Stumps   []adaboostStump
	Alphas   []float64
	NEst     int
	Classes  int
}
type adaboostStump struct {
	Feature   int
	Threshold float64
	LeftVote  float64
	RightVote float64
}

func NewAdaBoost(nEstimators int) *AdaBoostModel {
	if nEstimators < 10 { nEstimators = 50 }
	return &AdaBoostModel{NEst: nEstimators, Classes: 4}
}
func (m *AdaBoostModel) Type() ModelType { return ModelAdaBoost }

func (m *AdaBoostModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Stumps) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	votes := make([]float64, m.Classes)
	totalWeight := 0.0
	for i, s := range m.Stumps {
		var vote float64
		if features[s.Feature] < s.Threshold {
			vote = s.LeftVote
		} else {
			vote = s.RightVote
		}
		ci := int(vote)
		if ci >= 0 && ci < m.Classes {
			votes[ci] += m.Alphas[i]
		}
		totalWeight += m.Alphas[i]
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if votes[c] > votes[bestClass] { bestClass = int32(c) }
	}
	confidence := 0.0
	if totalWeight > 0 { confidence = votes[bestClass] / totalWeight }
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *AdaBoostModel) Serialize(path string) error {
	data := []byte("ADAB")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1); putU32(uint32(len(m.Stumps))); putU32(uint32(m.Classes))
	for _, s := range m.Stumps {
		putU32(uint32(s.Feature)); putF64(s.Threshold); putF64(s.LeftVote); putF64(s.RightVote)
	}
	for _, a := range m.Alphas { putF64(a) }
	dir := filepath.Dir(path); os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}

// ── 7. Linear SVM (SGD with hinge loss) ────────────────────────────

type SVMModel struct {
	Weights    [][FeatureDim + 1]float64
	Classes    int
	LR         float64
	MaxIter    int
	C          float64 // regularization strength
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
		if scores[c] > scores[bestClass] { bestClass = int32(c) }
	}
	// Platt scaling approximation
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]))
	if confidence > 1 { confidence = 1 }
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *SVMModel) Serialize(path string) error {
	data := []byte("SVM0")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1); putU32(uint32(m.Classes))
	putF64(m.LR); putF64(m.C)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ { putF64(m.Weights[c][d]) }
	}
	dir := filepath.Dir(path); os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}

// ── 8. Ridge Classifier ────────────────────────────────────────────

type RidgeModel struct {
	Weights  [][FeatureDim + 1]float64
	Classes  int
	Alpha    float64
}

func NewRidgeModel(alpha float64) *RidgeModel { return &RidgeModel{Classes: 4, Alpha: alpha} }
func (m *RidgeModel) Type() ModelType { return ModelRidge }

func (m *RidgeModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	scores := make([]float64, m.Classes)
	for c := 0; c < m.Classes; c++ {
		scores[c] = m.Weights[c][FeatureDim]
		for d := 0; d < FeatureDim; d++ { scores[c] += m.Weights[c][d] * features[d] }
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] { bestClass = int32(c) }
	}
	confidence := 1.0 / (1.0 + math.Exp(-(scores[bestClass] - 0.5)))
	if confidence > 1 { confidence = 1 }
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *RidgeModel) Serialize(path string) error {
	data := []byte("RIDG")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1); putU32(uint32(m.Classes)); putF64(m.Alpha)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ { putF64(m.Weights[c][d]) }
	}
	dir := filepath.Dir(path); os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}

// ── 9. Perceptron ──────────────────────────────────────────────────

type PerceptronModel struct {
	Weights  [][FeatureDim + 1]float64
	Classes  int
	LR       float64
	MaxIter  int
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
		for d := 0; d < FeatureDim; d++ { scores[c] += m.Weights[c][d] * features[d] }
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] { bestClass = int32(c) }
	}
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]+scores[0]))
	if confidence > 1 { confidence = 1 }
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *PerceptronModel) Serialize(path string) error {
	data := []byte("PERC")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1); putU32(uint32(m.Classes)); putF64(m.LR)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ { putF64(m.Weights[c][d]) }
	}
	dir := filepath.Dir(path); os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}

// ── 10. Passive Aggressive Classifier ──────────────────────────────

type PAModel struct {
	Weights  [][FeatureDim + 1]float64
	Classes  int
	C        float64
	MaxIter  int
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
		for d := 0; d < FeatureDim; d++ { scores[c] += m.Weights[c][d] * features[d] }
	}
	bestClass := int32(0)
	for c := 1; c < m.Classes; c++ {
		if scores[c] > scores[bestClass] { bestClass = int32(c) }
	}
	confidence := 1.0 / (1.0 + math.Exp(-scores[bestClass]+scores[0]))
	if confidence > 1 { confidence = 1 }
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *PAModel) Serialize(path string) error {
	data := []byte("PASG")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, math.Float64bits(v)); data = append(data, b...) }
	putU32(1); putU32(uint32(m.Classes)); putF64(m.C)
	for c := 0; c < m.Classes; c++ {
		for d := 0; d <= FeatureDim; d++ { putF64(m.Weights[c][d]) }
	}
	dir := filepath.Dir(path); os.MkdirAll(dir, 0755)
	os.WriteFile(path+".tmp", data, 0644)
	return os.Rename(path+".tmp", path)
}
