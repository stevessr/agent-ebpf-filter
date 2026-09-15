package ml

// FeatureSourceDescriptor documents one stable slice of the shared 128-dim
// feature vector consumed by the local classifiers.
type FeatureSourceDescriptor struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Dimensions  int    `json:"dimensions"`
	Description string `json:"description"`
}

// FeatureSourceCatalog mirrors FeatureExtractor's layout. Keep these ranges
// stable because persisted training data and serialized models rely on the
// semantic meaning of each feature dimension.
func FeatureSourceCatalog() []FeatureSourceDescriptor {
	return []FeatureSourceDescriptor{
		{
			ID:          FeatureSourceCommandProcess,
			Label:       "命令/进程特征",
			Start:       0,
			End:         31,
			Dimensions:  32,
			Description: "行为类别、shell/root/network/file flags、分类置信度与命令长度。",
		},
		{
			ID:          FeatureSourceArgumentStats,
			Label:       "参数统计特征",
			Start:       32,
			End:         63,
			Dimensions:  32,
			Description: "参数长度/熵、flag/position、敏感路径、扩展名、URL/IP、重定向与唯一性。",
		},
		{
			ID:          FeatureSourceEmbedding,
			Label:       "语义 Embedding",
			Start:       64,
			End:         95,
			Dimensions:  32,
			Description: "命令与参数的 64 维 LSH embedding 前 32 维投影。",
		},
		{
			ID:          FeatureSourceHistory,
			Label:       "近期行为历史",
			Start:       96,
			End:         111,
			Dimensions:  16,
			Description: "命令频率、BLOCK/ALERT 比例、异常趋势、敏感/网络/root 比例与主体多样性。",
		},
		{
			ID:          FeatureSourceTemporal,
			Label:       "事件速率/时间",
			Start:       112,
			End:         119,
			Dimensions:  8,
			Description: "每秒速率、活跃 PID、时段/星期、周期时间编码与近期平均参数长度。",
		},
		{
			ID:          FeatureSourceNetworkAudit,
			Label:       "网络审计特征",
			Start:       120,
			End:         127,
			Dimensions:  8,
			Description: "网络风险分、可疑端口、反弹 shell、外泄、DNS tunnel、明文协议、异常目标与扫描。",
		},
	}
}
