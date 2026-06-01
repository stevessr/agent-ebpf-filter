package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

type mlBinaryModelReader struct {
	raw []byte
	pos int
	err error
}

func newMLBinaryModelReader(path string, magic string) (*mlBinaryModelReader, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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

func (r *mlBinaryModelReader) done() error { return r.err }

func DeserializeAdaBoost(path string) (*AdaBoostModel, error) {
	r, err := newMLBinaryModelReader(path, "ADAB")
	if err != nil {
		return nil, err
	}
	_ = r.readU32()
	nStumps := int(r.readU32())
	classes := int(r.readU32())
	if classes <= 0 {
		classes = 4
	}
	m := &AdaBoostModel{Stumps: make([]adaboostStump, nStumps), Alphas: make([]float64, nStumps), NEst: nStumps, Classes: classes}
	for i := 0; i < nStumps; i++ {
		m.Stumps[i] = adaboostStump{Feature: int(r.readU32()), Threshold: r.readF64(), LeftVote: r.readF64(), RightVote: r.readF64()}
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
	_ = r.readU32()
	classes := int(r.readU32())
	if classes <= 0 {
		classes = 4
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
	_ = r.readU32()
	classes := int(r.readU32())
	if classes <= 0 {
		classes = 4
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
	_ = r.readU32()
	classes := int(r.readU32())
	if classes <= 0 {
		classes = 4
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
	_ = r.readU32()
	classes := int(r.readU32())
	if classes <= 0 {
		classes = 4
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
