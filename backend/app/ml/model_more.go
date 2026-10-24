package ml

import (
	"encoding/binary"
	"math"
	"math/rand"
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
	Means   [][FeatureDim]float64
	Vars    [][FeatureDim]float64
	Priors  []float64
	Classes int
}

func NewNaiveBayes() *NaiveBayesModel      { return &NaiveBayesModel{Classes: 4} }
func (m *NaiveBayesModel) Type() ModelType { return ModelNaiveBayes }

func (m *NaiveBayesModel) Predict(features [FeatureDim]float64) Prediction {
	if m.Classes <= 0 || len(m.Means) < m.Classes || len(m.Vars) < m.Classes || len(m.Priors) < m.Classes {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	bestClass := int32(0)
	bestLogProb := math.Inf(-1)
	logProbs := make([]float64, m.Classes)
	for c := range logProbs {
		logProbs[c] = math.Inf(-1)
	}
	for c := 0; c < m.Classes; c++ {
		// The smoothed 1e-10 prior previously made a never-observed class
		// artificially competitive: zero variance across 128 dimensions
		// could overwhelm its tiny prior and produce a confident false BLOCK.
		// No observed examples means this class is unavailable to predict.
		if m.Priors[c] <= 0 || math.IsNaN(m.Priors[c]) {
			continue
		}
		logProb := math.Log(m.Priors[c])
		for d := 0; d < FeatureDim; d++ {
			variance := math.Max(m.Vars[c][d], 0) + 1e-8
			diff := features[d] - m.Means[c][d]
			logProb += -0.5 * (diff*diff/variance + math.Log(2*math.Pi*variance))
		}
		if math.IsNaN(logProb) {
			continue
		}
		logProbs[c] = logProb
		if logProb > bestLogProb {
			bestLogProb = logProb
			bestClass = int32(c)
		}
	}
	if math.IsInf(bestLogProb, -1) {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	sumExp := 0.0
	for _, lp := range logProbs {
		sumExp += math.Exp(lp - bestLogProb)
	}
	if sumExp <= 0 || math.IsNaN(sumExp) || math.IsInf(sumExp, 0) {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	confidence := 1.0 / sumExp
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *NaiveBayesModel) Serialize(path string) error {
	data := []byte("NBAY")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(m.Classes))
	for c := 0; c < m.Classes; c++ {
		putF64(m.Priors[c])
		for d := 0; d < FeatureDim; d++ {
			putF64(m.Means[c][d])
			putF64(m.Vars[c][d])
		}
	}
	return writeModelFileAtomic(path, data, 0o644)
}

func DeserializeNaiveBayes(path string) (*NaiveBayesModel, error) {
	r, err := newMLBinaryModelReader(path, "NBAY")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	nc := r.readBoundedCount("Naive Bayes class count", 1, mlBinaryMaxClasses)
	r.requireItems("Naive Bayes", nc, mlBinaryNaiveBayesRowBytes, 0)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &NaiveBayesModel{Classes: nc}
	m.Means = make([][FeatureDim]float64, nc)
	m.Vars = make([][FeatureDim]float64, nc)
	m.Priors = make([]float64, nc)
	for c := 0; c < nc; c++ {
		m.Priors[c] = r.readF64()
		for d := 0; d < FeatureDim; d++ {
			m.Means[c][d] = r.readF64()
			m.Vars[c][d] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}

// ── 5. Extra Trees ─────────────────────────────────────────────────

type ExtraTreesModel struct {
	Forest   *DecisionForest
	MaxDepth int
	NumTrees int
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

// BuildExtraTrees preserves the historical uniform-bootstrap API. For a
// class-balanced variant (without changing inference serialization), use the
// explicit option below, enabled only by the caller's BalanceClasses setting.
func BuildExtraTrees(trainSet []TrainSample, numTrees, maxDepth, minLeaf int, seed int64) *DecisionForest {
	return BuildExtraTreesWithBalance(trainSet, numTrees, maxDepth, minLeaf, seed, false)
}

func BuildExtraTreesWithBalance(trainSet []TrainSample, numTrees, maxDepth, minLeaf int, seed int64, balance bool) *DecisionForest {
	if numTrees < 1 {
		numTrees = 1
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	if minLeaf < 1 {
		minLeaf = 1
	}
	rng := rand.New(rand.NewSource(seed))
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	if len(trainSet) == 0 {
		return forest // untrained, never synthesize a successful prediction
	}
	fCount := int(math.Sqrt(float64(FeatureDim)))
	if fCount < 1 {
		fCount = 1
	}
	for ti := 0; ti < numTrees; ti++ {
		bootstrap := make([]TrainSample, len(trainSet))
		if balance {
			classStratifiedBootstrap(trainSet, bootstrap, rng)
		} else {
			for i := range bootstrap {
				bootstrap[i] = trainSet[rng.Intn(len(trainSet))]
			}
		}
		forest.Trees[ti] = DecisionTree{
			Nodes: buildExtraTree(bootstrap, 0, maxDepth, minLeaf, fCount, rng),
		}
	}
	forest.IsTrained = true
	return forest
}

func buildExtraTree(samples []TrainSample, depth, maxDepth, minLeaf, fCount int, rng *rand.Rand) []DecisionNode {
	leaf := func() []DecisionNode {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}
	if minLeaf < 1 {
		minLeaf = 1
	}
	if depth >= maxDepth || len(samples) < 2*minLeaf || len(samples) == 0 {
		return leaf()
	}
	allSame := true
	for _, s := range samples[1:] {
		if s.label != samples[0].label {
			allSame = false
			break
		}
	}
	if allSame {
		return leaf()
	}

	// Extra Trees: random feature subset and randomized thresholds, selecting
	// the highest *valid* Gini gain. Previously fCount was ignored, leaving a
	// 127/128 chance of a useless split when only one signal dimension varied.
	if fCount > FeatureDim {
		fCount = FeatureDim
	}
	if fCount <= 0 {
		return leaf()
	}
	parentGini := giniImpurity(samples)
	bestGain := 0.0
	bestFeature := -1
	bestThreshold := 0.0
	features := rng.Perm(FeatureDim)
	for index, fi := range features {
		// If the initial random feature subset is completely uninformative,
		// widen the search rather than producing a constant majority leaf.
		// Sparse syscall/category vectors often contain only one informative
		// dimension out of 128; this bounded fallback preserves learnability.
		if index >= fCount && bestGain > 0 {
			break
		}
		minV, maxV := samples[0].features[fi], samples[0].features[fi]
		if math.IsNaN(minV) || math.IsInf(minV, 0) {
			continue
		}
		valid := true
		for _, s := range samples[1:] {
			v := s.features[fi]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
				break
			}
			if v < minV { minV = v }
			if v > maxV { maxV = v }
		}
		if !valid || minV >= maxV {
			continue
		}
		for trial := 0; trial < 3; trial++ {
			// Quantize *before* scoring to match the DecisionNode float32
			// threshold used by the online predictor.
			threshold := float64(float32(minV + rng.Float64()*(maxV-minV)))
			if !(threshold > minV && threshold <= maxV) {
				continue
			}
			var leftCounts, rightCounts [4]int
			nLeft := 0
			for _, s := range samples {
				if s.features[fi] < threshold {
					nLeft++
					if s.label >= 0 && s.label < 4 { leftCounts[s.label]++ }
				} else {
					if s.label >= 0 && s.label < 4 { rightCounts[s.label]++ }
				}
			}
			nRight := len(samples)-nLeft
			if nLeft < minLeaf || nRight < minLeaf {
				continue
			}
			leftWeight := float64(nLeft)/float64(len(samples))
			gain := parentGini -
				leftWeight*giniFromCounts(leftCounts, nLeft) -
				(1-leftWeight)*giniFromCounts(rightCounts, nRight)
			if gain > bestGain {
				bestGain, bestFeature, bestThreshold = gain, fi, threshold
			}
		}
	}
	if bestFeature < 0 {
		return leaf()
	}
	var left, right []TrainSample
	for _, s := range samples {
		if s.features[bestFeature] < bestThreshold {
			left = append(left, s)
		} else {
			right = append(right, s)
		}
	}
	leftNodes := buildExtraTree(left, depth+1, maxDepth, minLeaf, fCount, rng)
	rightNodes := buildExtraTree(right, depth+1, maxDepth, minLeaf, fCount, rng)
	leftOff, rightOff := 1, 1+len(leftNodes)
	for i := range leftNodes {
		if node := &leftNodes[i]; !node.IsLeaf() {
			node.LeftChild += int16(leftOff)
			node.RightChild += int16(leftOff)
		}
	}
	for i := range rightNodes {
		if node := &rightNodes[i]; !node.IsLeaf() {
			node.LeftChild += int16(rightOff)
			node.RightChild += int16(rightOff)
		}
	}
	nodes := []DecisionNode{{FeatureIndex: uint8(bestFeature), Threshold: float32(bestThreshold), LeftChild: int16(leftOff), RightChild: int16(rightOff)}}
	nodes = append(nodes, leftNodes...)
	nodes = append(nodes, rightNodes...)
	return nodes
}

// ── 6. AdaBoost with Decision Stumps ───────────────────────────────

type AdaBoostModel struct {
	Stumps  []AdaBoostStump
	Alphas  []float64
	NEst    int
	Classes int
}
type AdaBoostStump struct {
	Feature   int
	Threshold float64
	LeftVote  float64
	RightVote float64
}

func NewAdaBoost(nEstimators int) *AdaBoostModel {
	if nEstimators < 10 {
		nEstimators = 50
	}
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
		if votes[c] > votes[bestClass] {
			bestClass = int32(c)
		}
	}
	confidence := 0.0
	if totalWeight > 0 {
		confidence = votes[bestClass] / totalWeight
	}
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *AdaBoostModel) Serialize(path string) error {
	data := []byte("ADAB")
	putU32 := func(v uint32) { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); data = append(data, b...) }
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}
	putU32(1)
	putU32(uint32(len(m.Stumps)))
	putU32(uint32(m.Classes))
	for _, s := range m.Stumps {
		putU32(uint32(s.Feature))
		putF64(s.Threshold)
		putF64(s.LeftVote)
		putF64(s.RightVote)
	}
	for _, a := range m.Alphas {
		putF64(a)
	}
	return writeModelFileAtomic(path, data, 0o644)
}

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
