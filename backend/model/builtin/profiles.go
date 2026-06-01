package main

// BuiltinModelCatalogItem describes one local model/profile exposed to the UI.
type BuiltinModelCatalogItem struct {
	Value       string         `json:"value"`
	Label       string         `json:"label"`
	Base        string         `json:"base"`
	Category    string         `json:"category"`
	Description string         `json:"description"`
	Recommended bool           `json:"recommended,omitempty"`
	Defaults    map[string]int `json:"defaults,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
}

type builtinModelProfile struct {
	Type        ModelType
	Base        ModelType
	Label       string
	Category    string
	Description string
	Recommended bool
	Defaults    map[string]int
	Tags        []string
	Apply       func(MLConfig) MLConfig
}

var builtinModelProfiles = []builtinModelProfile{
	p(ModelRandomForest, ModelRandomForest, "Random Forest", "树模型", "稳健默认随机森林，适合作为主力 baseline。", true, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"holdout", "stable"}, nil),
	p(ModelRandomForestFast, ModelRandomForest, "Random Forest Fast", "树模型", "少量浅树，优先推理速度与正确命令放行延迟。", true, map[string]int{"numTrees": 5, "maxDepth": 4, "minSamplesLeaf": 2}, []string{"fast", "allow"}, nil),
	p(ModelRandomForestShallow, ModelRandomForest, "Random Forest Shallow", "树模型", "浅层森林，降低过拟合与误伤风险。", false, map[string]int{"numTrees": 15, "maxDepth": 4, "minSamplesLeaf": 3}, []string{"low-overfit"}, nil),
	p(ModelRandomForestStable, ModelRandomForest, "Random Forest Stable", "树模型", "中等深度 + 较多树，偏稳定性和多次重复表现。", true, map[string]int{"numTrees": 51, "maxDepth": 10, "minSamplesLeaf": 3}, []string{"stable"}, nil),
	p(ModelRandomForestDeep, ModelRandomForest, "Random Forest Deep", "树模型", "更深森林，用于探索高上限但训练/推理成本更高。", false, map[string]int{"numTrees": 31, "maxDepth": 12, "minSamplesLeaf": 3}, []string{"high-capacity"}, nil),
	p(ModelRandomForestWide, ModelRandomForest, "Random Forest Wide", "树模型", "更多树的宽森林，降低方差。", false, map[string]int{"numTrees": 71, "maxDepth": 8, "minSamplesLeaf": 3}, []string{"wide"}, nil),

	p(ModelExtraTrees, ModelExtraTrees, "Extra Trees", "树模型", "随机阈值的极随机树，对照随机森林泛化。", false, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"randomized"}, nil),
	p(ModelExtraTreesFast, ModelExtraTrees, "Extra Trees Fast", "树模型", "少量 Extra Trees，速度优先。", false, map[string]int{"numTrees": 9, "maxDepth": 5, "minSamplesLeaf": 3}, []string{"fast"}, nil),
	p(ModelExtraTreesDeep, ModelExtraTrees, "Extra Trees Deep", "树模型", "深层 Extra Trees，高容量探索。", false, map[string]int{"numTrees": 31, "maxDepth": 12, "minSamplesLeaf": 3}, []string{"high-capacity"}, nil),
	p(ModelExtraTreesWide, ModelExtraTrees, "Extra Trees Wide", "树模型", "更多 Extra Trees，偏低方差对照。", false, map[string]int{"numTrees": 71, "maxDepth": 10, "minSamplesLeaf": 3}, []string{"wide"}, nil),

	p(ModelLogisticRegression, ModelLogisticRegression, "Logistic L2", "线性模型", "L2 正则逻辑回归，轻量可解释。", false, map[string]int{"numTrees": 10, "maxDepth": 8, "minSamplesLeaf": 1000}, []string{"interpretable"}, nil),
	p(ModelLogisticFast, ModelLogisticRegression, "Logistic Fast", "线性模型", "少迭代逻辑回归，快速基线。", false, map[string]int{"numTrees": 20, "maxDepth": 8, "minSamplesLeaf": 500}, []string{"fast"}, nil),
	p(ModelLogisticNone, ModelLogisticRegression, "Logistic None", "线性模型", "无正则逻辑回归，观察正则化影响。", false, map[string]int{"numTrees": 50, "maxDepth": 4, "minSamplesLeaf": 2000}, []string{"ablation"}, nil),
	p(ModelLogisticL1, ModelLogisticRegression, "Logistic L1", "线性模型", "L1 稀疏逻辑回归，便于解释关键特征。", true, map[string]int{"numTrees": 100, "maxDepth": 12, "minSamplesLeaf": 4000}, []string{"interpretable", "sparse"}, nil),
	p(ModelLogisticBalanced, ModelLogisticRegression, "Logistic Balanced", "线性模型", "类别加权 L2 逻辑回归，面向稀缺高风险类。", false, map[string]int{"numTrees": 20, "maxDepth": 8, "minSamplesLeaf": 2000}, []string{"balanced"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),
	p(ModelLogisticL1Balanced, ModelLogisticRegression, "Logistic L1 Balanced", "线性模型", "L1 + 类别加权，兼顾稀疏解释与不平衡。", false, map[string]int{"numTrees": 50, "maxDepth": 12, "minSamplesLeaf": 4000}, []string{"balanced", "sparse"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),

	p(ModelSVM, ModelSVM, "Linear SVM", "线性模型", "线性 hinge-loss 基线。", false, map[string]int{"numTrees": 5, "maxDepth": 8, "minSamplesLeaf": 500}, []string{"margin"}, nil),
	p(ModelSVMLong, ModelSVM, "SVM Long", "线性模型", "更长迭代 SVM，用于收敛性探索。", false, map[string]int{"numTrees": 5, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"long"}, nil),
	p(ModelSVMBalanced, ModelSVM, "SVM Balanced", "线性模型", "类别加权 SVM，侧重少数类召回。", false, map[string]int{"numTrees": 10, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"balanced"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),

	p(ModelPerceptron, ModelPerceptron, "Perceptron", "在线线性模型", "在线感知机基线。", false, map[string]int{"numTrees": 20, "maxDepth": 8, "minSamplesLeaf": 1000}, []string{"online"}, nil),
	p(ModelPerceptronLong, ModelPerceptron, "Perceptron Long", "在线线性模型", "更长迭代感知机。", false, map[string]int{"numTrees": 150, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"online", "long"}, nil),
	p(ModelPerceptronBalanced, ModelPerceptron, "Perceptron Balanced", "在线线性模型", "类别加权感知机。", false, map[string]int{"numTrees": 50, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"balanced", "online"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),

	p(ModelPassiveAggressive, ModelPassiveAggressive, "Passive-Aggressive", "在线线性模型", "PA 在线更新模型，适合流式对照。", false, map[string]int{"numTrees": 10, "maxDepth": 8, "minSamplesLeaf": 1000}, []string{"online"}, nil),
	p(ModelPassiveAggressiveLong, ModelPassiveAggressive, "PA Long", "在线线性模型", "更长迭代 PA 模型。", false, map[string]int{"numTrees": 20, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"online", "long"}, nil),
	p(ModelPassiveAggressiveBalanced, ModelPassiveAggressive, "PA Balanced", "在线线性模型", "类别加权 PA 模型。", false, map[string]int{"numTrees": 20, "maxDepth": 8, "minSamplesLeaf": 4000}, []string{"balanced", "online"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),

	p(ModelKNN, ModelKNN, "KNN Euclidean", "近邻/原型", "欧氏距离 KNN，训练快但推理随样本数增长。", false, map[string]int{"numTrees": 5, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"instance-based"}, nil),
	p(ModelKNNManhattan, ModelKNN, "KNN Manhattan", "近邻/原型", "曼哈顿距离 KNN，高维稀疏特征对照。", false, map[string]int{"numTrees": 7, "maxDepth": 12, "minSamplesLeaf": 5}, []string{"distance"}, nil),
	p(ModelKNNCosine, ModelKNN, "KNN Cosine", "近邻/原型", "余弦距离 KNN，关注命令模式方向相似。", false, map[string]int{"numTrees": 7, "maxDepth": 16, "minSamplesLeaf": 5}, []string{"distance"}, nil),
	p(ModelKNNDistance, ModelKNN, "KNN Distance Weighted", "近邻/原型", "距离加权 KNN，近邻影响更大。", false, map[string]int{"numTrees": 5, "maxDepth": 12, "minSamplesLeaf": 8}, []string{"distance", "weighted"}, nil),

	p(ModelNearestCentroid, ModelNearestCentroid, "Nearest Centroid", "近邻/原型", "欧氏最近质心，极快且解释清晰。", false, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"fast", "interpretable"}, nil),
	p(ModelNearestCentroidBalanced, ModelNearestCentroid, "Nearest Centroid Balanced", "近邻/原型", "最近质心 + 均匀先验，减少多数类偏置。", false, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"balanced", "fast"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),
	p(ModelNearestCentroidCosine, ModelNearestCentroid, "Nearest Centroid Cosine", "近邻/原型", "余弦质心，适合模式方向相似度。", false, map[string]int{"numTrees": 20, "maxDepth": 4, "minSamplesLeaf": 5}, []string{"cosine", "fast"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),
	p(ModelNearestCentroidManhattan, ModelNearestCentroid, "Nearest Centroid Manhattan", "近邻/原型", "曼哈顿质心，适合稀疏/离散特征。", false, map[string]int{"numTrees": 40, "maxDepth": 12, "minSamplesLeaf": 5}, []string{"manhattan", "fast"}, nil),

	p(ModelNaiveBayes, ModelNaiveBayes, "Naive Bayes", "概率模型", "高斯朴素贝叶斯，快速概率 baseline。", false, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"probabilistic"}, nil),
	p(ModelNaiveBayesBalanced, ModelNaiveBayes, "Naive Bayes Balanced", "概率模型", "均匀类先验朴素贝叶斯。", false, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"balanced", "probabilistic"}, func(c MLConfig) MLConfig { c.BalanceClasses = true; return c }),

	p(ModelRidge, ModelRidge, "Ridge", "线性模型", "Ridge 分类器，速度快。", false, map[string]int{"numTrees": 5, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"linear"}, nil),
	p(ModelRidgeLight, ModelRidge, "Ridge Light", "线性模型", "低正则 Ridge，快速探索。", false, map[string]int{"numTrees": 1, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"linear", "light"}, nil),
	p(ModelRidgeStrong, ModelRidge, "Ridge Strong", "线性模型", "强正则 Ridge，观察过拟合/欠拟合边界。", false, map[string]int{"numTrees": 200, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"linear", "regularized"}, nil),

	p(ModelAdaBoost, ModelAdaBoost, "AdaBoost", "Boosting/集成", "决策桩 AdaBoost，对照 boosting 方向。", false, map[string]int{"numTrees": 100, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"boosting"}, nil),
	p(ModelAdaBoostFast, ModelAdaBoost, "AdaBoost Fast", "Boosting/集成", "少估计器 AdaBoost，速度优先。", false, map[string]int{"numTrees": 25, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"boosting", "fast"}, nil),
	p(ModelAdaBoostLarge, ModelAdaBoost, "AdaBoost Large", "Boosting/集成", "更多估计器 AdaBoost，容量对照。", false, map[string]int{"numTrees": 200, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"boosting", "large"}, nil),

	p(ModelEnsemble, ModelEnsemble, "Soft-vote Ensemble", "Boosting/集成", "本地软投票集成，融合 RF/Logistic/NB/KNN/Centroid/LightRF。", true, map[string]int{"numTrees": 31, "maxDepth": 8, "minSamplesLeaf": 5}, []string{"ensemble", "stable"}, nil),
}

func p(t, base ModelType, label, category, description string, recommended bool, defaults map[string]int, tags []string, apply func(MLConfig) MLConfig) builtinModelProfile {
	return builtinModelProfile{Type: t, Base: base, Label: label, Category: category, Description: description, Recommended: recommended, Defaults: defaults, Tags: tags, Apply: apply}
}

func init() {
	for _, profile := range builtinModelProfiles {
		if profile.Type == profile.Base {
			continue
		}
		alias, base := profile.Type, profile.Base
		RegisterModel(alias, func() Model {
			model, err := NewModel(base)
			if err != nil {
				return nil
			}
			return wrapModelType(model, alias)
		})
	}
}

func BuiltinModelCatalog() []BuiltinModelCatalogItem {
	items := make([]BuiltinModelCatalogItem, 0, len(builtinModelProfiles))
	for _, profile := range builtinModelProfiles {
		items = append(items, BuiltinModelCatalogItem{
			Value:       string(profile.Type),
			Label:       profile.Label,
			Base:        string(profile.Base),
			Category:    profile.Category,
			Description: profile.Description,
			Recommended: profile.Recommended,
			Defaults:    copyDefaultMap(profile.Defaults),
			Tags:        append([]string(nil), profile.Tags...),
		})
	}
	return items
}

func AllModelTypeStrings() []string {
	types := AllModelTypes()
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, string(t))
	}
	return out
}

func builtinModelDisplayName(t ModelType) (string, bool) {
	for _, profile := range builtinModelProfiles {
		if profile.Type == t {
			return profile.Label, true
		}
	}
	return "", false
}

func baseModelType(t ModelType) ModelType {
	for _, profile := range builtinModelProfiles {
		if profile.Type == t {
			return profile.Base
		}
	}
	return t
}

func applyBuiltinModelPreset(cfg MLConfig) MLConfig {
	requested := cfg.ModelType
	if requested == "" {
		requested = ModelRandomForest
	}
	for _, profile := range builtinModelProfiles {
		if profile.Type != requested {
			continue
		}
		cfg.ModelType = profile.Base
		if shouldApplyBuiltinDefaults(cfg, profile) {
			if v := profile.Defaults["numTrees"]; v > 0 {
				cfg.NumTrees = v
			}
			if v := profile.Defaults["maxDepth"]; v > 0 {
				cfg.MaxDepth = v
			}
			if v := profile.Defaults["minSamplesLeaf"]; v > 0 {
				cfg.MinSamplesLeaf = v
			}
		}
		if profile.Apply != nil {
			cfg = profile.Apply(cfg)
		}
		return cfg
	}
	cfg.ModelType = requested
	return cfg
}

func shouldApplyBuiltinDefaults(cfg MLConfig, profile builtinModelProfile) bool {
	if profile.Type == profile.Base {
		return cfg.NumTrees <= 0 || cfg.MaxDepth <= 0 || cfg.MinSamplesLeaf <= 0
	}
	if cfg.NumTrees <= 0 || cfg.MaxDepth <= 0 || cfg.MinSamplesLeaf <= 0 {
		return true
	}
	defaults := DefaultMLConfig()
	return cfg.NumTrees == defaults.NumTrees && cfg.MaxDepth == defaults.MaxDepth && cfg.MinSamplesLeaf == defaults.MinSamplesLeaf
}

func copyDefaultMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

type modelTypeWrapper struct {
	inner  Model
	typeID ModelType
}

func (m *modelTypeWrapper) Predict(features [FeatureDim]float64) Prediction {
	return m.inner.Predict(features)
}
func (m *modelTypeWrapper) Serialize(path string) error { return m.inner.Serialize(path) }
func (m *modelTypeWrapper) Type() ModelType             { return m.typeID }

func wrapModelType(model Model, requested ModelType) Model {
	if model == nil {
		return nil
	}
	if requested == "" || model.Type() == requested {
		return model
	}
	return &modelTypeWrapper{inner: model, typeID: requested}
}

func unwrapModelType(model Model) Model {
	if wrapped, ok := model.(*modelTypeWrapper); ok && wrapped.inner != nil {
		return wrapped.inner
	}
	return model
}
