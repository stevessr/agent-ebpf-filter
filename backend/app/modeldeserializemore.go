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
	mlBinaryMaxTreeNodes       = 1<<15 - 1
	mlBinaryMaxModelFileBytes  = 16 << 20
	mlKNNMaxModelFileBytes     = 128 << 20
	mlJSONMaxModelFileBytes    = 16 << 20
	mlEnsembleManifestMaxBytes = 256 << 10
	mlMaxEnsembleMembers       = 64
	mlMaxTrainingSamples       = 100000
	mlBinaryFloatBytes         = 8
	mlBinaryAdaEstimatorBytes  = 4 + 4*mlBinaryFloatBytes
	mlBinaryNaiveBayesRowBytes = mlBinaryFloatBytes + FeatureDim*2*mlBinaryFloatBytes
	mlBinaryLinearWeightsBytes = (FeatureDim + 1) * mlBinaryFloatBytes
)

func readBoundedMLModelFile(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("invalid model file limit %d", maxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("model file exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func newMLBinaryModelReader(path string, magic string) (*mlBinaryModelReader, error) {
	return newMLBinaryModelReaderWithLimit(path, magic, mlBinaryMaxModelFileBytes)
}

func newMLBinaryModelReaderWithLimit(path string, magic string, maxBytes int64) (*mlBinaryModelReader, error) {
	raw, err := readBoundedMLModelFile(path, maxBytes)
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

func (r *mlBinaryModelReader) readU8() uint8 {
	if r.err != nil {
		return 0
	}
	if r.pos+1 > len(r.raw) {
		r.err = fmt.Errorf("unexpected EOF at byte %d", r.pos)
		return 0
	}
	v := r.raw[r.pos]
	r.pos++
	return v
}

func (r *mlBinaryModelReader) readU16() uint16 {
	if r.err != nil {
		return 0
	}
	if r.pos+2 > len(r.raw) {
		r.err = fmt.Errorf("unexpected EOF at byte %d", r.pos)
		return 0
	}
	v := binary.LittleEndian.Uint16(r.raw[r.pos:])
	r.pos += 2
	return v
}

func (r *mlBinaryModelReader) readF32() float32 {
	value := math.Float32frombits(r.readU32())
	if r.err == nil && (math.IsNaN(float64(value)) || math.IsInf(float64(value), 0)) {
		r.err = fmt.Errorf("invalid non-finite float at byte %d", r.pos-4)
		return 0
	}
	return value
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
	if math.IsNaN(v) || math.IsInf(v, 0) {
		r.err = fmt.Errorf("invalid non-finite float at byte %d", r.pos-8)
		return 0
	}
	return v
}

func (r *mlBinaryModelReader) readVersion() {
	version := r.readU32()
	if r.err == nil && version != mlBinaryModelVersion {
		r.err = fmt.Errorf("unsupported model version %d", version)
	}
}

func (r *mlBinaryModelReader) readSupportedVersion(name string, versions ...uint32) uint32 {
	version := r.readU32()
	if r.err != nil {
		return 0
	}
	for _, supported := range versions {
		if version == supported {
			return version
		}
	}
	r.err = fmt.Errorf("unsupported %s version %d", name, version)
	return 0
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

func (r *mlBinaryModelReader) readBytes(name string, count int) []byte {
	if r.err != nil {
		return nil
	}
	if count < 0 || count > len(r.raw)-r.pos {
		r.err = fmt.Errorf("invalid %s length %d at byte %d", name, count, r.pos)
		return nil
	}
	value := r.raw[r.pos : r.pos+count]
	r.pos += count
	return value
}

func (r *mlBinaryModelReader) readBoundedString(name string, maxBytes int) string {
	length := r.readBoundedCount(name+" length", 0, maxBytes)
	return string(r.readBytes(name, length))
}

func (r *mlBinaryModelReader) readBoolU32(name string) bool {
	value := r.readU32()
	if r.err != nil {
		return false
	}
	if value > 1 {
		r.err = fmt.Errorf("invalid %s value %d", name, value)
		return false
	}
	return value == 1
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
