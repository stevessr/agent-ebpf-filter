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

// AttackImpactMetrics makes false negatives on destructive/high-impact samples
// substantially more visible than ordinary classification errors. This mirrors
// a security-oriented design where a global risk objective is evaluated beside
// attack-vector-specific objectives instead of optimizing aggregate accuracy.
type AttackImpactMetrics struct {
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

// EvaluateAttackImpactMetrics evaluates labeled samples with asymmetric costs:
// missing an attack is worse than over-alerting, and missing destructive or
// credential/privilege behavior is the most expensive error.
func EvaluateAttackImpactMetrics(samples []TrainingSample, model Model) AttackImpactMetrics {
	if model == nil {
		return AttackImpactMetrics{}
	}
	return evaluateAttackImpactMetrics(samples, func(features [FeatureDim]float64) int32 {
		return model.Predict(features).Action
	})
}

func evaluateAttackImpactMetrics(samples []TrainingSample, predict func([FeatureDim]float64) int32) AttackImpactMetrics {
	if len(samples) == 0 || predict == nil {
		return AttackImpactMetrics{}
	}

	var out AttackImpactMetrics
	var attack, high attackMetricAccumulator
	vectorStats := map[ThreatVector]*attackMetricAccumulator{}
	for _, vector := range threatVectors {
		vectorStats[vector] = &attackMetricAccumulator{}
	}

	benignTotal := 0
	benignFalsePositive := 0
	catastrophicMisses := 0
	weightedDetected := 0.0
	weightedTotal := 0.0
	observedVectors := map[ThreatVector]bool{}
	cost := 0.0
	maxCost := 0.0

	for _, sample := range samples {
		if sample.Label < 0 || sample.Label > 3 {
			continue
		}
		predicted := predict(sample.Features)
		isAttack := isAttackLabel(sample.Label)
		detected := isDefensiveAction(predicted)

		if !isAttack {
			benignTotal++
			maxCost += 2.0
			switch predicted {
			case 1: // BLOCK on benign traffic is the most expensive false positive.
				benignFalsePositive++
				cost += 2.0
			case 3: // ALERT is less disruptive, but still a false positive.
				benignFalsePositive++
				cost += 1.0
			case 2: // REWRITE changes behavior; account for it without equating it to BLOCK.
				benignFalsePositive++
				cost += 0.6
			}
			continue
		}

		attack.total++
		out.AttackSamples++
		severity := sampleImpactWeight(sample)
		weightedTotal += severity
		maxCost += severity
		if detected {
			attack.detected++
			weightedDetected += severity
		} else {
			cost += severity
		}

		vectors := threatVectorsForSample(sample)
		for _, vector := range vectors {
			observedVectors[vector] = true
			stats := vectorStats[vector]
			stats.total++
			if detected {
				stats.detected++
			}
		}

		if isHighImpactSample(sample) {
			high.total++
			out.HighImpactSamples++
			// For high-impact behavior, BLOCK/ALERT is considered a strong
			// defensive response. REWRITE is useful but should not earn full
			// high-impact recall.
			if predicted == 1 || predicted == 3 {
				high.detected++
			}
			if predicted == 0 {
				catastrophicMisses++
			}
			if predicted == 2 {
				cost += severity * 0.35
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
	if benignTotal > 0 {
		out.BenignFalsePositive = float64(benignFalsePositive) / float64(benignTotal)
	}
	if weightedTotal > 0 {
		out.RiskWeightedRecall = weightedDetected / weightedTotal
	}
	if maxCost > 0 {
		out.SecurityUtility = clampUnit(1.0 - cost/maxCost)
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

// sampleImpactWeight is deliberately asymmetric. A missed destructive or
// credential/privilege sample costs far more than a routine false positive.
func sampleImpactWeight(sample TrainingSample) float64 {
	weight := 5.0
	category := strings.ToUpper(strings.TrimSpace(sample.Category))
	switch category {
	case "FILE_DELETE", "PROCESS_KILL":
		weight = 12
	case "SENSITIVE":
		weight = 11
	case "FILE_PERMISSION":
		weight = 9
	case "NETWORK":
		weight = 8
	case "PROCESS_EXEC", "FILE_WRITE":
		weight = 7
	case "CONTAINER", "DATABASE":
		weight = 6
	case "PACKAGE_MANAGER", "COMPRESSION":
		weight = 5.5
	}
	if isHighImpactSample(sample) {
		weight = math.Max(weight, 12)
	}
	// An anomaly score is contextual evidence, not a label. It may increase the
	// consequence weight modestly but cannot create a threat vector by itself.
	weight *= 1.0 + 0.20*clampUnit(sample.AnomalyScore)
	return weight
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
	case "PROCESS_EXEC", "PACKAGE_MANAGER", "CONTAINER":
		add(ThreatPersistence)
	case "FILE_WRITE":
		add(ThreatDestruction)
		add(ThreatPersistence)
	case "DATABASE":
		add(ThreatExfiltration)
	}

	// NETWORK alone is intentionally not equated with exfiltration. Specific
	// evidence determines whether network behavior is intrusion, exfiltration,
	// persistence/C2, or simply remains global-only.
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
		"iodine", "scp ", "rsync ", "rclone ",
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

	out := make([]ThreatVector, 0, len(seen))
	for _, vector := range threatVectors {
		if seen[vector] {
			out = append(out, vector)
		}
	}
	if len(out) == 0 && isAttackLabel(sample.Label) {
		// Unknown malicious behavior still counts toward global attack recall,
		// but is intentionally not fabricated into a specific vector.
		return nil
	}
	return out
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
