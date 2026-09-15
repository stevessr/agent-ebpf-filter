package ml

import "sort"

// ModelSecurityGroupSummary aggregates security behavior for one
// model-family × feature-representation group. Recall values are weighted by
// the actual validation support of each attack vector rather than averaging
// per-model percentages, which would over-emphasize tiny slices.
type ModelSecurityGroupSummary struct {
	Key                       string  `json:"key"`
	Family                    string  `json:"family"`
	FamilyLabel               string  `json:"familyLabel"`
	FeatureClass              string  `json:"featureClass"`
	ModelCount                int     `json:"modelCount"`
	SuccessfulModels          int     `json:"successfulModels"`
	ScoredSamples             int     `json:"scoredSamples"`
	AttackSamples             int     `json:"attackSamples"`
	HighImpactSamples         int     `json:"highImpactSamples"`
	IntrusionSamples          int     `json:"intrusionSamples"`
	DestructionSamples        int     `json:"destructionSamples"`
	ExfiltrationSamples       int     `json:"exfiltrationSamples"`
	PersistenceSamples        int     `json:"persistenceSamples"`
	MeanSecurityUtility       float64 `json:"meanSecurityUtility"`
	AttackRecall              float64 `json:"attackRecall"`
	HighImpactRecall          float64 `json:"highImpactRecall"`
	IntrusionRecall           float64 `json:"intrusionRecall"`
	DestructionRecall         float64 `json:"destructionRecall"`
	ExfiltrationRecall        float64 `json:"exfiltrationRecall"`
	PersistenceRecall         float64 `json:"persistenceRecall"`
	CatastrophicMissRate      float64 `json:"catastrophicMissRate"`
	BenignFalsePositiveRate   float64 `json:"benignFalsePositiveRate"`
	BestModelType             string  `json:"bestModelType,omitempty"`
	BestModelLabel            string  `json:"bestModelLabel,omitempty"`
	BestModelSecurityUtility  float64 `json:"bestModelSecurityUtility"`
}

type modelSecurityGroupAccumulator struct {
	summary                ModelSecurityGroupSummary
	securityUtilityWeight  float64
	attackDetected         float64
	highImpactDetected     float64
	intrusionDetected      float64
	destructionDetected    float64
	exfiltrationDetected   float64
	persistenceDetected    float64
	catastrophicMisses     float64
	benignSamples          int
	benignFalsePositives   float64
}

// SummarizeModelTuneSecurity returns deterministic family × feature summaries
// suitable for logs, APIs and later visualization.
func SummarizeModelTuneSecurity(candidates []MLModelTuneCandidate) []ModelSecurityGroupSummary {
	groups := map[string]*modelSecurityGroupAccumulator{}
	for _, candidate := range candidates {
		key := candidate.Family + "/" + candidate.FeatureClass
		group := groups[key]
		if group == nil {
			group = &modelSecurityGroupAccumulator{summary: ModelSecurityGroupSummary{
				Key:          key,
				Family:       candidate.Family,
				FamilyLabel:  candidate.FamilyLabel,
				FeatureClass: candidate.FeatureClass,
			}}
			groups[key] = group
		}
		group.summary.ModelCount++
		if candidate.Error != "" || candidate.AttackMetrics.ScoredSamples <= 0 {
			continue
		}

		group.summary.SuccessfulModels++
		metrics := candidate.AttackMetrics
		group.summary.ScoredSamples += metrics.ScoredSamples
		group.summary.AttackSamples += metrics.AttackSamples
		group.summary.HighImpactSamples += metrics.HighImpactSamples
		group.summary.IntrusionSamples += metrics.IntrusionSamples
		group.summary.DestructionSamples += metrics.DestructionSamples
		group.summary.ExfiltrationSamples += metrics.ExfiltrationSamples
		group.summary.PersistenceSamples += metrics.PersistenceSamples

		group.securityUtilityWeight += metrics.SecurityUtility * float64(metrics.ScoredSamples)
		group.attackDetected += metrics.AttackRecall * float64(metrics.AttackSamples)
		group.highImpactDetected += metrics.HighImpactRecall * float64(metrics.HighImpactSamples)
		group.intrusionDetected += metrics.IntrusionRecall * float64(metrics.IntrusionSamples)
		group.destructionDetected += metrics.DestructionRecall * float64(metrics.DestructionSamples)
		group.exfiltrationDetected += metrics.ExfiltrationRecall * float64(metrics.ExfiltrationSamples)
		group.persistenceDetected += metrics.PersistenceRecall * float64(metrics.PersistenceSamples)
		group.catastrophicMisses += metrics.CatastrophicMissRate * float64(metrics.HighImpactSamples)
		group.benignSamples += metrics.BenignSamples
		group.benignFalsePositives += metrics.BenignFalsePositive * float64(metrics.BenignSamples)

		if group.summary.BestModelType == "" || metrics.SecurityUtility > group.summary.BestModelSecurityUtility {
			group.summary.BestModelType = candidate.ModelType
			group.summary.BestModelLabel = candidate.Label
			group.summary.BestModelSecurityUtility = metrics.SecurityUtility
		}
	}

	out := make([]ModelSecurityGroupSummary, 0, len(groups))
	for _, group := range groups {
		summary := group.summary
		if summary.ScoredSamples > 0 {
			summary.MeanSecurityUtility = group.securityUtilityWeight / float64(summary.ScoredSamples)
		}
		if summary.AttackSamples > 0 {
			summary.AttackRecall = group.attackDetected / float64(summary.AttackSamples)
		}
		if summary.HighImpactSamples > 0 {
			summary.HighImpactRecall = group.highImpactDetected / float64(summary.HighImpactSamples)
			summary.CatastrophicMissRate = group.catastrophicMisses / float64(summary.HighImpactSamples)
		}
		if summary.IntrusionSamples > 0 {
			summary.IntrusionRecall = group.intrusionDetected / float64(summary.IntrusionSamples)
		}
		if summary.DestructionSamples > 0 {
			summary.DestructionRecall = group.destructionDetected / float64(summary.DestructionSamples)
		}
		if summary.ExfiltrationSamples > 0 {
			summary.ExfiltrationRecall = group.exfiltrationDetected / float64(summary.ExfiltrationSamples)
		}
		if summary.PersistenceSamples > 0 {
			summary.PersistenceRecall = group.persistenceDetected / float64(summary.PersistenceSamples)
		}
		if group.benignSamples > 0 {
			summary.BenignFalsePositiveRate = group.benignFalsePositives / float64(group.benignSamples)
		}
		out = append(out, summary)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].MeanSecurityUtility != out[j].MeanSecurityUtility {
			return out[i].MeanSecurityUtility > out[j].MeanSecurityUtility
		}
		if out[i].CatastrophicMissRate != out[j].CatastrophicMissRate {
			return out[i].CatastrophicMissRate < out[j].CatastrophicMissRate
		}
		return out[i].Key < out[j].Key
	})
	return out
}
