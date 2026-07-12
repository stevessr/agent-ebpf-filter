package app

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// ---- moved from backend/zz_merged_backend.go section decision_forest.go ----

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

// Prediction is the output of the random forest
// Action: ALLOW=0, BLOCK=1, REWRITE=2, ALERT=3
type Prediction struct {
	Action       int32
	Confidence   float64
	AnomalyScore float64
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
func (f *DecisionForest) Type() ModelType { return ModelRandomForest }

// NewDecisionForest creates a new random forest
func NewDecisionForest(numTrees, maxDepth, numClasses int) *DecisionForest {
	return &DecisionForest{
		Trees:       make([]DecisionTree, numTrees),
		NumClasses:  numClasses,
		MaxDepth:    maxDepth,
		NumFeatures: FeatureDim,
	}
}

// Predict returns the ensemble prediction
func (f *DecisionForest) Predict(features [FeatureDim]float64) Prediction {
	if !f.IsTrained || len(f.Trees) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0}
	}

	classVotes := make([]float64, f.NumClasses)
	validVotes := 0.0

	for i := range f.Trees {
		leaf := f.Trees[i].Predict(features)
		cls := int(math.Round(float64(leaf)))
		if cls < 0 || cls >= f.NumClasses {
			continue
		}
		classVotes[cls]++
		validVotes++
	}

	if validVotes == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	// Find class with most votes.
	bestClass := int32(0)
	bestVotes := classVotes[0]
	for i := 1; i < f.NumClasses; i++ {
		if classVotes[i] > bestVotes {
			bestVotes = classVotes[i]
			bestClass = int32(i)
		}
	}

	confidence := bestVotes / validVotes
	anomalyScore := 1.0 - confidence

	return Prediction{
		Action:       bestClass,
		Confidence:   confidence,
		AnomalyScore: anomalyScore,
	}
}

// Serialize writes the forest to a binary file
func (f *DecisionForest) Serialize(path string) error {
	data := make([]byte, 0, 256*1024) // ~250KB typical

	// Header
	header := make([]byte, 17)
	copy(header[0:4], []byte("FORE"))             // magic
	binary.LittleEndian.PutUint32(header[4:8], 1) // version
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(f.Trees)))
	binary.LittleEndian.PutUint32(header[12:16], uint32(f.MaxDepth))
	header[16] = uint8(f.NumFeatures)
	data = append(data, header...)

	// Trees
	for _, tree := range f.Trees {
		nodeCount := make([]byte, 4)
		binary.LittleEndian.PutUint32(nodeCount, uint32(len(tree.Nodes)))
		data = append(data, nodeCount...)

		for _, node := range tree.Nodes {
			nodeData := make([]byte, 13)
			nodeData[0] = node.FeatureIndex
			binary.LittleEndian.PutUint32(nodeData[1:5], math.Float32bits(node.Threshold))
			binary.LittleEndian.PutUint16(nodeData[5:7], uint16(node.LeftChild))
			binary.LittleEndian.PutUint16(nodeData[7:9], uint16(node.RightChild))
			binary.LittleEndian.PutUint32(nodeData[9:13], math.Float32bits(node.LeafValue))
			data = append(data, nodeData...)
		}
	}

	// Atomic write
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// DeserializeForest reads a forest from a binary file
func DeserializeForest(path string) (*DecisionForest, error) {
	r, err := newMLBinaryModelReader(path, "FORE")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	numTrees := r.readBoundedCount("forest tree count", 0, mlBinaryMaxEstimators)
	maxDepth := r.readBoundedCount("forest max depth", 0, 64)
	numFeatures := int(r.readU8())
	if numFeatures < 1 || numFeatures > FeatureDim {
		return nil, fmt.Errorf("invalid forest feature count %d", numFeatures)
	}
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}

	forest := &DecisionForest{
		Trees:       make([]DecisionTree, numTrees),
		NumClasses:  4,
		MaxDepth:    maxDepth,
		NumFeatures: numFeatures,
		IsTrained:   true,
	}

	for ti := 0; ti < numTrees; ti++ {
		numNodes := r.readBoundedCount(fmt.Sprintf("forest tree %d node count", ti), 0, mlBinaryMaxTreeNodes)
		r.requireItems(fmt.Sprintf("forest tree %d", ti), numNodes, 13, 0)
		if err := r.doneIfInvalid(); err != nil {
			return nil, err
		}
		nodes := make([]DecisionNode, numNodes)
		for ni := 0; ni < numNodes; ni++ {
			nodes[ni] = DecisionNode{
				FeatureIndex: r.readU8(),
				Threshold:    r.readF32(),
				LeftChild:    int16(r.readU16()),
				RightChild:   int16(r.readU16()),
				LeafValue:    r.readF32(),
			}
		}
		if err := validateDecisionTreeNodes(nodes, numFeatures); err != nil {
			return nil, fmt.Errorf("invalid forest tree %d: %w", ti, err)
		}
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return forest, nil
}

func validateDecisionTreeNodes(nodes []DecisionNode, numFeatures int) error {
	for index := range nodes {
		node := nodes[index]
		if node.IsLeaf() {
			continue
		}
		if int(node.FeatureIndex) >= numFeatures {
			return fmt.Errorf("node %d feature index %d exceeds %d", index, node.FeatureIndex, numFeatures)
		}
		left := int(node.LeftChild)
		right := int(node.RightChild)
		if left <= index || left >= len(nodes) || right <= index || right >= len(nodes) {
			return fmt.Errorf("node %d has invalid children %d,%d", index, left, right)
		}
	}
	return nil
}

// Prune removes underperforming trees from the forest.
// Each tree is evaluated on the provided training samples; trees with accuracy
// more than one standard deviation below the mean are removed.
// Returns the number of trees removed.
func (f *DecisionForest) Prune(samples []trainSample) int {
	if !f.IsTrained || len(f.Trees) < 3 {
		return 0
	}

	treeAcc := make([]float64, len(f.Trees))
	for ti := range f.Trees {
		correct := 0
		for _, s := range samples {
			pred := f.Trees[ti].Predict(s.features)
			if int(math.Round(float64(pred))) == int(s.label) {
				correct++
			}
		}
		treeAcc[ti] = float64(correct) / float64(len(samples))
	}

	mean := 0.0
	for _, acc := range treeAcc {
		mean += acc
	}
	mean /= float64(len(treeAcc))

	variance := 0.0
	for _, acc := range treeAcc {
		d := acc - mean
		variance += d * d
	}
	variance /= float64(len(treeAcc))
	stddev := math.Sqrt(variance)

	threshold := mean - stddev
	if threshold < 0.25 {
		threshold = 0.25
	}

	kept := make([]DecisionTree, 0, len(f.Trees))
	for ti, acc := range treeAcc {
		if acc >= threshold || len(kept) < 2 {
			kept = append(kept, f.Trees[ti])
		}
	}

	removed := len(f.Trees) - len(kept)
	if removed == 0 || len(kept) < 2 {
		return 0
	}

	f.Trees = kept
	return removed
}

// actionLabel maps action integers to strings
var actionLabel = map[int32]string{
	0: "ALLOW",
	1: "BLOCK",
	2: "REWRITE",
	3: "ALERT",
}

func actionFromLabel(label string) int32 {
	switch label {
	case "BLOCK":
		return 1
	case "REWRITE":
		return 2
	case "ALERT":
		return 3
	default:
		return 0 // ALLOW
	}
}
