package app

import (
	"testing"
)

func TestNGramExtractor(t *testing.T) {
	extractor := NewNGramExtractor(3, 16)

	// 测试离散化
	bin := extractor.discretize(0.5)
	if bin < 0 || bin >= 16 {
		t.Fatalf("discretize out of range: %d", bin)
	}

	// 测试特征提取
	var features [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		features[i] = float64(i) * 0.1
	}

	ngramFeatures := extractor.ExtractNGrams(features)

	// 验证输出维度
	if len(ngramFeatures) != FeatureDim {
		t.Fatalf("output dimension mismatch: got %d, want %d", len(ngramFeatures), FeatureDim)
	}

	// 验证 unigram 统计
	if len(extractor.UnigramFreq) == 0 {
		t.Fatal("unigram frequency map is empty")
	}

	// 验证 bigram 统计
	if len(extractor.BigramFreq) == 0 {
		t.Fatal("bigram frequency map is empty")
	}

	// 验证 trigram 统计
	if len(extractor.TrigramFreq) == 0 {
		t.Fatal("trigram frequency map is empty")
	}
}

func TestNGramModel(t *testing.T) {
	// 创建一个简单的基线分类器
	baseClassifier := NewDecisionForest(5, 3, 2)

	// 创建 N-Gram 模型
	ngramModel := NewNGramModel(3, 16, baseClassifier, ModelRandomForest)

	// 测试预测
	var features [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		features[i] = float64(i) * 0.05
	}

	pred := ngramModel.Predict(features)

	// 验证预测结果
	if pred.Action < 0 || pred.Action > 2 {
		t.Fatalf("invalid prediction action: %d", pred.Action)
	}

	if pred.Confidence < 0 || pred.Confidence > 1 {
		t.Fatalf("invalid prediction confidence: %f", pred.Confidence)
	}
}

func TestNGramModelType(t *testing.T) {
	baseClassifier := NewDecisionForest(5, 3, 2)
	ngramModel := NewNGramModel(3, 16, baseClassifier, ModelRandomForest)

	if ngramModel.Type() != ModelRandomForest {
		t.Fatalf("model type mismatch: got %s, want %s", ngramModel.Type(), ModelRandomForest)
	}
}

func BenchmarkNGramExtraction(b *testing.B) {
	extractor := NewNGramExtractor(3, 16)
	var features [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		features[i] = float64(i) * 0.1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractor.ExtractNGrams(features)
	}
}

func BenchmarkNGramModelPredict(b *testing.B) {
	baseClassifier := NewDecisionForest(5, 3, 2)
	ngramModel := NewNGramModel(3, 16, baseClassifier, ModelRandomForest)

	var features [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		features[i] = float64(i) * 0.05
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ngramModel.Predict(features)
	}
}
