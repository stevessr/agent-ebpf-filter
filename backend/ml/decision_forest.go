package ml

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// DecisionNode stores one node in a decision tree.
// Flat array layout (pre-order) for cache-friendly inference.
type DecisionNode struct {
	FeatureIndex uint8
	Threshold    float32
	LeftChild    int16
	RightChild   int16
	LeafValue    float32 // class probability when leaf
}

// IsLeaf returns true if this is a terminal node
func (n *DecisionNode) IsLeaf() bool {
	return n.LeftChild == -1 && n.RightChild == -1
}

// DecisionTree is a single tree in the random forest
type DecisionTree struct {
	Nodes []DecisionNode
}

// Predict traverses the tree and returns the leaf value
func (t *DecisionTree) Predict(features [FeatureDim]float64) float32 {
	if len(t.Nodes) == 0 {
		return 0
	}
	nodeIdx := 0
	for {
		node := &t.Nodes[nodeIdx]
		if node.IsLeaf() {
			return node.LeafValue
		}
		if features[node.FeatureIndex] < float64(node.Threshold) {
			nodeIdx = int(node.LeftChild)
		} else {
			nodeIdx = int(node.RightChild)
		}
		if nodeIdx < 0 || nodeIdx >= len(t.Nodes) {
			return 0
		}
	}
}

// DecisionForest is a random forest ensemble of decision trees.
// Pure Go implementation — no external ML dependencies.
type DecisionForest struct {
	Trees       []DecisionTree
	NumClasses  int // 4 for ALLOW/BLOCK/REWRITE/ALERT
	MaxDepth    int
	NumFeatures int
	IsTrained   bool
}

// Type returns the model type identifier
func (f *DecisionForest) Type() ModelType { return "random_forest" }

// NewDecisionForest creates a new random forest
func NewDecisionForest(numTrees, maxDepth, numClasses int) *DecisionForest {
	return &DecisionForest{
		Trees:       make([]DecisionTree, numTrees),
		NumClasses:  numClasses,
		MaxDepth:    maxDepth,
		NumFeatures: FeatureDim,
	}
}

// Predict runs inference on the forest and returns a Prediction
func (f *DecisionForest) Predict(features [FeatureDim]float64) Prediction {
	if !f.IsTrained || len(f.Trees) == 0 {
		return Prediction{Action: 0, Confidence: 0}
	}

	// Accumulate class probabilities across all trees
	classProbs := make([]float64, f.NumClasses)
	for _, tree := range f.Trees {
		leafVal := tree.Predict(features)
		classIdx := int(leafVal * float32(f.NumClasses))
		if classIdx >= f.NumClasses {
			classIdx = f.NumClasses - 1
		}
		if classIdx < 0 {
			classIdx = 0
		}
		classProbs[classIdx] += 1.0
	}

	// Normalize
	total := float64(len(f.Trees))
	bestClass := 0
	bestProb := 0.0
	for i, p := range classProbs {
		classProbs[i] = p / total
		if classProbs[i] > bestProb {
			bestProb = classProbs[i]
			bestClass = i
		}
	}

	// Compute anomaly score (distance from uniform distribution)
	uniform := 1.0 / float64(f.NumClasses)
	anomalyDist := 0.0
	for _, p := range classProbs {
		anomalyDist += math.Abs(p - uniform)
	}
	anomalyScore := anomalyDist / (2.0 * (1.0 - uniform))

	return Prediction{
		Action:       int32(bestClass),
		Confidence:   bestProb,
		AnomalyScore: anomalyScore,
	}
}

// Serialize saves the forest to a binary file
func (f *DecisionForest) Serialize(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write header
	header := struct {
		Magic      [4]byte
		Version    uint32
		NumTrees   uint32
		NumClasses uint32
		MaxDepth   uint32
	}{
		Magic:      [4]byte{'R', 'F', 'O', 'R'},
		Version:    1,
		NumTrees:   uint32(len(f.Trees)),
		NumClasses: uint32(f.NumClasses),
		MaxDepth:   uint32(f.MaxDepth),
	}
	if err := binary.Write(file, binary.LittleEndian, &header); err != nil {
		return err
	}

	// Write trees
	for _, tree := range f.Trees {
		numNodes := uint32(len(tree.Nodes))
		if err := binary.Write(file, binary.LittleEndian, numNodes); err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if err := binary.Write(file, binary.LittleEndian, &node); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeserializeForest loads a forest from a binary file
func DeserializeForest(path string) (*DecisionForest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var header struct {
		Magic      [4]byte
		Version    uint32
		NumTrees   uint32
		NumClasses uint32
		MaxDepth   uint32
	}
	if err := binary.Read(file, binary.LittleEndian, &header); err != nil {
		return nil, err
	}
	if header.Magic != [4]byte{'R', 'F', 'O', 'R'} {
		return nil, fmt.Errorf("invalid forest file magic: %v", header.Magic)
	}

	forest := &DecisionForest{
		Trees:       make([]DecisionTree, header.NumTrees),
		NumClasses:  int(header.NumClasses),
		MaxDepth:    int(header.MaxDepth),
		NumFeatures: FeatureDim,
		IsTrained:   true,
	}

	for i := uint32(0); i < header.NumTrees; i++ {
		var numNodes uint32
		if err := binary.Read(file, binary.LittleEndian, &numNodes); err != nil {
			return nil, err
		}
		forest.Trees[i].Nodes = make([]DecisionNode, numNodes)
		for j := uint32(0); j < numNodes; j++ {
			if err := binary.Read(file, binary.LittleEndian, &forest.Trees[i].Nodes[j]); err != nil {
				return nil, err
			}
		}
	}
	return forest, nil
}

func init() {
	RegisterModel("random_forest", func() Model { return NewDecisionForest(31, 8, 4) })
}
