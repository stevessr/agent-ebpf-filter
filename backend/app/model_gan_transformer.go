package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"agent-ebpf-filter/core"
)

const (
	ganTransformerClasses         = 4
	ganTransformerDefaultHeads    = 4
	ganTransformerDefaultTokens   = 8
	ganTransformerDefaultLR       = 0.035
	ganTransformerSyntheticWeight = 0.45
)

// GANTransformerEncoder is a compact transformer encoder block for fixed
// 128-dim tabular feature vectors. It treats the feature vector as a short token
// sequence and applies deterministic multi-head self-attention before the GAN
// discriminator head consumes the encoded vector.
type GANTransformerEncoder struct {
	NumHeads   int       `json:"numHeads"`
	TokenCount int       `json:"tokenCount"`
	TokenDim   int       `json:"tokenDim"`
	Wq         []float64 `json:"wq"`
	Wk         []float64 `json:"wk"`
	Wv         []float64 `json:"wv"`
	Wo         []float64 `json:"wo"`
}

func NewGANTransformerEncoder(numHeads int) *GANTransformerEncoder {
	tokenCount, tokenDim := ganTransformerTokenShape()
	e := &GANTransformerEncoder{
		NumHeads:   numHeads,
		TokenCount: tokenCount,
		TokenDim:   tokenDim,
	}
	e.ensure()
	return e
}

func ganTransformerTokenShape() (int, int) {
	tokenCount := ganTransformerDefaultTokens
	for tokenCount > 1 && FeatureDim%tokenCount != 0 {
		tokenCount--
	}
	return tokenCount, FeatureDim / tokenCount
}

func (e *GANTransformerEncoder) ensure() {
	if e.TokenCount <= 0 || e.TokenDim <= 0 || e.TokenCount*e.TokenDim != FeatureDim {
		e.TokenCount, e.TokenDim = ganTransformerTokenShape()
	}
	if e.NumHeads <= 0 || e.TokenDim%e.NumHeads != 0 {
		e.NumHeads = ganTransformerDefaultHeads
		if e.TokenDim%e.NumHeads != 0 {
			e.NumHeads = 1
		}
	}
	size := e.TokenDim * e.TokenDim
	if len(e.Wq) != size {
		e.Wq = ganTransformerInitProjection(e.TokenDim, 1)
	}
	if len(e.Wk) != size {
		e.Wk = ganTransformerInitProjection(e.TokenDim, 2)
	}
	if len(e.Wv) != size {
		e.Wv = ganTransformerInitProjection(e.TokenDim, 3)
	}
	if len(e.Wo) != size {
		e.Wo = ganTransformerInitProjection(e.TokenDim, 4)
	}
}

func ganTransformerInitProjection(dim, salt int) []float64 {
	out := make([]float64, dim*dim)
	for i := 0; i < dim; i++ {
		for j := 0; j < dim; j++ {
			base := 0.0
			if i == j {
				base = 1.0
			}
			wiggle := math.Sin(float64((i+1)*(j+1)*(salt+3))) * 0.035
			out[i*dim+j] = base*0.82 + wiggle
		}
	}
	return out
}

func (e *GANTransformerEncoder) Encode(x [FeatureDim]float64) [FeatureDim]float64 {
	e.ensure()
	tokens := make([][]float64, e.TokenCount)
	normTokens := make([][]float64, e.TokenCount)
	for t := 0; t < e.TokenCount; t++ {
		raw := make([]float64, e.TokenDim)
		copy(raw, x[t*e.TokenDim:(t+1)*e.TokenDim])
		tokens[t] = raw
		normTokens[t] = ganTransformerLayerNorm(raw)
	}

	q := make([][]float64, e.TokenCount)
	k := make([][]float64, e.TokenCount)
	v := make([][]float64, e.TokenCount)
	for t := 0; t < e.TokenCount; t++ {
		q[t] = ganTransformerProject(normTokens[t], e.Wq, e.TokenDim)
		k[t] = ganTransformerProject(normTokens[t], e.Wk, e.TokenDim)
		v[t] = ganTransformerProject(normTokens[t], e.Wv, e.TokenDim)
	}

	context := make([][]float64, e.TokenCount)
	for t := range context {
		context[t] = make([]float64, e.TokenDim)
	}
	headDim := e.TokenDim / e.NumHeads
	scale := math.Sqrt(float64(headDim))
	if scale <= 0 {
		scale = 1
	}
	for h := 0; h < e.NumHeads; h++ {
		start := h * headDim
		end := start + headDim
		for qi := 0; qi < e.TokenCount; qi++ {
			scores := make([]float64, e.TokenCount)
			maxScore := math.Inf(-1)
			for ki := 0; ki < e.TokenCount; ki++ {
				score := 0.0
				for d := start; d < end; d++ {
					score += q[qi][d] * k[ki][d]
				}
				score /= scale
				scores[ki] = score
				if score > maxScore {
					maxScore = score
				}
			}
			denom := 0.0
			for ki := 0; ki < e.TokenCount; ki++ {
				scores[ki] = math.Exp(scores[ki] - maxScore)
				denom += scores[ki]
			}
			if denom <= 0 || math.IsNaN(denom) || math.IsInf(denom, 0) {
				denom = 1
			}
			for ki := 0; ki < e.TokenCount; ki++ {
				weight := scores[ki] / denom
				for d := start; d < end; d++ {
					context[qi][d] += weight * v[ki][d]
				}
			}
		}
	}

	var out [FeatureDim]float64
	for t := 0; t < e.TokenCount; t++ {
		projected := ganTransformerProject(context[t], e.Wo, e.TokenDim)
		for d := 0; d < e.TokenDim; d++ {
			idx := t*e.TokenDim + d
			activated := 0.5 + 0.5*math.Tanh(projected[d])
			out[idx] = clamp01(0.78*x[idx] + 0.22*activated)
		}
	}
	return out
}

func ganTransformerProject(x []float64, w []float64, dim int) []float64 {
	out := make([]float64, dim)
	for i := 0; i < dim; i++ {
		sum := 0.0
		row := i * dim
		for j := 0; j < dim; j++ {
			sum += w[row+j] * x[j]
		}
		out[i] = sum
	}
	return out
}

func ganTransformerLayerNorm(x []float64) []float64 {
	out := make([]float64, len(x))
	if len(x) == 0 {
		return out
	}
	mean := 0.0
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	variance := 0.0
	for _, v := range x {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(x))
	invStd := 1.0 / math.Sqrt(variance+1e-6)
	for i, v := range x {
		out[i] = (v - mean) * invStd
	}
	return out
}

// GANTransformerModel combines a class-conditioned synthetic generator with a
// transformer-encoded discriminator/classifier. The generator expands sparse
// labeled classes around class centroids, while the discriminator learns a
// multi-class softmax head over transformer-encoded real and generated samples.
type GANTransformerModel struct {
	NumClasses        int                    `json:"numClasses"`
	LatentDim         int                    `json:"latentDim"`
	Epochs            int                    `json:"epochs"`
	SyntheticPerClass int                    `json:"syntheticPerClass"`
	LearningRate      float64                `json:"learningRate"`
	GeneratorScale    float64                `json:"generatorScale"`
	Encoder           *GANTransformerEncoder `json:"encoder"`
	ClassCentroids    [][FeatureDim]float64  `json:"classCentroids"`
	ClassVars         [][FeatureDim]float64  `json:"classVars"`
	ClassCounts       []int                  `json:"classCounts"`
	ClassPriors       []float64              `json:"classPriors"`
	Weights           [][FeatureDim + 1]float64
	IsTrained         bool `json:"isTrained"`
}

type ganTransformerExample struct {
	features [FeatureDim]float64
	label    int32
	weight   float64
}

func NewGANTransformerModel(latentDim, epochs, syntheticPerClass int) *GANTransformerModel {
	m := &GANTransformerModel{
		NumClasses:        ganTransformerClasses,
		LatentDim:         ganClampInt(latentDim, 8, 64),
		Epochs:            ganClampInt(epochs, 4, 96),
		SyntheticPerClass: ganClampInt(syntheticPerClass, 2, 64),
		LearningRate:      ganTransformerDefaultLR,
		GeneratorScale:    0.35,
		Encoder:           NewGANTransformerEncoder(ganTransformerDefaultHeads),
	}
	m.ensure()
	return m
}

func (m *GANTransformerModel) ensure() {
	if m.NumClasses <= 0 {
		m.NumClasses = ganTransformerClasses
	}
	if m.LatentDim <= 0 {
		m.LatentDim = 16
	}
	if m.Epochs <= 0 {
		m.Epochs = 24
	}
	if m.SyntheticPerClass <= 0 {
		m.SyntheticPerClass = 8
	}
	if m.LearningRate <= 0 {
		m.LearningRate = ganTransformerDefaultLR
	}
	if m.GeneratorScale <= 0 {
		m.GeneratorScale = 0.35
	}
	if m.Encoder == nil {
		m.Encoder = NewGANTransformerEncoder(ganTransformerDefaultHeads)
	} else {
		m.Encoder.ensure()
	}
	if len(m.ClassCentroids) != m.NumClasses {
		m.ClassCentroids = make([][FeatureDim]float64, m.NumClasses)
	}
	if len(m.ClassVars) != m.NumClasses {
		m.ClassVars = make([][FeatureDim]float64, m.NumClasses)
	}
	if len(m.ClassCounts) != m.NumClasses {
		m.ClassCounts = make([]int, m.NumClasses)
	}
	if len(m.ClassPriors) != m.NumClasses {
		m.ClassPriors = make([]float64, m.NumClasses)
		for c := range m.ClassPriors {
			m.ClassPriors[c] = 1.0 / float64(m.NumClasses)
		}
	}
	if len(m.Weights) != m.NumClasses {
		m.Weights = make([][FeatureDim + 1]float64, m.NumClasses)
	}
}

func (m *GANTransformerModel) Type() ModelType { return ModelGANTransformer }

func (m *GANTransformerModel) Train(samples []TrainingSample, cfg MLConfig) {
	m.NumClasses = ganTransformerClasses
	if cfg.NumTrees > 0 {
		m.LatentDim = ganClampInt(cfg.NumTrees, 8, 64)
	}
	if cfg.MaxDepth > 0 {
		m.Epochs = ganClampInt(cfg.MaxDepth*4, 4, 96)
	}
	if cfg.MinSamplesLeaf > 0 {
		m.SyntheticPerClass = ganClampInt(cfg.MinSamplesLeaf*2, 2, 64)
	}
	m.ensure()
	m.fitClassStats(samples)
	m.initDiscriminatorWeights()
	examples := m.buildTrainingExamples(samples)
	m.trainDiscriminator(examples)
	m.IsTrained = true
}

func (m *GANTransformerModel) fitClassStats(samples []TrainingSample) {
	m.ensure()
	for c := 0; c < m.NumClasses; c++ {
		m.ClassCentroids[c] = [FeatureDim]float64{}
		m.ClassVars[c] = [FeatureDim]float64{}
		m.ClassCounts[c] = 0
	}
	var global [FeatureDim]float64
	total := 0
	for _, sample := range samples {
		if sample.Label < 0 || int(sample.Label) >= m.NumClasses {
			continue
		}
		c := int(sample.Label)
		m.ClassCounts[c]++
		total++
		for d := 0; d < FeatureDim; d++ {
			value := clamp01(sample.Features[d])
			m.ClassCentroids[c][d] += value
			global[d] += value
		}
	}
	if total > 0 {
		for d := 0; d < FeatureDim; d++ {
			global[d] /= float64(total)
		}
	}
	for c := 0; c < m.NumClasses; c++ {
		if m.ClassCounts[c] == 0 {
			m.ClassCentroids[c] = global
			continue
		}
		for d := 0; d < FeatureDim; d++ {
			m.ClassCentroids[c][d] /= float64(m.ClassCounts[c])
		}
	}
	for _, sample := range samples {
		if sample.Label < 0 || int(sample.Label) >= m.NumClasses {
			continue
		}
		c := int(sample.Label)
		for d := 0; d < FeatureDim; d++ {
			diff := clamp01(sample.Features[d]) - m.ClassCentroids[c][d]
			m.ClassVars[c][d] += diff * diff
		}
	}
	for c := 0; c < m.NumClasses; c++ {
		denom := float64(m.ClassCounts[c])
		if denom <= 1 {
			denom = 1
		}
		for d := 0; d < FeatureDim; d++ {
			m.ClassVars[c][d] = math.Max(m.ClassVars[c][d]/denom, 1e-4)
		}
		m.ClassPriors[c] = (float64(m.ClassCounts[c]) + 1) / (float64(total) + float64(m.NumClasses))
	}
}

func (m *GANTransformerModel) initDiscriminatorWeights() {
	m.ensure()
	var global [FeatureDim]float64
	total := 0
	for c, count := range m.ClassCounts {
		total += count
		for d := 0; d < FeatureDim; d++ {
			global[d] += m.ClassCentroids[c][d] * float64(count)
		}
	}
	if total > 0 {
		for d := 0; d < FeatureDim; d++ {
			global[d] /= float64(total)
		}
	}
	for c := 0; c < m.NumClasses; c++ {
		for d := 0; d < FeatureDim; d++ {
			m.Weights[c][d] = (m.ClassCentroids[c][d] - global[d]) * 0.25
		}
		m.Weights[c][FeatureDim] = math.Log(m.ClassPriors[c] + 1e-9)
	}
}

func (m *GANTransformerModel) buildTrainingExamples(samples []TrainingSample) []ganTransformerExample {
	examples := make([]ganTransformerExample, 0, len(samples)+m.NumClasses*m.SyntheticPerClass)
	for _, sample := range samples {
		if sample.Label < 0 || int(sample.Label) >= m.NumClasses {
			continue
		}
		examples = append(examples, ganTransformerExample{
			features: clampFeatureVector(sample.Features),
			label:    sample.Label,
			weight:   1.0,
		})
	}
	for c := 0; c < m.NumClasses; c++ {
		if m.ClassCounts[c] == 0 {
			continue
		}
		for i := 0; i < m.SyntheticPerClass; i++ {
			examples = append(examples, ganTransformerExample{
				features: m.Generate(int32(c), i),
				label:    int32(c),
				weight:   ganTransformerSyntheticWeight,
			})
		}
	}
	return examples
}

func (m *GANTransformerModel) trainDiscriminator(examples []ganTransformerExample) {
	if len(examples) == 0 {
		return
	}
	l2 := 1e-4
	for epoch := 0; epoch < m.Epochs; epoch++ {
		phase := epoch % len(examples)
		for i := 0; i < len(examples); i++ {
			ex := examples[(i+phase)%len(examples)]
			if ex.label < 0 || int(ex.label) >= m.NumClasses {
				continue
			}
			encoded := m.Encoder.Encode(ex.features)
			probs := m.softmaxEncoded(encoded)
			decay := 1.0 - m.LearningRate*l2
			if decay < 0.99 {
				decay = 0.99
			}
			for c := 0; c < m.NumClasses; c++ {
				target := 0.0
				if int32(c) == ex.label {
					target = 1.0
				}
				grad := (target - probs[c]) * m.LearningRate * ex.weight
				for d := 0; d < FeatureDim; d++ {
					m.Weights[c][d] = m.Weights[c][d]*decay + grad*encoded[d]
				}
				m.Weights[c][FeatureDim] += grad
			}
		}
	}
}

// Generate emits a class-conditioned synthetic feature vector. The generator
// combines class centroids, deterministic latent noise, adversarial boundary
// mixing, and the transformer encoder's attention-shaped residual.
func (m *GANTransformerModel) Generate(label int32, nonce int) [FeatureDim]float64 {
	m.ensure()
	c := int(label)
	if c < 0 || c >= m.NumClasses {
		c = 0
	}
	var candidate [FeatureDim]float64
	other := (c + nonce + 1) % m.NumClasses
	if m.ClassCounts[other] == 0 {
		other = c
	}
	for d := 0; d < FeatureDim; d++ {
		std := math.Sqrt(math.Max(m.ClassVars[c][d], 1e-4))
		noise := m.latentNoise(c, nonce, d)
		value := m.ClassCentroids[c][d] + noise*std*m.GeneratorScale
		if other != c {
			value = 0.88*value + 0.12*m.ClassCentroids[other][d]
		}
		candidate[d] = clamp01(value)
	}
	attended := m.Encoder.Encode(candidate)
	for d := 0; d < FeatureDim; d++ {
		candidate[d] = clamp01(0.74*candidate[d] + 0.26*attended[d])
	}
	return candidate
}

func (m *GANTransformerModel) latentNoise(classIdx, nonce, dim int) float64 {
	latentSlot := dim % ganMaxInt(m.LatentDim, 1)
	angle := float64((classIdx+1)*97+(nonce+1)*53+(latentSlot+1)*19+(dim+1)*7) * 0.173
	return 0.65*math.Sin(angle) + 0.35*math.Cos(angle*0.37)
}

func (m *GANTransformerModel) Predict(features [FeatureDim]float64) Prediction {
	m.ensure()
	if !m.IsTrained || len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	cleaned := clampFeatureVector(features)
	encoded := m.Encoder.Encode(cleaned)
	probs := m.softmaxEncoded(encoded)
	bestClass := int32(0)
	bestProb := probs[0]
	for c := 1; c < m.NumClasses; c++ {
		if probs[c] > bestProb {
			bestProb = probs[c]
			bestClass = int32(c)
		}
	}
	realism := m.classRealism(cleaned, int(bestClass))
	anomaly := clamp01(0.65*(1.0-bestProb) + 0.35*(1.0-realism))
	return Prediction{
		Action:       bestClass,
		Confidence:   clamp01(bestProb),
		AnomalyScore: anomaly,
	}
}

func (m *GANTransformerModel) softmaxEncoded(encoded [FeatureDim]float64) []float64 {
	logits := make([]float64, m.NumClasses)
	maxLogit := math.Inf(-1)
	for c := 0; c < m.NumClasses; c++ {
		score := m.Weights[c][FeatureDim]
		for d := 0; d < FeatureDim; d++ {
			score += m.Weights[c][d] * encoded[d]
		}
		logits[c] = score
		if score > maxLogit {
			maxLogit = score
		}
	}
	denom := 0.0
	for c := 0; c < m.NumClasses; c++ {
		logits[c] = math.Exp(logits[c] - maxLogit)
		denom += logits[c]
	}
	if denom <= 0 || math.IsNaN(denom) || math.IsInf(denom, 0) {
		denom = 1
	}
	for c := 0; c < m.NumClasses; c++ {
		logits[c] /= denom
	}
	return logits
}

func (m *GANTransformerModel) classRealism(features [FeatureDim]float64, classIdx int) float64 {
	if classIdx < 0 || classIdx >= m.NumClasses || len(m.ClassCentroids) != m.NumClasses {
		return 0.5
	}
	dist := 0.0
	for d := 0; d < FeatureDim; d++ {
		std := math.Sqrt(math.Max(m.ClassVars[classIdx][d], 1e-4))
		z := (features[d] - m.ClassCentroids[classIdx][d]) / std
		dist += z * z
	}
	dist = math.Sqrt(dist / float64(FeatureDim))
	return clamp01(math.Exp(-0.5 * dist))
}

func (m *GANTransformerModel) Serialize(path string) error {
	m.ensure()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DeserializeGANTransformer(path string) (*GANTransformerModel, error) {
	raw, err := readBoundedMLModelFile(path, mlJSONMaxModelFileBytes)
	if err != nil {
		return nil, err
	}
	var model GANTransformerModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, fmt.Errorf("invalid GAN Transformer model: %w", err)
	}
	if err := validateGANTransformerModel(&model); err != nil {
		return nil, fmt.Errorf("invalid GAN Transformer model: %w", err)
	}
	model.ensure()
	return &model, nil
}

func validateGANTransformerModel(model *GANTransformerModel) error {
	if model.NumClasses != ganTransformerClasses {
		return fmt.Errorf("class count %d", model.NumClasses)
	}
	if model.LatentDim < 8 || model.LatentDim > 64 {
		return fmt.Errorf("latent dimension %d", model.LatentDim)
	}
	if model.Epochs < 4 || model.Epochs > 96 {
		return fmt.Errorf("epoch count %d", model.Epochs)
	}
	if model.SyntheticPerClass < 2 || model.SyntheticPerClass > 64 {
		return fmt.Errorf("synthetic sample count %d", model.SyntheticPerClass)
	}
	if !ganFinite(model.LearningRate) || model.LearningRate <= 0 || model.LearningRate > 1 {
		return fmt.Errorf("learning rate %v", model.LearningRate)
	}
	if !ganFinite(model.GeneratorScale) || model.GeneratorScale <= 0 || model.GeneratorScale > 10 {
		return fmt.Errorf("generator scale %v", model.GeneratorScale)
	}
	if model.Encoder == nil {
		return fmt.Errorf("missing encoder")
	}
	encoder := model.Encoder
	if encoder.TokenCount <= 0 || encoder.TokenCount > FeatureDim || encoder.TokenDim <= 0 || encoder.TokenDim > FeatureDim || encoder.TokenCount*encoder.TokenDim != FeatureDim {
		return fmt.Errorf("encoder token shape %dx%d", encoder.TokenCount, encoder.TokenDim)
	}
	if encoder.NumHeads <= 0 || encoder.NumHeads > encoder.TokenDim || encoder.TokenDim%encoder.NumHeads != 0 {
		return fmt.Errorf("encoder head count %d", encoder.NumHeads)
	}
	projectionSize := encoder.TokenDim * encoder.TokenDim
	for name, values := range map[string][]float64{
		"encoder wq": encoder.Wq,
		"encoder wk": encoder.Wk,
		"encoder wv": encoder.Wv,
		"encoder wo": encoder.Wo,
	} {
		if len(values) != projectionSize || !ganFiniteSlice(values) {
			return fmt.Errorf("%s shape or value", name)
		}
	}
	if len(model.ClassCentroids) != model.NumClasses || len(model.ClassVars) != model.NumClasses ||
		len(model.ClassCounts) != model.NumClasses || len(model.ClassPriors) != model.NumClasses || len(model.Weights) != model.NumClasses {
		return fmt.Errorf("class parameter shape")
	}
	totalCount := 0
	totalPrior := 0.0
	for class := 0; class < model.NumClasses; class++ {
		if model.ClassCounts[class] < 0 || model.ClassCounts[class] > mlMaxTrainingSamples {
			return fmt.Errorf("class %d sample count %d", class, model.ClassCounts[class])
		}
		totalCount += model.ClassCounts[class]
		prior := model.ClassPriors[class]
		if !ganFinite(prior) || prior < 0 || prior > 1 {
			return fmt.Errorf("class %d prior %v", class, prior)
		}
		totalPrior += prior
		for feature := 0; feature < FeatureDim; feature++ {
			if !ganFinite(model.ClassCentroids[class][feature]) || !ganFinite(model.ClassVars[class][feature]) || model.ClassVars[class][feature] < 0 {
				return fmt.Errorf("class %d feature %d statistics", class, feature)
			}
		}
		for feature := 0; feature <= FeatureDim; feature++ {
			if !ganFinite(model.Weights[class][feature]) {
				return fmt.Errorf("class %d weight %d", class, feature)
			}
		}
	}
	if totalCount > mlMaxTrainingSamples {
		return fmt.Errorf("total sample count %d", totalCount)
	}
	if totalPrior <= 0 || math.Abs(totalPrior-1) > 1e-6 {
		return fmt.Errorf("class prior sum %v", totalPrior)
	}
	return nil
}

func ganFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func ganFiniteSlice(values []float64) bool {
	for _, value := range values {
		if !ganFinite(value) {
			return false
		}
	}
	return true
}

// trainGANTransformer trains the GAN + Transformer model on the stored dataset.
func (t *ModelTrainer) trainGANTransformer(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.beginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 8 {
		return nil, TrainResult{Error: "need >=8 labeled samples for GAN Transformer"}
	}

	validationRatio := cfg.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}
	shuffled := append([]TrainingSample(nil), labeled...)
	deterministicShuffleTrainingSamples(shuffled)
	validationCount := int(math.Round(float64(len(shuffled)) * validationRatio))
	if validationCount < 1 {
		validationCount = 1
	}
	if validationCount >= len(shuffled) {
		validationCount = len(shuffled) - 1
	}
	trainCount := len(shuffled) - validationCount
	trainRaw := append([]TrainingSample(nil), shuffled[:trainCount]...)
	validationRaw := append([]TrainingSample(nil), shuffled[trainCount:]...)

	latentDim := cfg.NumTrees
	if latentDim <= 0 {
		latentDim = 16
	}
	epochs := cfg.MaxDepth * 4
	if epochs <= 0 {
		epochs = 24
	}
	syntheticPerClass := cfg.MinSamplesLeaf * 2
	if syntheticPerClass <= 0 {
		syntheticPerClass = 8
	}
	model := NewGANTransformerModel(latentDim, epochs, syntheticPerClass)
	t.logf("GAN+Transformer 训练: latent=%d epochs=%d synthetic/class=%d train=%d validation=%d",
		model.LatentDim, model.Epochs, model.SyntheticPerClass, len(trainRaw), len(validationRaw))
	model.Train(trainRaw, cfg)

	trainAccuracy := evalModelLabeled(model, trainRaw)
	validationAccuracy := evalModelLabeled(model, validationRaw)
	t.logf("GAN+Transformer accuracy: train=%.2f%% validation=%.2f%%",
		trainAccuracy*100, validationAccuracy*100)

	t.finishMetrics(validationAccuracy, trainAccuracy, validationAccuracy, len(labeled), len(trainRaw), len(validationRaw))
	t.setLastSplit(trainRaw, validationRaw)
	return model, TrainResult{
		Accuracy:           validationAccuracy,
		TrainAccuracy:      trainAccuracy,
		ValidationAccuracy: validationAccuracy,
		NumSamples:         len(labeled),
		TrainSamples:       len(trainRaw),
		ValidationSamples:  len(validationRaw),
	}
}

func deterministicShuffleTrainingSamples(samples []TrainingSample) {
	for i := len(samples) - 1; i > 0; i-- {
		j := int((uint64(i)*1103515245 + 12345) % uint64(i+1))
		samples[i], samples[j] = samples[j], samples[i]
	}
}

func clampFeatureVector(features [FeatureDim]float64) [FeatureDim]float64 {
	var out [FeatureDim]float64
	for d := 0; d < FeatureDim; d++ {
		out[d] = clamp01(features[d])
	}
	return out
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func ganClampInt(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func ganMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	RegisterModel(core.ModelGANTransformer, func() Model {
		return NewGANTransformerModel(16, 24, 8)
	})
}
