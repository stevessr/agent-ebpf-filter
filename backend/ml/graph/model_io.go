package graph

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

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
