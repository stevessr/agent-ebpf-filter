package ml

import (
	"agent-ebpf-filter/core"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
			out[idx] = Clamp01(0.78*x[idx] + 0.22*activated)
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

func NewGANTransformerModel(latentDim, epochs, syntheticPerClass int) *GANTransformerModel {
	m := &GANTransformerModel{
		NumClasses:        ganTransformerClasses,
		LatentDim:         min(max(latentDim, 8), 64),
		Epochs:            min(max(epochs, 4), 96),
		SyntheticPerClass: min(max(syntheticPerClass, 2), 64),
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

func (m *GANTransformerModel) Predict(features [FeatureDim]float64) Prediction {
	m.ensure()
	if !m.IsTrained || len(m.Weights) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	cleaned := clampFeatures01(features)
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
	anomaly := Clamp01(0.65*(1.0-bestProb) + 0.35*(1.0-realism))
	return Prediction{
		Action:       bestClass,
		Confidence:   Clamp01(bestProb),
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
	return Clamp01(math.Exp(-0.5 * dist))
}

func (m *GANTransformerModel) Serialize(path string) error {
	m.ensure()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
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

func init() {
	RegisterModel(core.ModelGANTransformer, func() Model {
		return NewGANTransformerModel(16, 24, 8)
	})
}
