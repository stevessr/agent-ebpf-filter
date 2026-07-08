// Package graph implements a Graph Neural Network (GNN) for classifying
// security operations captured by eBPF tracing. It models feature interactions
// as a graph and uses message passing to learn complex relationships between
// command characteristics, file access patterns, network behavior, and
// historical context.
package graph

import "math"

// FeatureGroup defines a semantic grouping of features that becomes a node in the graph.
type FeatureGroup struct {
	Name  string
	Start int // inclusive start index in the 128-dim feature vector
	End   int // exclusive end index
	Dim   int // End - Start
}

// DefaultFeatureGroups returns the 16 semantic groupings of the 128-dim feature vector.
// Each group becomes a node in the feature interaction graph.
func DefaultFeatureGroups() []FeatureGroup {
	return []FeatureGroup{
		{Name: "behavior_category", Start: 0, End: 15, Dim: 15},
		{Name: "process_flags", Start: 15, End: 22, Dim: 7},
		{Name: "io_flags", Start: 22, End: 28, Dim: 6},
		{Name: "command_meta", Start: 28, End: 32, Dim: 4},
		{Name: "arg_statistics", Start: 32, End: 38, Dim: 6},
		{Name: "sensitive_paths", Start: 38, End: 48, Dim: 10},
		{Name: "file_extensions", Start: 48, End: 58, Dim: 10},
		{Name: "url_patterns", Start: 58, End: 64, Dim: 6},
		{Name: "embedding_low", Start: 64, End: 80, Dim: 16},
		{Name: "embedding_high", Start: 80, End: 96, Dim: 16},
		{Name: "freq_history", Start: 96, End: 104, Dim: 8},
		{Name: "pattern_history", Start: 104, End: 112, Dim: 8},
		{Name: "temporal", Start: 112, End: 120, Dim: 8},
		{Name: "risk_scores", Start: 120, End: 124, Dim: 4},
		{Name: "network_anomalies", Start: 124, End: 128, Dim: 4},
		{Name: "global_summary", Start: 0, End: 128, Dim: 128},
	}
}

// EdgeDef defines a static edge in the feature interaction graph.
type EdgeDef struct {
	Source int
	Target int
}

// DefaultEdges returns the structural edges connecting feature groups.
// These encode domain knowledge about how feature groups interact.
func DefaultEdges() []EdgeDef {
	return []EdgeDef{
		// Process identity <-> behavior
		{0, 1}, {1, 0},
		{1, 2}, {2, 1},
		{1, 3}, {3, 1},
		// I/O patterns <-> paths and network
		{2, 5}, {5, 2},
		{2, 6}, {6, 2},
		{2, 7}, {7, 2},
		{2, 14}, {14, 2},
		// Command <-> arguments
		{3, 4}, {4, 3},
		{4, 5}, {5, 4},
		{4, 7}, {7, 4},
		// Embedding coherence
		{8, 9}, {9, 8},
		// History chains
		{10, 11}, {11, 10},
		{11, 12}, {12, 11},
		{10, 12}, {12, 10},
		// Risk connections
		{13, 14}, {14, 13},
		{5, 13}, {13, 5},
		{7, 14}, {14, 7},
		// Global summary connects to all
		{15, 0}, {15, 1}, {15, 2}, {15, 3}, {15, 4}, {15, 5}, {15, 6}, {15, 7},
		{15, 8}, {15, 9}, {15, 10}, {15, 11}, {15, 12}, {15, 13}, {15, 14},
		{0, 15}, {1, 15}, {2, 15}, {3, 15}, {4, 15}, {5, 15}, {6, 15}, {7, 15},
		{8, 15}, {9, 15}, {10, 15}, {11, 15}, {12, 15}, {13, 15}, {14, 15},
		// Cross-domain security-critical edges
		{0, 13}, {13, 0},
		{1, 13}, {13, 1},
		{0, 14}, {14, 0},
	}
}

// NodeState holds the state of a node during message passing.
type NodeState struct {
	Features  []float64
	Embedding []float64
	Messages  []float64
}

// GraphInstance represents a constructed feature interaction graph
// for a single input sample.
type GraphInstance struct {
	Nodes    []NodeState
	AdjList  [][]int
	NumNodes int
}

// Activate applies the activation function to a value.
func Activate(x float64, fn ActivationFunc) float64 {
	switch fn {
	case ActivationReLU:
		if x > 0 {
			return x
		}
		return 0
	case ActivationLeakyReLU:
		if x > 0 {
			return x
		}
		return 0.01 * x
	case ActivationTanh:
		return math.Tanh(x)
	case ActivationSigmoid:
		return 1.0 / (1.0 + math.Exp(-x))
	case ActivationNone:
		return x
	}
	return x
}

// ActivateGrad returns the gradient of the activation function.
func ActivateGrad(x float64, activated float64, fn ActivationFunc) float64 {
	switch fn {
	case ActivationReLU:
		if x > 0 {
			return 1.0
		}
		return 0.0
	case ActivationLeakyReLU:
		if x > 0 {
			return 1.0
		}
		return 0.01
	case ActivationTanh:
		return 1.0 - activated*activated
	case ActivationSigmoid:
		return activated * (1.0 - activated)
	case ActivationNone:
		return 1.0
	}
	return 1.0
}
