package ml

import (
	"agent-ebpf-filter/core"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// NGramFeatureExtractor 从原始特征向量中提取 n-gram 风格的序列特征
// 将连续的特征值视为"词序列"，提取 unigram, bigram, trigram 统计
type NGramFeatureExtractor struct {
	N           int // n-gram 的最大阶数 (1=unigram, 2=bigram, 3=trigram)
	BinCount    int // 特征值离散化的 bin 数量
	UnigramFreq map[int]float64
	BigramFreq  map[[2]int]float64
	TrigramFreq map[[3]int]float64
}

// NewNGramExtractor 创建一个 n-gram 特征提取器
func NewNGramExtractor(n int, binCount int) *NGramFeatureExtractor {
	return &NGramFeatureExtractor{
		N:           n,
		BinCount:    binCount,
		UnigramFreq: make(map[int]float64),
		BigramFreq:  make(map[[2]int]float64),
		TrigramFreq: make(map[[3]int]float64),
	}
}

// discretize 将连续特征值离散化到 bin
func (ng *NGramFeatureExtractor) discretize(val float64) int {
	// 使用 tanh 压缩到 [-1, 1] 然后映射到 [0, BinCount-1]
	normalized := math.Tanh(val)
	bin := int((normalized + 1.0) / 2.0 * float64(ng.BinCount-1))
	if bin < 0 {
		bin = 0
	}
	if bin >= ng.BinCount {
		bin = ng.BinCount - 1
	}
	return bin
}

// ExtractNGrams 从特征向量提取 n-gram 统计特征
func (ng *NGramFeatureExtractor) ExtractNGrams(features [FeatureDim]float64) [FeatureDim]float64 {
	// 离散化所有特征
	bins := make([]int, FeatureDim)
	for i := 0; i < FeatureDim; i++ {
		bins[i] = ng.discretize(features[i])
	}

	// 清空频率统计
	ng.UnigramFreq = make(map[int]float64)
	ng.BigramFreq = make(map[[2]int]float64)
	ng.TrigramFreq = make(map[[3]int]float64)

	// 统计 unigram
	if ng.N >= 1 {
		for _, bin := range bins {
			ng.UnigramFreq[bin]++
		}
	}

	// 统计 bigram
	if ng.N >= 2 {
		for i := 0; i < len(bins)-1; i++ {
			bigram := [2]int{bins[i], bins[i+1]}
			ng.BigramFreq[bigram]++
		}
	}

	// 统计 trigram
	if ng.N >= 3 {
		for i := 0; i < len(bins)-2; i++ {
			trigram := [3]int{bins[i], bins[i+1], bins[i+2]}
			ng.TrigramFreq[trigram]++
		}
	}

	// 构建新的特征向量
	var output [FeatureDim]float64
	idx := 0

	// 填充 unigram 特征 (前 BinCount 维)
	for i := 0; i < ng.BinCount && idx < FeatureDim; i++ {
		output[idx] = ng.UnigramFreq[i] / float64(FeatureDim)
		idx++
	}

	// 填充 bigram 特征的代表性样本
	maxBigrams := min(FeatureDim-idx, 32)
	bigramCount := 0
	for bg, freq := range ng.BigramFreq {
		_ = bg // 使用 bigram 键
		if bigramCount >= maxBigrams {
			break
		}
		if idx < FeatureDim {
			// 使用转移概率作为特征
			output[idx] = freq / float64(FeatureDim-1)
			idx++
			bigramCount++
		}
	}

	// 填充 trigram 特征的代表性样本
	maxTrigrams := min(FeatureDim-idx, 16)
	trigramCount := 0
	for tg, freq := range ng.TrigramFreq {
		_ = tg // 使用 trigram 键
		if trigramCount >= maxTrigrams {
			break
		}
		if idx < FeatureDim {
			output[idx] = freq / float64(FeatureDim-2)
			idx++
			trigramCount++
		}
	}

	// 剩余维度填充原始特征
	for idx < FeatureDim {
		output[idx] = features[idx]
		idx++
	}

	return output
}

// NGramModel 结合 n-gram 特征提取和分类器
type NGramModel struct {
	Extractor  *NGramFeatureExtractor
	Classifier Model
	modelType  core.ModelType
}

// NewNGramModel 创建一个 n-gram 增强的模型
func NewNGramModel(n int, binCount int, classifier Model, modelType core.ModelType) *NGramModel {
	return &NGramModel{
		Extractor:  NewNGramExtractor(n, binCount),
		Classifier: classifier,
		modelType:  modelType,
	}
}

func (m *NGramModel) Type() core.ModelType {
	return m.modelType
}

func (m *NGramModel) Predict(features [FeatureDim]float64) Prediction {
	// 提取 n-gram 特征
	ngramFeatures := m.Extractor.ExtractNGrams(features)

	// 使用分类器预测
	return m.Classifier.Predict(ngramFeatures)
}

func (m *NGramModel) Serialize(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写入魔数和版本
	if _, err := f.Write([]byte{'N', 'G', 'R', 'M'}); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(1)); err != nil {
		return err
	}

	// 写入 n-gram 配置
	if err := binary.Write(f, binary.LittleEndian, uint32(m.Extractor.N)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(m.Extractor.BinCount)); err != nil {
		return err
	}

	// 保存底层分类器
	classifierPath := path + ".classifier"
	if err := m.Classifier.Serialize(classifierPath); err != nil {
		return fmt.Errorf("serialize classifier: %w", err)
	}

	return nil
}

func DeserializeNGramModel(path string, classifier Model, modelType core.ModelType) (*NGramModel, error) {
	r, err := newMLBinaryModelReader(path, "NGRM")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	n := r.readBoundedCount("n-gram order", 1, 3)
	binCount := r.readBoundedCount("n-gram bin count", 1, FeatureDim)
	if err := r.done(); err != nil {
		return nil, err
	}
	if classifier == nil {
		return nil, fmt.Errorf("n-gram classifier is nil")
	}
	return NewNGramModel(n, binCount, classifier, modelType), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	// 注册 n-gram 增强的模型变种
	RegisterModel(core.ModelNGramRandomForest, func() Model {
		baseRF := NewDecisionForest(31, 8, 4)
		return NewNGramModel(3, 16, baseRF, core.ModelNGramRandomForest)
	})

	RegisterModel(core.ModelNGramLogistic, func() Model {
		baseLogistic := NewLogisticModel(0.01, "l2", 1000)
		return NewNGramModel(3, 16, baseLogistic, core.ModelNGramLogistic)
	})

	RegisterModel(core.ModelNGramKNN, func() Model {
		baseKNN := NewKNNModel(5, "euclidean", "uniform")
		return NewNGramModel(3, 16, baseKNN, core.ModelNGramKNN)
	})
}
