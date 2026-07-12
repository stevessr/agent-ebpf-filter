package graph

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
)

const (
	maxGNNModelFileBytes = 32 << 20
	maxGNNHiddenDim      = 256
	maxGNNLayers         = 64
	maxGNNTrainingSteps  = 100000
)

// GNNConfig holds configuration for the GNN model.
type GNNConfig struct {
	HiddenDim    int     // hidden dimension for GNN layers (default 32)
	NumLayers    int     // number of message passing layers (default 2)
	DropoutRate  float64 // dropout rate (default 0.1)
	LearningRate float64 // learning rate for training (default 0.005)
	L2Lambda     float64 // L2 regularization strength (default 0.001)
	Epochs       int     // number of training epochs (default 100)
	BatchSize    int     // mini-batch size (default 32)
	Patience     int     // early stopping patience (default 15)
	UseSAGE      bool    // use GraphSAGE layers instead of GAT (default false)
}

// DefaultGNNConfig returns sensible defaults.
func DefaultGNNConfig() GNNConfig {
	return GNNConfig{
		HiddenDim:    32,
		NumLayers:    2,
		DropoutRate:  0.1,
		LearningRate: 0.005,
		L2Lambda:     0.001,
		Epochs:       100,
		BatchSize:    32,
		Patience:     15,
		UseSAGE:      false,
	}
}

// GNNClassifier is the complete Graph Neural Network model for
// classifying security operations.
type GNNClassifier struct {
	Config      GNNConfig
	Groups      []FeatureGroup
	Edges       []EdgeDef
	Projections []*DenseLayer
	GATLayers   []*GATLayer
	SAGELayers  []*SAGELayer
	ReadoutFC1  *DenseLayer
	ReadoutFC2  *DenseLayer
	Classifier  *DenseLayer
	IsTrained   bool
	Rng         *rand.Rand
	LastGraph   *GraphInstance
}

// NewGNNClassifier creates a new GNN classifier.
func NewGNNClassifier(cfg GNNConfig) *GNNClassifier {
	rng := rand.New(rand.NewSource(42))
	groups := DefaultFeatureGroups()

	projections := make([]*DenseLayer, len(groups))
	for i, g := range groups {
		projections[i] = NewDenseLayer(g.Dim, cfg.HiddenDim, ActivationLeakyReLU, rng)
	}

	var gatLayers []*GATLayer
	var sageLayers []*SAGELayer

	for l := 0; l < cfg.NumLayers; l++ {
		if cfg.UseSAGE {
			sageLayers = append(sageLayers, NewSAGELayer(cfg.HiddenDim, cfg.HiddenDim, rng))
		} else {
			gatLayers = append(gatLayers, NewGATLayer(cfg.HiddenDim, cfg.HiddenDim, rng))
		}
	}

	readoutIn := cfg.HiddenDim * len(groups)
	readoutFC1 := NewDenseLayer(readoutIn, cfg.HiddenDim*2, ActivationLeakyReLU, rng)
	readoutFC2 := NewDenseLayer(cfg.HiddenDim*2, cfg.HiddenDim, ActivationLeakyReLU, rng)
	classifier := NewDenseLayer(cfg.HiddenDim, NumClasses, ActivationNone, rng)

	return &GNNClassifier{
		Config:      cfg,
		Groups:      groups,
		Edges:       DefaultEdges(),
		Projections: projections,
		GATLayers:   gatLayers,
		SAGELayers:  sageLayers,
		ReadoutFC1:  readoutFC1,
		ReadoutFC2:  readoutFC2,
		Classifier:  classifier,
		Rng:         rng,
	}
}

// BuildGraph constructs a feature interaction graph from a 128-dim feature vector.
func (m *GNNClassifier) BuildGraph(features []float64) *GraphInstance {
	n := len(m.Groups)
	g := &GraphInstance{
		Nodes:    make([]NodeState, n),
		AdjList:  make([][]int, n),
		NumNodes: n,
	}

	for i, grp := range m.Groups {
		start := grp.Start
		end := grp.End
		if end > len(features) {
			end = len(features)
		}
		g.Nodes[i].Features = make([]float64, grp.Dim)
		if start < end {
			copy(g.Nodes[i].Features, features[start:end])
		}
	}

	for _, e := range m.Edges {
		if e.Source < n && e.Target < n {
			g.AdjList[e.Source] = append(g.AdjList[e.Source], e.Target)
		}
	}

	return g
}

// Forward performs a complete forward pass: features → graph → message passing → readout → logits.
func (m *GNNClassifier) Forward(features []float64) []float64 {
	g := m.BuildGraph(features)
	m.LastGraph = g

	for i := range g.Nodes {
		g.Nodes[i].Embedding = m.Projections[i].Forward(g.Nodes[i].Features)
	}

	if m.Config.UseSAGE {
		for _, layer := range m.SAGELayers {
			newEmb := layer.Forward(g)
			for i := range g.Nodes {
				for d := 0; d < len(newEmb[i]) && d < len(g.Nodes[i].Embedding); d++ {
					g.Nodes[i].Embedding[d] = g.Nodes[i].Embedding[d]*0.5 + newEmb[i][d]*0.5
				}
			}
		}
	} else {
		for _, layer := range m.GATLayers {
			newEmb := layer.Forward(g)
			for i := range g.Nodes {
				for d := 0; d < len(newEmb[i]) && d < len(g.Nodes[i].Embedding); d++ {
					g.Nodes[i].Embedding[d] = g.Nodes[i].Embedding[d]*0.5 + newEmb[i][d]*0.5
				}
			}
		}
	}

	hiddenDim := m.Config.HiddenDim
	graphEmb := make([]float64, hiddenDim*len(m.Groups))
	for i, node := range g.Nodes {
		offset := i * hiddenDim
		for d := 0; d < hiddenDim && d < len(node.Embedding); d++ {
			graphEmb[offset+d] = node.Embedding[d]
		}
	}

	h1 := m.ReadoutFC1.Forward(graphEmb)
	h2 := m.ReadoutFC2.Forward(h1)
	logits := m.Classifier.Forward(h2)

	return logits
}

// PredictProbs runs forward pass and returns class probabilities via softmax.
func (m *GNNClassifier) PredictProbs(features []float64) []float64 {
	logits := m.Forward(features)
	return softmax(logits)
}

// PredictClass returns the predicted class, confidence, and anomaly score.
func (m *GNNClassifier) PredictClass(features []float64) (int, float64, float64) {
	probs := m.PredictProbs(features)

	bestClass := 0
	bestProb := 0.0
	for i, p := range probs {
		if p > bestProb {
			bestProb = p
			bestClass = i
		}
	}

	uniform := 1.0 / float64(len(probs))
	anomalyDist := 0.0
	for _, p := range probs {
		anomalyDist += math.Abs(p - uniform)
	}
	anomalyScore := anomalyDist / (2.0 * (1.0 - uniform))

	return bestClass, bestProb, anomalyScore
}

// Backward performs backpropagation for the entire GNN model given the gradient of loss w.r.t. logits.
func (m *GNNClassifier) Backward(gradLogits []float64) {
	gradH2 := m.Classifier.Backward(gradLogits)
	gradH1 := m.ReadoutFC2.Backward(gradH2)
	gradGraphEmb := m.ReadoutFC1.Backward(gradH1)

	n := len(m.Groups)
	hiddenDim := m.Config.HiddenDim
	gradEmbeddings := make([][]float64, n)
	for i := 0; i < n; i++ {
		gradEmbeddings[i] = make([]float64, hiddenDim)
		offset := i * hiddenDim
		for d := 0; d < hiddenDim; d++ {
			gradEmbeddings[i][d] = gradGraphEmb[offset+d]
		}
	}

	if m.Config.UseSAGE {
		for l := len(m.SAGELayers) - 1; l >= 0; l-- {
			layer := m.SAGELayers[l]
			layerGradOut := make([][]float64, n)
			for i := 0; i < n; i++ {
				layerGradOut[i] = make([]float64, hiddenDim)
				for d := 0; d < hiddenDim; d++ {
					layerGradOut[i][d] = gradEmbeddings[i][d] * 0.5
					gradEmbeddings[i][d] = gradEmbeddings[i][d] * 0.5
				}
			}
			dPrevEmb := layer.Backward(m.LastGraph, layerGradOut)
			for i := 0; i < n; i++ {
				for d := 0; d < hiddenDim; d++ {
					gradEmbeddings[i][d] += dPrevEmb[i][d]
				}
			}
		}
	} else {
		for l := len(m.GATLayers) - 1; l >= 0; l-- {
			layer := m.GATLayers[l]
			layerGradOut := make([][]float64, n)
			for i := 0; i < n; i++ {
				layerGradOut[i] = make([]float64, hiddenDim)
				for d := 0; d < hiddenDim; d++ {
					layerGradOut[i][d] = gradEmbeddings[i][d] * 0.5
					gradEmbeddings[i][d] = gradEmbeddings[i][d] * 0.5
				}
			}
			dPrevEmb := layer.Backward(m.LastGraph, layerGradOut)
			for i := 0; i < n; i++ {
				for d := 0; d < hiddenDim; d++ {
					gradEmbeddings[i][d] += dPrevEmb[i][d]
				}
			}
		}
	}

	for i := 0; i < n; i++ {
		m.Projections[i].Backward(gradEmbeddings[i])
	}
}

// Train trains the GNN classifier on a dataset.
func (m *GNNClassifier) Train(features [][]float64, labels []int, epochs int, batchSize int, progressCallback func(float64)) float64 {
	m.IsTrained = true
	n := len(features)
	if n == 0 {
		return 0
	}

	var finalLoss float64
	for epoch := 0; epoch < epochs; epoch++ {
		epochLoss := 0.0
		indices := rand.Perm(n)

		for i := 0; i < n; i += batchSize {
			end := i + batchSize
			if end > n {
				end = n
			}
			actualBatchSize := end - i

			m.ZeroGrad()

			for idx := i; idx < end; idx++ {
				feat := features[indices[idx]]
				lbl := labels[indices[idx]]

				probs := m.PredictProbs(feat)

				lossVal := 0.0
				if probs[lbl] > 1e-15 {
					lossVal = -math.Log(probs[lbl])
				} else {
					lossVal = 35.0
				}
				epochLoss += lossVal

				gradLogits := make([]float64, NumClasses)
				for c := 0; c < NumClasses; c++ {
					if c == lbl {
						gradLogits[c] = probs[c] - 1.0
					} else {
						gradLogits[c] = probs[c]
					}
				}

				m.Backward(gradLogits)
			}

			m.ApplyGradients(actualBatchSize)
		}

		finalLoss = epochLoss / float64(n)
		if progressCallback != nil {
			progressCallback(float64(epoch+1) / float64(epochs))
		}
	}

	return finalLoss
}

// ZeroGrad resets all gradient accumulators in the model.
func (m *GNNClassifier) ZeroGrad() {
	for _, p := range m.Projections {
		p.ZeroGrad()
	}
	for _, l := range m.GATLayers {
		l.ZeroGrad()
	}
	for _, l := range m.SAGELayers {
		l.ZeroGrad()
	}
	m.ReadoutFC1.ZeroGrad()
	m.ReadoutFC2.ZeroGrad()
	m.Classifier.ZeroGrad()
}

// ApplyGradients updates all weights in the model.
func (m *GNNClassifier) ApplyGradients(batchSize int) {
	lr := m.Config.LearningRate
	l2 := m.Config.L2Lambda

	for _, p := range m.Projections {
		p.ApplyGradients(lr, l2, batchSize)
	}
	for _, l := range m.GATLayers {
		l.ApplyGradients(lr, l2, batchSize)
	}
	for _, l := range m.SAGELayers {
		l.ApplyGradients(lr, l2, batchSize)
	}
	m.ReadoutFC1.ApplyGradients(lr, l2, batchSize)
	m.ReadoutFC2.ApplyGradients(lr, l2, batchSize)
	m.Classifier.ApplyGradients(lr, l2, batchSize)
}

// ── Serialization Helpers ───────────────────────────────────────────────────

type denseLayerSave struct {
	Weights [][]float64
	Bias    []float64
}

type gatLayerSave struct {
	W         denseLayerSave
	AttnLeft  []float64
	AttnRight []float64
}

type sageLayerSave struct {
	WSelf     denseLayerSave
	WNeighbor denseLayerSave
}

type gnnModelSave struct {
	Config      GNNConfig
	IsTrained   bool
	Projections []denseLayerSave
	GATLayers   []gatLayerSave
	SAGELayers  []sageLayerSave
	ReadoutFC1  denseLayerSave
	ReadoutFC2  denseLayerSave
	Classifier  denseLayerSave
}

func (l *DenseLayer) toSave() denseLayerSave {
	return denseLayerSave{
		Weights: l.Weights,
		Bias:    l.Bias,
	}
}

func (l *DenseLayer) fromSave(save denseLayerSave) {
	l.Weights = save.Weights
	l.Bias = save.Bias
}

func (m *GNNClassifier) toSave() gnnModelSave {
	save := gnnModelSave{
		Config:     m.Config,
		IsTrained:  m.IsTrained,
		ReadoutFC1: m.ReadoutFC1.toSave(),
		ReadoutFC2: m.ReadoutFC2.toSave(),
		Classifier: m.Classifier.toSave(),
	}
	save.Projections = make([]denseLayerSave, len(m.Projections))
	for i, p := range m.Projections {
		save.Projections[i] = p.toSave()
	}
	save.GATLayers = make([]gatLayerSave, len(m.GATLayers))
	for i, l := range m.GATLayers {
		save.GATLayers[i] = gatLayerSave{
			W:         l.W.toSave(),
			AttnLeft:  l.AttnLeft,
			AttnRight: l.AttnRight,
		}
	}
	save.SAGELayers = make([]sageLayerSave, len(m.SAGELayers))
	for i, l := range m.SAGELayers {
		save.SAGELayers[i] = sageLayerSave{
			WSelf:     l.WSelf.toSave(),
			WNeighbor: l.WNeighbor.toSave(),
		}
	}
	return save
}

func (m *GNNClassifier) fromSave(save gnnModelSave) {
	m.Config = save.Config
	m.IsTrained = save.IsTrained
	m.ReadoutFC1.fromSave(save.ReadoutFC1)
	m.ReadoutFC2.fromSave(save.ReadoutFC2)
	m.Classifier.fromSave(save.Classifier)

	for i, p := range m.Projections {
		if i < len(save.Projections) {
			p.fromSave(save.Projections[i])
		}
	}
	for i, l := range m.GATLayers {
		if i < len(save.GATLayers) {
			l.W.fromSave(save.GATLayers[i].W)
			l.AttnLeft = save.GATLayers[i].AttnLeft
			l.AttnRight = save.GATLayers[i].AttnRight
		}
	}
	for i, l := range m.SAGELayers {
		if i < len(save.SAGELayers) {
			l.WSelf.fromSave(save.SAGELayers[i].WSelf)
			l.WNeighbor.fromSave(save.SAGELayers[i].WNeighbor)
		}
	}
}

func (m *GNNClassifier) Serialize(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	save := m.toSave()
	return encoder.Encode(&save)
}

func DeserializeGNNClassifier(path string) (*GNNClassifier, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxGNNModelFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxGNNModelFileBytes {
		return nil, fmt.Errorf("GNN model file exceeds %d bytes", maxGNNModelFileBytes)
	}
	decoder := gob.NewDecoder(bytes.NewReader(raw))
	var save gnnModelSave
	if err := decoder.Decode(&save); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("GNN model file contains trailing data")
		}
		return nil, fmt.Errorf("invalid trailing GNN model data: %w", err)
	}
	if err := validateGNNModelSave(save); err != nil {
		return nil, fmt.Errorf("invalid GNN model: %w", err)
	}

	model := NewGNNClassifier(save.Config)
	model.fromSave(save)
	return model, nil
}

func validateGNNModelSave(save gnnModelSave) error {
	cfg := save.Config
	if cfg.HiddenDim < 1 || cfg.HiddenDim > maxGNNHiddenDim {
		return fmt.Errorf("hidden dimension %d", cfg.HiddenDim)
	}
	if cfg.NumLayers < 1 || cfg.NumLayers > maxGNNLayers {
		return fmt.Errorf("layer count %d", cfg.NumLayers)
	}
	if !finiteGNN(cfg.DropoutRate) || cfg.DropoutRate < 0 || cfg.DropoutRate >= 1 {
		return fmt.Errorf("dropout rate %v", cfg.DropoutRate)
	}
	if !finiteGNN(cfg.LearningRate) || cfg.LearningRate <= 0 || cfg.LearningRate > 1 {
		return fmt.Errorf("learning rate %v", cfg.LearningRate)
	}
	if !finiteGNN(cfg.L2Lambda) || cfg.L2Lambda < 0 || cfg.L2Lambda > 1 {
		return fmt.Errorf("L2 lambda %v", cfg.L2Lambda)
	}
	if cfg.Epochs < 1 || cfg.Epochs > maxGNNTrainingSteps {
		return fmt.Errorf("epoch count %d", cfg.Epochs)
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > maxGNNTrainingSteps {
		return fmt.Errorf("batch size %d", cfg.BatchSize)
	}
	if cfg.Patience < 1 || cfg.Patience > maxGNNTrainingSteps {
		return fmt.Errorf("patience %d", cfg.Patience)
	}

	groups := DefaultFeatureGroups()
	if len(save.Projections) != len(groups) {
		return fmt.Errorf("projection count %d", len(save.Projections))
	}
	for index, projection := range save.Projections {
		if err := validateDenseLayerSave(projection, groups[index].Dim, cfg.HiddenDim); err != nil {
			return fmt.Errorf("projection %d: %w", index, err)
		}
	}
	if cfg.UseSAGE {
		if len(save.GATLayers) != 0 || len(save.SAGELayers) != cfg.NumLayers {
			return fmt.Errorf("message-passing layer count")
		}
		for index, layer := range save.SAGELayers {
			if err := validateDenseLayerSave(layer.WSelf, cfg.HiddenDim, cfg.HiddenDim); err != nil {
				return fmt.Errorf("SAGE layer %d self weights: %w", index, err)
			}
			if err := validateDenseLayerSave(layer.WNeighbor, cfg.HiddenDim, cfg.HiddenDim); err != nil {
				return fmt.Errorf("SAGE layer %d neighbor weights: %w", index, err)
			}
		}
	} else {
		if len(save.SAGELayers) != 0 || len(save.GATLayers) != cfg.NumLayers {
			return fmt.Errorf("message-passing layer count")
		}
		for index, layer := range save.GATLayers {
			if err := validateDenseLayerSave(layer.W, cfg.HiddenDim, cfg.HiddenDim); err != nil {
				return fmt.Errorf("GAT layer %d weights: %w", index, err)
			}
			if len(layer.AttnLeft) != cfg.HiddenDim || len(layer.AttnRight) != cfg.HiddenDim ||
				!finiteGNNSlice(layer.AttnLeft) || !finiteGNNSlice(layer.AttnRight) {
				return fmt.Errorf("GAT layer %d attention shape or value", index)
			}
		}
	}
	readoutInput := cfg.HiddenDim * len(groups)
	if err := validateDenseLayerSave(save.ReadoutFC1, readoutInput, cfg.HiddenDim*2); err != nil {
		return fmt.Errorf("readout FC1: %w", err)
	}
	if err := validateDenseLayerSave(save.ReadoutFC2, cfg.HiddenDim*2, cfg.HiddenDim); err != nil {
		return fmt.Errorf("readout FC2: %w", err)
	}
	if err := validateDenseLayerSave(save.Classifier, cfg.HiddenDim, NumClasses); err != nil {
		return fmt.Errorf("classifier: %w", err)
	}
	return nil
}

func validateDenseLayerSave(layer denseLayerSave, inputDim, outputDim int) error {
	if len(layer.Weights) != outputDim || len(layer.Bias) != outputDim || !finiteGNNSlice(layer.Bias) {
		return fmt.Errorf("expected %dx%d weights and %d biases", outputDim, inputDim, outputDim)
	}
	for rowIndex, row := range layer.Weights {
		if len(row) != inputDim || !finiteGNNSlice(row) {
			return fmt.Errorf("row %d has invalid shape or value", rowIndex)
		}
	}
	return nil
}

func finiteGNN(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteGNNSlice(values []float64) bool {
	for _, value := range values {
		if !finiteGNN(value) {
			return false
		}
	}
	return true
}
