package ml

import (
	"math"
	"testing"
)

func TestKNNExactDuplicateVotesAreNotTruncatedByK(t *testing.T) {
	knn := NewKNNModel(1, "euclidean", "distance")
	knn.Samples = make([][FeatureDim]float64, 8)
	knn.Labels = []int32{0, 1, 1, 1, 1, 1, 1, 1}
	var point [FeatureDim]float64
	point[4] = 0.5
	for i := range knn.Samples {
		knn.Samples[i] = point
	}
	prediction := knn.Predict(point)
	if prediction.Action != 1 || math.Abs(prediction.Confidence-0.875) > 1e-9 {
		t.Fatalf("one first ALLOW row overruled seven identical BLOCK rows at K=1: %+v", prediction)
	}
	if prediction.AnomalyScore > 1e-9 {
		t.Fatalf("exact duplicate neighbors must have zero distance anomaly: %+v", prediction)
	}
}

func TestKNNInvalidSampleDistancesAreExcluded(t *testing.T) {
	knn := NewKNNModel(2, "euclidean", "uniform")
	knn.Samples = make([][FeatureDim]float64, 2)
	knn.Labels = []int32{1, 0}
	knn.Samples[0][0] = math.NaN()
	knn.Samples[1][0] = 0.25
	var query [FeatureDim]float64
	query[0] = 0.25
	prediction := knn.Predict(query)
	if prediction.Action != 0 || prediction.Confidence != 1 {
		t.Fatalf("NaN-distance BLOCK sample participated in voting: %+v", prediction)
	}
}

func TestKNNDistanceWeightedExactDuplicatesIncludeAllLabels(t *testing.T) {
	knn := NewKNNModel(2, "manhattan", "distance")
	knn.Samples = make([][FeatureDim]float64, 6)
	knn.Labels = []int32{1, 3, 3, 3, 3, 0}
	var q [FeatureDim]float64
	q[0] = 0.125
	for i := 0; i < 5; i++ {
		knn.Samples[i] = q
	}
	knn.Samples[5][0] = 0.1251
	prediction := knn.Predict(q)
	if prediction.Action != 3 || math.Abs(prediction.Confidence-0.8) > 1e-9 {
		t.Fatalf("exact ALERT evidence lost to K truncation or near neighbors: %+v", prediction)
	}
}
