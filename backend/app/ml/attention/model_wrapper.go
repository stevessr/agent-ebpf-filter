package attention

import "fmt"

// attentionEnhancedModel applies attention to the feature vector before delegating
// to the wrapped classifier.
type attentionEnhancedModel struct {
	modelType ModelType
	base      Model
	attention AttentionLayer
}

func newAttentionEnhancedModel(modelType ModelType, base Model) Model {
	return &attentionEnhancedModel{
		modelType: modelType,
		base:      base,
		attention: NewAdditiveAttention(),
	}
}

func newAttentionEnhancedModelWithLayer(modelType ModelType, base Model, attention AttentionLayer) Model {
	return &attentionEnhancedModel{
		modelType: modelType,
		base:      base,
		attention: attention,
	}
}

func (m *attentionEnhancedModel) Type() ModelType { return m.modelType }

func (m *attentionEnhancedModel) Predict(features [FeatureDim]float64) Prediction {
	if m.base == nil || m.attention == nil {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	attended := m.attention.Forward(features)
	return m.base.Predict(attended)
}

func (m *attentionEnhancedModel) Serialize(path string) error {
	if m.base == nil {
		return fmt.Errorf("attention-enhanced model has no base model")
	}
	return m.base.Serialize(path)
}

// standaloneAttentionModel wraps an attention layer as a standalone model
// that returns a fixed prediction based on attention output magnitude.
type standaloneAttentionModel struct {
	modelType ModelType
	attention AttentionLayer
}

func newStandaloneAttentionModel(modelType ModelType, attention AttentionLayer) Model {
	return &standaloneAttentionModel{
		modelType: modelType,
		attention: attention,
	}
}

func (m *standaloneAttentionModel) Type() ModelType { return m.modelType }

func (m *standaloneAttentionModel) Predict(features [FeatureDim]float64) Prediction {
	if m.attention == nil {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	attended := m.attention.Forward(features)

	// Compute magnitude of attended output as anomaly score
	var magnitude float64
	for i := 0; i < FeatureDim; i++ {
		magnitude += attended[i] * attended[i]
	}
	magnitude = magnitude / float64(FeatureDim)

	// Normalize to [0, 1] range
	anomalyScore := 1.0 / (1.0 + magnitude)

	// Simple threshold-based action
	action := int32(0)
	if anomalyScore > 0.7 {
		action = 2 // Block
	} else if anomalyScore > 0.4 {
		action = 1 // Alert
	}

	return Prediction{
		Action:       action,
		Confidence:   0.6,
		AnomalyScore: anomalyScore,
	}
}

func (m *standaloneAttentionModel) Serialize(path string) error {
	if m.attention == nil {
		return fmt.Errorf("standalone attention model has no attention layer")
	}
	return m.attention.Serialize(path)
}

