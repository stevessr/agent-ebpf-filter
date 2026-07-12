package graph

import (
	"encoding/gob"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestGNNModelDeserializeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.gob")
	model := NewGNNClassifier(DefaultGNNConfig())
	model.IsTrained = true
	if err := model.Serialize(path); err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	loaded, err := DeserializeGNNClassifier(path)
	if err != nil {
		t.Fatalf("DeserializeGNNClassifier() error = %v", err)
	}
	if !loaded.IsTrained || loaded.Config != model.Config {
		t.Fatalf("loaded model metadata = %+v, trained=%v", loaded.Config, loaded.IsTrained)
	}
}

func TestGNNModelDeserializerRejectsTruncationAndTrailingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.gob")
	model := NewGNNClassifier(DefaultGNNConfig())
	if err := model.Serialize(path); err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, cut := range []int{0, 1, 8, len(valid) / 2, len(valid) - 1} {
		assertGNNModelRejected(t, path, valid[:cut])
	}
	assertGNNModelRejected(t, path, append(append([]byte(nil), valid...), 0xff))
}

func TestGNNModelDeserializerRejectsUnsafeConfigAndShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.gob")
	model := NewGNNClassifier(DefaultGNNConfig())

	unsafeConfig := model.toSave()
	unsafeConfig.Config.HiddenDim = maxGNNHiddenDim + 1
	writeGNNModelSave(t, path, unsafeConfig)
	assertGNNModelRejected(t, path, nil)

	unsafeShape := model.toSave()
	unsafeShape.ReadoutFC1.Weights = nil
	writeGNNModelSave(t, path, unsafeShape)
	assertGNNModelRejected(t, path, nil)

	unsafeValue := model.toSave()
	unsafeValue.Classifier.Weights[0][0] = math.NaN()
	writeGNNModelSave(t, path, unsafeValue)
	assertGNNModelRejected(t, path, nil)
}

func writeGNNModelSave(t *testing.T, path string, save gnnModelSave) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := gob.NewEncoder(file).Encode(&save); err != nil {
		_ = file.Close()
		t.Fatalf("Encode() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertGNNModelRejected(t *testing.T, path string, payload []byte) {
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
	if _, err := DeserializeGNNClassifier(path); err == nil {
		t.Fatal("deserializer accepted malformed model")
	}
}
