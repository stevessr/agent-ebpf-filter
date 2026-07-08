package graph

const NumClasses = 4

const FeatureGroupCount = 16

// ActivationFunc is the type of activation function.
type ActivationFunc int

const (
	ActivationReLU     ActivationFunc = 0
	ActivationLeakyReLU ActivationFunc = 1
	ActivationTanh     ActivationFunc = 2
	ActivationSigmoid  ActivationFunc = 3
	ActivationNone     ActivationFunc = 4
)