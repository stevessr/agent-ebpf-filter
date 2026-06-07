package app

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
