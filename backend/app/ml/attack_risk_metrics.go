package ml

import (
	"math"
	"strings"
)

// ThreatVector is a host-centric attack dimension used for evaluation and
// AutoML selection. It deliberately describes attacker impact rather than the
// classifier implementation so the same metrics can compare every model family.
type ThreatVector string

const (
	ThreatIntrusion    ThreatVector = "intrusion"
	ThreatDestruction  ThreatVector = "destruction"
	ThreatExfiltration ThreatVector = "exfiltration"
	ThreatPersistence  ThreatVector = "persistence"
)

var threatVectors = []ThreatVector{
	ThreatIntrusion,
	ThreatDestruction,
	ThreatExfiltration,
	ThreatPersistence,
}

var featureCategoryNames = [15]string{
	"UNKNOWN",
	"FILE_READ",
	"FILE_WRITE",
	"FILE_DELETE",
	"FILE_PERMISSION",
	"NETWORK",
	"PROCESS_EXEC",
	"PROCESS_KILL",
	"SYSTEM_INFO",
	"PACKAGE_MANAGER",
	"DATABASE",
	"COMPRESSION",
	"DEVELOPMENT",
	"CONTAINER",
	"SENSITIVE",
}

// AttackImpactMetrics makes false negatives on destructive/high-impact samples
// substantially more visible than ordinary classification errors. This mirrors
// a security-oriented design where a global risk objective is evaluated beside
// attack-vector-specific objectives instead of optimizing aggregate accuracy.
type AttackImpactMetrics struct {
	ScoredSamples        int     `json:"scoredSamples"`
	BenignSamples        int     `json:"benignSamples"`
	AttackSamples        int     `json:"attackSamples"`
	HighImpactSamples    int     `json:"highImpactSamples"`
	IntrusionSamples     int     `json:"intrusionSamples"`
	DestructionSamples   int     `json:"destructionSamples"`
	ExfiltrationSamples  int     `json:"exfiltrationSamples"`
	PersistenceSamples   int     `json:"persistenceSamples"`
	AttackRecall         float64 `json:"attackRecall"`
	HighImpactRecall     float64 `json:"highImpactRecall"`
	IntrusionRecall      float64 `json:"intrusionRecall"`
	DestructionRecall    float64 `json:"destructionRecall"`
	ExfiltrationRecall   float64 `json:"exfiltrationRecall"`
	PersistenceRecall    float64 `json:"persistenceRecall"`
	CatastrophicMissRate float64 `json:"catastrophicMissRate"`
	BenignFalsePositive  float64 `json:"benignFalsePositiveRate"`
	RiskWeightedRecall   float64 `json:"riskWeightedRecall"`
	SecurityUtility      float64 `json:"securityUtility"`
	ThreatVectorCoverage float64 `json:"threatVectorCoverage"`
}

type attackMetricAccumulator struct {
	total    int
	detected int
}

func (a attackMetricAccumulator) recall() float64 {
	if a.total == 0 {
		return 0
	}
	return float64(a.detected) / float64(a.total)
}

type attackImpactDescriptor struct {
	Features   [FeatureDim]float64
	Label      int32
	HighImpact bool
	Weight     float64
	Vectors    []ThreatVector
}

// EvaluateAttackImpactMetrics evaluates full training samples. Command/category
// metadata is used when available, which makes this the richest evaluator for
// cross-model selection and offline reporting.
func EvaluateAttackImpactMetrics(samples []TrainingSample, model Model) AttackImpactMetrics {
	if model == nil {
		return AttackImpactMetrics{}
	}
	descriptors := make([]attackImpactDescriptor, 0, len(samples))
	for _, sample := range samples {
		if sample.Label < 0 || sample.Label > 3 {
			continue
		}
		descriptors = append(descriptors, attackImpactDescriptor{
			Features:   sample.Features,
			Label:      sample.Label,
			HighImpact: isHighImpactSample(sample),
			Weight:     sampleImpactWeight(sample),
			Vectors:    threatVectorsForSample(sample),
		})
	}
	return evaluateAttackImpactDescriptors(descriptors, func(features [FeatureDim]float64) int32 {
		return model.Predict(features).Action
	})
}

// EvaluateAttackImpactTrainSamples is the lightweight grid-search evaluator.
// TrainSample intentionally carries only features+label, so attack semantics are
// recovered from the stable feature contract: category one-hot [0,14] and
// network-audit evidence [120,127]. No command parsing or extra I/O is required.
func EvaluateAttackImpactTrainSamples(samples []TrainSample, predict func([FeatureDim]float64) int32) AttackImpactMetrics {
	if len(samples) == 0 || predict == nil {
		return AttackImpactMetrics{}
	}
	descriptors := make([]attackImpactDescriptor, 0, len(samples))
	for _, sample := range samples {
		if sample.label < 0 || sample.label > 3 {
			continue
		}
		descriptors = append(descriptors, attackImpactDescriptor{
			Features:   sample.features,
			Label:      sample.label,
			HighImpact: featureHighImpact(sample.features, sample.label),
			Weight:     featureImpactWeight(sample.features, sample.label),
			Vectors:    threatVectorsFromFeatures(sample.features),
		})
	}
	return evaluateAttackImpactDescriptors(descriptors, predict)
}

func evaluateAttackImpactMetrics(samples []TrainingSample, predict func([FeatureDim]float64) int32) AttackImpactMetrics {
	if predict == nil {
		return AttackImpactMetrics{}
	}
	descriptors := make([]attackImpactDescriptor, 0, len(samples))
	for _, sample := range samples {
		if sample.Label < 0 || sample.Label > 3 {
			continue
		}
		descriptors = append(descriptors, attackImpactDescriptor{
			Features:   sample.Features,
			Label:      sample.Label,
			HighImpact: isHighImpactSample(sample),
			Weight:     sampleImpactWeight(sample),
			Vectors:    threatVectorsForSample(sample),
		})
	}
	return evaluateAttackImpactDescriptors(descriptors, predict)
}

func evaluateAttackImpactDescriptors(samples []attackImpactDescriptor, predict func([FeatureDim]float64) int32) AttackImpactMetrics {
	if len(samples) == 0 || predict == nil {
		return AttackImpactMetrics{}
	}

	var out AttackImpactMetrics
	var attack, high attackMetricAccumulator
	vectorStats := map[ThreatVector]*attackMetricAccumulator{}
	for _, vector := range threatVectors {
		vectorStats[vector] = &attackMetricAccumulator{}
	}

	benignFalsePositive := 0
	catastrophicMisses := 0
	weightedDetected := 0.0
	weightedTotal := 0.0
	observedVectors := map[ThreatVector]bool{}
	cost := 0.0
	maxCost := 0.0

	for _, sample := range samples {
		out.ScoredSamples++
		predicted := predict(sample.Features)
		if !isAttackLabel(sample.Label) {
			out.BenignSamples++
			// A false BLOCK is the most disruptive benign mistake. ALERT is less
			// disruptive, while REWRITE still changes program semantics and is not free.
			maxCost += 2.5
			switch predicted {
			case 1:
				benignFalsePositive++
				cost += 2.5
			case 3:
				benignFalsePositive++
				cost += 1.25
			case 2:
				benignFalsePositive++
				cost += 0.5
			}
			continue
		}

		attack.total++
		out.AttackSamples++
		severity := math.Max(sample.Weight, 1)
		weightedTotal += severity
		maxCost += severity

		credit := defensiveActionCredit(predicted)
		if isDefensiveAction(predicted) {
			attack.detected++
		}
		weightedDetected += severity * credit
		cost += severity * (1.0 - credit)

		for _, vector := range sample.Vectors {
			observedVectors[vector] = true
			stats := vectorStats[vector]
			stats.total++
			if isDefensiveAction(predicted) {
				stats.detected++
			}
		}

		if sample.HighImpact {
			high.total++
			out.HighImpactSamples++
			// High-impact recall intentionally requires a strong response. A
			// REWRITE still earns partial utility through defensiveActionCredit,
			// but does not hide weak containment behind a binary recall metric.
			if isStrongDefensiveAction(predicted) {
				high.detected++
			}
			if predicted == 0 {
				catastrophicMisses++
			}
		}
	}

	out.IntrusionSamples = vectorStats[ThreatIntrusion].total
	out.DestructionSamples = vectorStats[ThreatDestruction].total
	out.ExfiltrationSamples = vectorStats[ThreatExfiltration].total
	out.PersistenceSamples = vectorStats[ThreatPersistence].total
	out.AttackRecall = attack.recall()
	out.HighImpactRecall = high.recall()
	out.IntrusionRecall = vectorStats[ThreatIntrusion].recall()
	out.DestructionRecall = vectorStats[ThreatDestruction].recall()
	out.ExfiltrationRecall = vectorStats[ThreatExfiltration].recall()
	out.PersistenceRecall = vectorStats[ThreatPersistence].recall()
	if high.total > 0 {
		out.CatastrophicMissRate = float64(catastrophicMisses) / float64(high.total)
	}
	if out.BenignSamples > 0 {
		out.BenignFalsePositive = float64(benignFalsePositive) / float64(out.BenignSamples)
	}
	if weightedTotal > 0 {
		out.RiskWeightedRecall = weightedDetected / weightedTotal
	}
	if maxCost > 0 {
		out.SecurityUtility = clampUnit(1.0 - cost/maxCost)
		// Catastrophic ALLOW decisions should dominate small aggregate gains.
		// Keep the utility smooth for AutoML while imposing a strong monotonic
		// penalty instead of an opaque hard-coded candidate rejection threshold.
		if high.total > 0 {
			out.SecurityUtility *= 1.0 - 0.65*out.CatastrophicMissRate
			out.SecurityUtility = clampUnit(out.SecurityUtility)
		}
	}
	out.ThreatVectorCoverage = float64(len(observedVectors)) / float64(len(threatVectors))
	return out
}

// REWRITE is included because the ground truth says the original command is
// unsafe enough to require intervention, even if it does not require blocking.
func isAttackLabel(label int32) bool {
	return label == 1 || label == 2 || label == 3
}

func isDefensiveAction(action int32) bool {
	return action == 1 || action == 2 || action == 3
}

func isStrongDefensiveAction(action int32) bool {
	return action == 1 || action == 3
}

func defensiveActionCredit(action int32) float64 {
	switch action {
	case 1: // BLOCK
		return 1.0
	case 3: // ALERT: strong detection but not guaranteed prevention.
		return 0.88
	case 2: // REWRITE: useful mitigation, weaker for active compromise.
		return 0.55
	default: // ALLOW
		return 0
	}
}

func isHighImpactSample(sample TrainingSample) bool {
	category := strings.ToUpper(strings.TrimSpace(sample.Category))
	lower := sampleCommandText(sample)

	switch category {
	case "SENSITIVE", "FILE_DELETE", "PROCESS_KILL":
		return true
	case "FILE_PERMISSION":
		if containsAny(lower, "chmod ", "chown ", "setcap ", "/etc/sudoers", "authorized_keys") {
			return true
		}
	case "NETWORK":
		if containsAny(lower, "nc -e", "/dev/tcp/", "curl -d @", "curl --data @", "wget --post-file", "dnscat", "iodine") {
			return true
		}
	}

	return containsAny(lower,
		"/etc/shadow", "~/.ssh", ".ssh/", "~/.aws", ".aws/credentials",
		"rm -rf", "shred ", "mkfs", "wipefs", "dd if=", "iptables -f",
		"ufw disable", "firewall-cmd --remove", "nc -e", "/dev/tcp/",
		"systemctl enable", "crontab", "authorized_keys",
	)
}

func featureHighImpact(features [FeatureDim]float64, label int32) bool {
	category := featureCategory(features)
	switch category {
	case "SENSITIVE", "FILE_DELETE", "PROCESS_KILL":
		return true
	case "FILE_PERMISSION":
		return label == 1
	}
	// Stable network feature contract: reverse shell / explicit exfiltration /
	// DNS tunnel are high-consequence evidence. Generic NETWORK is not.
	return featureEnabled(features, 122) || featureEnabled(features, 123) || featureEnabled(features, 124)
}

// sampleImpactWeight is deliberately asymmetric. A missed destructive or
// credential/privilege sample costs far more than a routine false positive.
func sampleImpactWeight(sample TrainingSample) float64 {
	weight := categoryImpactWeight(strings.ToUpper(strings.TrimSpace(sample.Category)))
	if isHighImpactSample(sample) {
		weight = math.Max(weight, 12)
	}
	// An anomaly score is contextual evidence, not a label. It may increase the
	// consequence weight modestly but cannot create a threat vector by itself.
	weight *= 1.0 + 0.20*clampUnit(sample.AnomalyScore)
	return weight
}

func featureImpactWeight(features [FeatureDim]float64, label int32) float64 {
	weight := categoryImpactWeight(featureCategory(features))
	if featureHighImpact(features, label) {
		weight = math.Max(weight, 12)
	}
	// Network risk score [120] may strengthen consequence weighting but never
	// creates a vector by itself.
	if FeatureDim > 120 {
		weight *= 1.0 + 0.15*clampUnit(features[120])
	}
	return weight
}

func categoryImpactWeight(category string) float64 {
	switch category {
	case "FILE_DELETE", "PROCESS_KILL":
		return 12
	case "SENSITIVE":
		return 11
	case "FILE_PERMISSION":
		return 9
	case "NETWORK":
		return 8
	case "PROCESS_EXEC", "FILE_WRITE":
		return 7
	case "CONTAINER", "DATABASE":
		return 6
	case "PACKAGE_MANAGER", "COMPRESSION":
		return 5.5
	default:
		return 5
	}
}

func threatVectorsForSample(sample TrainingSample) []ThreatVector {
	seen := map[ThreatVector]bool{}
	add := func(v ThreatVector) { seen[v] = true }
	category := strings.ToUpper(strings.TrimSpace(sample.Category))
	lower := sampleCommandText(sample)

	switch category {
	case "SENSITIVE", "FILE_PERMISSION":
		add(ThreatIntrusion)
	case "FILE_DELETE", "PROCESS_KILL":
		add(ThreatDestruction)
	case "PROCESS_EXEC", "CONTAINER":
		add(ThreatIntrusion)
	case "PACKAGE_MANAGER":
		add(ThreatPersistence)
	case "FILE_WRITE":
		add(ThreatDestruction)
	}

	// NETWORK and DATABASE alone are intentionally not equated with exfiltration.
	// Specific evidence determines whether activity is intrusion, exfiltration,
	// persistence/C2, destruction, or simply remains global-only.
	if containsAny(lower,
		"sudo ", "su ", "pkexec", "/etc/shadow", "~/.ssh", ".ssh/",
		"nsenter", "setcap ", "nmap ", "masscan", "nc -z", "nc -e", "/dev/tcp/",
		"iptables -f", "ufw disable", "firewall-cmd --remove",
	) {
		add(ThreatIntrusion)
	}
	if containsAny(lower,
		"rm -rf", "shred ", "wipefs", "mkfs", "> /dev/", "kill -9",
		"pkill ", "truncate -s 0", "dd if=",
	) {
		add(ThreatDestruction)
	}
	if containsAny(lower,
		"curl -d @", "curl --data @", "wget --post-file", "nc <", "dnscat",
		"iodine", "scp ", "rsync ", "rclone ", "mysqldump", "pg_dump",
	) {
		add(ThreatExfiltration)
	}
	if containsAny(lower,
		"crontab", "/etc/cron", "systemctl enable", "systemctl --user enable",
		"authorized_keys", ".config/autostart", "rc.local", "nc -e", "/dev/tcp/",
		"dnscat", "iodine",
	) {
		add(ThreatPersistence)
	}

	return orderedThreatVectors(seen)
}

func threatVectorsFromFeatures(features [FeatureDim]float64) []ThreatVector {
	seen := map[ThreatVector]bool{}
	add := func(v ThreatVector) { seen[v] = true }

	switch featureCategory(features) {
	case "SENSITIVE", "FILE_PERMISSION", "PROCESS_EXEC", "CONTAINER":
		add(ThreatIntrusion)
	case "FILE_DELETE", "PROCESS_KILL", "FILE_WRITE":
		add(ThreatDestruction)
	case "PACKAGE_MANAGER":
		add(ThreatPersistence)
	}

	if featureEnabled(features, 122) { // reverse shell
		add(ThreatIntrusion)
		add(ThreatPersistence)
	}
	if featureEnabled(features, 123) { // explicit data exfiltration
		add(ThreatExfiltration)
	}
	if featureEnabled(features, 124) { // DNS tunnel
		add(ThreatExfiltration)
		add(ThreatPersistence)
	}
	if featureEnabled(features, 127) { // port scan
		add(ThreatIntrusion)
	}
	if featureEnabled(features, 121) { // suspicious/C2 port
		add(ThreatIntrusion)
		add(ThreatPersistence)
	}
	if featureEnabled(features, 126) { // unusual target
		add(ThreatExfiltration)
	}
	return orderedThreatVectors(seen)
}

func orderedThreatVectors(seen map[ThreatVector]bool) []ThreatVector {
	out := make([]ThreatVector, 0, len(seen))
	for _, vector := range threatVectors {
		if seen[vector] {
			out = append(out, vector)
		}
	}
	return out
}

func featureCategory(features [FeatureDim]float64) string {
	bestIndex := 0
	bestValue := 0.0
	limit := len(featureCategoryNames)
	if FeatureDim < limit {
		limit = FeatureDim
	}
	for i := 0; i < limit; i++ {
		if features[i] > bestValue {
			bestIndex = i
			bestValue = features[i]
		}
	}
	if bestValue <= 0 {
		return "UNKNOWN"
	}
	return featureCategoryNames[bestIndex]
}

func featureEnabled(features [FeatureDim]float64, index int) bool {
	return index >= 0 && index < FeatureDim && features[index] >= 0.5
}

func sampleCommandText(sample TrainingSample) string {
	parts := make([]string, 0, len(sample.Args)+2)
	parts = append(parts, sample.Comm, sample.CommandLine)
	parts = append(parts, sample.Args...)
	return strings.ToLower(strings.Join(parts, " "))
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func clampUnit(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
