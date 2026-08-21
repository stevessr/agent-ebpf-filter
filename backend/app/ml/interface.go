package ml

// AttentionLayer is a unified interface for all attention mechanisms
type AttentionLayer interface {
	Forward(x [FeatureDim]float64) [FeatureDim]float64
	Backward(x, gradOut [FeatureDim]float64) [FeatureDim]float64
	Serialize(path string) error
}
