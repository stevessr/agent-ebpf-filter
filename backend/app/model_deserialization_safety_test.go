package app

import (
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

type binaryModelSafetyCase struct {
	name        string
	serialize   func(string) error
	deserialize func(string) error
	classOffset int
}

func binaryModelSafetyCases() []binaryModelSafetyCase {
	naiveBayes := NewNaiveBayes()
	naiveBayes.Means = make([][FeatureDim]float64, naiveBayes.Classes)
	naiveBayes.Vars = make([][FeatureDim]float64, naiveBayes.Classes)
	naiveBayes.Priors = make([]float64, naiveBayes.Classes)

	adaBoost := NewAdaBoost(2)
	adaBoost.Stumps = make([]adaboostStump, 2)
	adaBoost.Alphas = make([]float64, 2)

	svm := NewSVMModel(0.01, 10)
	svm.Weights = make([][FeatureDim + 1]float64, svm.Classes)
	ridge := NewRidgeModel(1)
	ridge.Weights = make([][FeatureDim + 1]float64, ridge.Classes)
	perceptron := NewPerceptron(0.01, 10)
	perceptron.Weights = make([][FeatureDim + 1]float64, perceptron.Classes)
	pa := NewPAModel(1, 10)
	pa.Weights = make([][FeatureDim + 1]float64, pa.Classes)

	return []binaryModelSafetyCase{
		{name: "naive_bayes", serialize: naiveBayes.Serialize, deserialize: func(path string) error { _, err := DeserializeNaiveBayes(path); return err }, classOffset: 8},
		{name: "adaboost", serialize: adaBoost.Serialize, deserialize: func(path string) error { _, err := DeserializeAdaBoost(path); return err }, classOffset: 12},
		{name: "svm", serialize: svm.Serialize, deserialize: func(path string) error { _, err := DeserializeSVM(path); return err }, classOffset: 8},
		{name: "ridge", serialize: ridge.Serialize, deserialize: func(path string) error { _, err := DeserializeRidge(path); return err }, classOffset: 8},
		{name: "perceptron", serialize: perceptron.Serialize, deserialize: func(path string) error { _, err := DeserializePerceptron(path); return err }, classOffset: 8},
		{name: "passive_aggressive", serialize: pa.Serialize, deserialize: func(path string) error { _, err := DeserializePA(path); return err }, classOffset: 8},
	}
}

func TestBinaryModelDeserializersRejectCorruptHeadersAndCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range binaryModelSafetyCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "model.bin")
			if err := tc.serialize(path); err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			invalidVersion := append([]byte(nil), valid...)
			binary.LittleEndian.PutUint32(invalidVersion[4:8], mlBinaryModelVersion+1)
			assertBinaryModelRejected(t, path, invalidVersion, tc.deserialize)

			invalidClasses := append([]byte(nil), valid...)
			binary.LittleEndian.PutUint32(invalidClasses[tc.classOffset:tc.classOffset+4], ^uint32(0))
			assertBinaryModelRejected(t, path, invalidClasses, tc.deserialize)

			withTrailingData := append(append([]byte(nil), valid...), 0xff)
			assertBinaryModelRejected(t, path, withTrailingData, tc.deserialize)
		})
	}
}

func TestAdaBoostDeserializerRejectsOversizedEstimatorCount(t *testing.T) {
	t.Parallel()

	tc := binaryModelSafetyCases()[1]
	path := filepath.Join(t.TempDir(), "adaboost.bin")
	if err := tc.serialize(path); err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	binary.LittleEndian.PutUint32(payload[8:12], mlBinaryMaxEstimators+1)
	assertBinaryModelRejected(t, path, payload, tc.deserialize)

	if err := tc.serialize(path); err != nil {
		t.Fatalf("Serialize() second error = %v", err)
	}
	payload, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() second error = %v", err)
	}
	binary.LittleEndian.PutUint32(payload[16:20], FeatureDim)
	assertBinaryModelRejected(t, path, payload, tc.deserialize)
}

func TestBinaryModelDeserializersRejectRandomTruncationsWithoutPanicking(t *testing.T) {
	t.Parallel()

	for caseIndex, tc := range binaryModelSafetyCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "model.bin")
			if err := tc.serialize(path); err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if err := tc.deserialize(path); err != nil {
				t.Fatalf("Deserialize(valid) error = %v", err)
			}

			cuts := map[int]struct{}{0: {}, 1: {}, 4: {}, 7: {}, 8: {}, len(valid) - 1: {}}
			rng := rand.New(rand.NewSource(int64(1000 + caseIndex)))
			for i := 0; i < 128; i++ {
				cuts[rng.Intn(len(valid))] = struct{}{}
			}
			for cut := range cuts {
				assertBinaryModelRejected(t, path, valid[:cut], tc.deserialize)
			}
		})
	}
}

func TestBinaryModelDeserializerRejectsOversizedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "oversized.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Truncate(mlBinaryMaxModelFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertBinaryModelRejected(t, path, nil, func(path string) error {
		_, err := newMLBinaryModelReader(path, "NBAY")
		return err
	})
}

func assertBinaryModelRejected(t *testing.T, path string, payload []byte, deserialize func(string) error) {
	t.Helper()
	if payload != nil {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("deserializer panicked: %v", recovered)
		}
	}()
	if err := deserialize(path); err == nil {
		t.Fatal("deserializer accepted malformed model")
	}
}
