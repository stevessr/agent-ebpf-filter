package ml

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"agent-ebpf-filter/core"
)

type extendedModelSafetyCase struct {
	name        string
	serialize   func(string) error
	deserialize func(string) error
}

func extendedModelSafetyCases() []extendedModelSafetyCase {
	forest := NewDecisionForest(1, 8, mlBinaryMaxClasses)
	forest.Trees[0] = DecisionTree{Nodes: []DecisionNode{{LeftChild: -1, RightChild: -1}}}
	forest.IsTrained = true

	knn := NewKNNModel(1, "euclidean", "uniform")
	knn.Samples = make([][FeatureDim]float64, 1)
	knn.Labels = []int32{0}

	logistic := NewLogisticModel(0.01, "l2", 100)
	logistic.Weights = make([][FeatureDim + 1]float64, logistic.NumClasses)

	centroid := NewNearestCentroid("cosine", true)
	centroid.Centroids = make([][FeatureDim]float64, centroid.Classes)
	centroid.Priors = make([]float64, centroid.Classes)
	for index := range centroid.Priors {
		centroid.Priors[index] = 1 / float64(centroid.Classes)
	}

	ganTransformer := NewGANTransformerModel(16, 24, 8)
	ensemble := NewEnsembleModel([]Model{logistic}, "soft", []float64{1})
	ngram := NewNGramModel(3, 16, logistic, core.ModelNGramLogistic)

	return []extendedModelSafetyCase{
		{name: "forest", serialize: forest.Serialize, deserialize: modelDeserializer(DeserializeForest)},
		{name: "knn", serialize: knn.Serialize, deserialize: modelDeserializer(DeserializeKNN)},
		{name: "logistic", serialize: logistic.Serialize, deserialize: modelDeserializer(DeserializeLogistic)},
		{name: "nearest_centroid", serialize: centroid.Serialize, deserialize: modelDeserializer(DeserializeNearestCentroid)},
		{name: "additive_attention", serialize: NewAdditiveAttention().Serialize, deserialize: modelDeserializer(DeserializeAdditiveAttention)},
		{name: "cross_attention", serialize: NewCrossAttentionLayer().Serialize, deserialize: modelDeserializer(DeserializeCrossAttention)},
		{name: "mamba_attention", serialize: NewMambaAttention().Serialize, deserialize: modelDeserializer(DeserializeMambaAttention)},
		{name: "multi_head_attention", serialize: NewMultiHeadAttention(4).Serialize, deserialize: modelDeserializer(DeserializeMultiHeadAttention)},
		{name: "rwkv_attention", serialize: NewRWKVAttention().Serialize, deserialize: modelDeserializer(DeserializeRWKVAttention)},
		{name: "scaled_dot_product_attention", serialize: NewScaledDotProductAttention().Serialize, deserialize: modelDeserializer(DeserializeScaledDotProductAttention)},
		{name: "self_attention", serialize: NewSelfAttention().Serialize, deserialize: modelDeserializer(DeserializeSelfAttention)},
		{name: "gan_transformer", serialize: ganTransformer.Serialize, deserialize: modelDeserializer(DeserializeGANTransformer)},
		{name: "ensemble", serialize: ensemble.Serialize, deserialize: modelDeserializer(DeserializeEnsemble)},
		{
			name:      "ngram",
			serialize: ngram.Serialize,
			deserialize: func(path string) error {
				_, err := DeserializeNGramModel(path, logistic, core.ModelNGramLogistic)
				return err
			},
		},
	}
}

func modelDeserializer[T any](deserialize func(string) (T, error)) func(string) error {
	return func(path string) error {
		_, err := deserialize(path)
		return err
	}
}

func TestExtendedModelDeserializersRejectTruncationAndTrailingData(t *testing.T) {
	for caseIndex, tc := range extendedModelSafetyCases() {
		t.Run(tc.name, func(t *testing.T) {
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
			rng := rand.New(rand.NewSource(int64(9000 + caseIndex)))
			for index := 0; index < 20; index++ {
				cuts[rng.Intn(len(valid))] = struct{}{}
			}
			for cut := range cuts {
				assertBinaryModelRejected(t, path, valid[:cut], tc.deserialize)
			}
			assertBinaryModelRejected(t, path, append(append([]byte(nil), valid...), 0xff), tc.deserialize)
		})
	}
}

func TestVariableLengthModelDeserializersRejectHostileCounts(t *testing.T) {
	for _, tc := range extendedModelSafetyCases() {
		if tc.name != "forest" && tc.name != "knn" && tc.name != "logistic" && tc.name != "nearest_centroid" && tc.name != "multi_head_attention" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model.bin")
			if err := tc.serialize(path); err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if len(payload) < 16 {
				t.Fatalf("serialized payload is unexpectedly short: %d", len(payload))
			}
			binary.LittleEndian.PutUint32(payload[8:12], ^uint32(0))
			assertBinaryModelRejected(t, path, payload, tc.deserialize)
		})
	}
}

func TestEnsembleDeserializerRejectsEscapingMemberPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ensemble.json")
	payload := []byte(`{"version":1,"voting":"soft","weights":[1],"modelTypes":["knn"],"modelFiles":["../outside.bin"]}`)
	assertBinaryModelRejected(t, path, payload, modelDeserializer(DeserializeEnsemble))
}

func TestGANTransformerDeserializerRejectsUnsafeDimensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gan.json")
	payload := []byte(fmt.Sprintf(`{"numClasses":%d,"latentDim":16,"epochs":24,"syntheticPerClass":8}`, mlMaxTrainingSamples))
	assertBinaryModelRejected(t, path, payload, modelDeserializer(DeserializeGANTransformer))
}

func TestBinaryModelReadersRejectNonFiniteValues(t *testing.T) {
	for _, tc := range extendedModelSafetyCases() {
		if tc.name != "forest" && tc.name != "logistic" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model.bin")
			if err := tc.serialize(path); err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			switch tc.name {
			case "forest":
				binary.LittleEndian.PutUint32(payload[22:26], math.Float32bits(float32(math.NaN())))
			case "logistic":
				binary.LittleEndian.PutUint64(payload[12:20], math.Float64bits(math.NaN()))
			}
			assertBinaryModelRejected(t, path, payload, tc.deserialize)
		})
	}
}
