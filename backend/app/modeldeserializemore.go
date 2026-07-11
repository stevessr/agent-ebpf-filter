package app

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// ---- moved from backend/zz_merged_backend.go section modeldeserializemore.go ----

type mlBinaryModelReader struct {
	raw []byte
	pos int
	err error
}

const (
	mlBinaryModelVersion       = 1
	mlBinaryMaxClasses         = 4
	mlBinaryMaxEstimators      = 4096
	mlBinaryMaxModelFileBytes  = 16 << 20
	mlBinaryFloatBytes         = 8
	mlBinaryAdaEstimatorBytes  = 4 + 4*mlBinaryFloatBytes
	mlBinaryNaiveBayesRowBytes = mlBinaryFloatBytes + FeatureDim*2*mlBinaryFloatBytes
	mlBinaryLinearWeightsBytes = (FeatureDim + 1) * mlBinaryFloatBytes
)

func newMLBinaryModelReader(path string, magic string) (*mlBinaryModelReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, mlBinaryMaxModelFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > mlBinaryMaxModelFileBytes {
		return nil, fmt.Errorf("model file exceeds %d bytes", mlBinaryMaxModelFileBytes)
	}
	if len(raw) < 8 || string(raw[:4]) != magic {
		return nil, fmt.Errorf("invalid %s model", magic)
	}
	return &mlBinaryModelReader{raw: raw, pos: 4}, nil
}

func (r *mlBinaryModelReader) readU32() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos+4 > len(r.raw) {
		r.err = fmt.Errorf("unexpected EOF at byte %d", r.pos)
		return 0
	}
	v := binary.LittleEndian.Uint32(r.raw[r.pos:])
	r.pos += 4
	return v
}

func (r *mlBinaryModelReader) readF64() float64 {
	if r.err != nil {
		return 0
	}
	if r.pos+8 > len(r.raw) {
		r.err = fmt.Errorf("unexpected EOF at byte %d", r.pos)
		return 0
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(r.raw[r.pos:]))
	r.pos += 8
	return v
}

func (r *mlBinaryModelReader) readVersion() {
	version := r.readU32()
	if r.err == nil && version != mlBinaryModelVersion {
		r.err = fmt.Errorf("unsupported model version %d", version)
	}
}

func (r *mlBinaryModelReader) readBoundedCount(name string, minValue, maxValue int) int {
	value := uint64(r.readU32())
	if r.err != nil {
		return 0
	}
	if value < uint64(minValue) || value > uint64(maxValue) {
		r.err = fmt.Errorf("invalid %s %d (expected %d..%d)", name, value, minValue, maxValue)
		return 0
	}
	return int(value)
}

func (r *mlBinaryModelReader) requireItems(name string, count, bytesPerItem, extraBytes int) {
	if r.err != nil {
		return
	}
	remaining := len(r.raw) - r.pos
	if count < 0 || bytesPerItem < 0 || extraBytes < 0 || extraBytes > remaining || (bytesPerItem > 0 && count > (remaining-extraBytes)/bytesPerItem) {
		r.err = fmt.Errorf("invalid %s payload size at byte %d", name, r.pos)
	}
}

func (r *mlBinaryModelReader) doneIfInvalid() error { return r.err }

func (r *mlBinaryModelReader) done() error {
	if r.err != nil {
		return r.err
	}
	if r.pos != len(r.raw) {
		return fmt.Errorf("unexpected trailing model data at byte %d", r.pos)
	}
	return nil
}

func DeserializeAdaBoost(path string) (*AdaBoostModel, error) {
	r, err := newMLBinaryModelReader(path, "ADAB")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	nStumps := r.readBoundedCount("AdaBoost estimator count", 0, mlBinaryMaxEstimators)
	classes := r.readBoundedCount("AdaBoost class count", 1, mlBinaryMaxClasses)
	r.requireItems("AdaBoost", nStumps, mlBinaryAdaEstimatorBytes, 0)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &AdaBoostModel{Stumps: make([]adaboostStump, nStumps), Alphas: make([]float64, nStumps), NEst: nStumps, Classes: classes}
	for i := 0; i < nStumps; i++ {
		m.Stumps[i] = adaboostStump{Feature: r.readBoundedCount("AdaBoost feature index", 0, FeatureDim-1), Threshold: r.readF64(), LeftVote: r.readF64(), RightVote: r.readF64()}
	}
	for i := 0; i < nStumps; i++ {
		m.Alphas[i] = r.readF64()
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}

func DeserializeSVM(path string) (*SVMModel, error) {
	r, err := newMLBinaryModelReader(path, "SVM0")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	classes := r.readBoundedCount("SVM class count", 1, mlBinaryMaxClasses)
	r.requireItems("SVM", classes, mlBinaryLinearWeightsBytes, 16)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &SVMModel{Classes: classes, LR: r.readF64(), C: r.readF64(), Weights: make([][FeatureDim + 1]float64, classes)}
	for c := 0; c < classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			m.Weights[c][d] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}

func DeserializeRidge(path string) (*RidgeModel, error) {
	r, err := newMLBinaryModelReader(path, "RIDG")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	classes := r.readBoundedCount("Ridge class count", 1, mlBinaryMaxClasses)
	r.requireItems("Ridge", classes, mlBinaryLinearWeightsBytes, 8)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &RidgeModel{Classes: classes, Alpha: r.readF64(), Weights: make([][FeatureDim + 1]float64, classes)}
	for c := 0; c < classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			m.Weights[c][d] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}

func DeserializePerceptron(path string) (*PerceptronModel, error) {
	r, err := newMLBinaryModelReader(path, "PERC")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	classes := r.readBoundedCount("Perceptron class count", 1, mlBinaryMaxClasses)
	r.requireItems("Perceptron", classes, mlBinaryLinearWeightsBytes, 8)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &PerceptronModel{Classes: classes, LR: r.readF64(), Weights: make([][FeatureDim + 1]float64, classes)}
	for c := 0; c < classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			m.Weights[c][d] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}

func DeserializePA(path string) (*PAModel, error) {
	r, err := newMLBinaryModelReader(path, "PASG")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	classes := r.readBoundedCount("passive-aggressive class count", 1, mlBinaryMaxClasses)
	r.requireItems("passive-aggressive", classes, mlBinaryLinearWeightsBytes, 8)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := &PAModel{Classes: classes, C: r.readF64(), Weights: make([][FeatureDim + 1]float64, classes)}
	for c := 0; c < classes; c++ {
		for d := 0; d <= FeatureDim; d++ {
			m.Weights[c][d] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}
