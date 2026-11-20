package ml

import (
	"fmt"
	"math"
)

type ganTransformerExample struct {
	features [FeatureDim]float64
	label    int32
	weight   float64
}

func (m *GANTransformerModel) Train(samples []TrainingSample, cfg MLConfig) {
	m.NumClasses = ganTransformerClasses
	if cfg.NumTrees > 0 {
		m.LatentDim = min(max(cfg.NumTrees, 8), 64)
	}
	if cfg.MaxDepth > 0 {
		m.Epochs = min(max(cfg.MaxDepth*4, 4), 96)
	}
	if cfg.MinSamplesLeaf > 0 {
		m.SyntheticPerClass = min(max(cfg.MinSamplesLeaf*2, 2), 64)
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
			value := Clamp01(sample.Features[d])
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
			diff := Clamp01(sample.Features[d]) - m.ClassCentroids[c][d]
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
			features: clampFeatures01(sample.Features),
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
		candidate[d] = Clamp01(value)
	}
	attended := m.Encoder.Encode(candidate)
	for d := 0; d < FeatureDim; d++ {
		candidate[d] = Clamp01(0.74*candidate[d] + 0.26*attended[d])
	}
	return candidate
}

func (m *GANTransformerModel) latentNoise(classIdx, nonce, dim int) float64 {
	latentSlot := dim % max(m.LatentDim, 1)
	angle := float64((classIdx+1)*97+(nonce+1)*53+(latentSlot+1)*19+(dim+1)*7) * 0.173
	return 0.65*math.Sin(angle) + 0.35*math.Cos(angle*0.37)
}

// trainGANTransformer trains the GAN + Transformer model on the stored dataset.
func (t *ModelTrainer) trainGANTransformer(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 8 {
		return nil, TrainResult{Error: "need >=8 labeled samples for GAN Transformer"}
	}
	_, _, trainRaw, validationRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: fmt.Sprintf("independent holdout: %v", err)}
	}

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
	t.Logf("GAN+Transformer 训练: latent=%d epochs=%d synthetic/class=%d train=%d validation=%d",
		model.LatentDim, model.Epochs, model.SyntheticPerClass, len(trainRaw), len(validationRaw))
	model.Train(trainRaw, cfg)

	trainAccuracy := evalModelLabeled(model, trainRaw)
	validationAccuracy := evalModelLabeled(model, validationRaw)
	t.Logf("GAN+Transformer accuracy: train=%.2f%% validation=%.2f%%",
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
