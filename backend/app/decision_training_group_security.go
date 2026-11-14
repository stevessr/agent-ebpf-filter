package app

// decisionGroupOutcome retains only the action decision and independent
// scenario/run component ID; raw commands never enter metric aggregation.
type decisionGroupOutcome struct {
	group     int
	actual    int32
	predicted int32
}

type decisionGroupSecurity struct {
	independentGroups int
	blockGroups       int
	benignGroups      int
	blockRecall       float64
	blockRecallLower95 float64
	benignFPR         float64
	benignFPRUpper95  float64
}

// summarizeDecisionGroupSecurity reports strict per-component risk:
// a BLOCK group succeeds only if every BLOCK command is blocked, while a
// benign group fails if any ALLOW command is not allowed. Mixed components
// contribute independently to both denominators. Wilson bounds apply to
// components rather than correlated rows; they still require independently
// sourced scenarios for external validity.
func summarizeDecisionGroupSecurity(outcomes []decisionGroupOutcome) decisionGroupSecurity {
	type state struct {
		hasBlock   bool
		hasAllow   bool
		blockMiss  bool
		benignFP   bool
	}
	byGroup := make(map[int]state)
	for _, outcome := range outcomes {
		group := byGroup[outcome.group]
		switch outcome.actual {
		case 0:
			group.hasAllow = true
			if outcome.predicted != 0 {
				group.benignFP = true
			}
		case 1:
			group.hasBlock = true
			if outcome.predicted != 1 {
				group.blockMiss = true
			}
		}
		byGroup[outcome.group] = group
	}
	result := decisionGroupSecurity{independentGroups: len(byGroup)}
	blockSuccesses, benignFailures := 0, 0
	for _, group := range byGroup {
		if group.hasBlock {
			result.blockGroups++
			if !group.blockMiss {
				blockSuccesses++
			}
		}
		if group.hasAllow {
			result.benignGroups++
			if group.benignFP {
				benignFailures++
			}
		}
	}
	if result.blockGroups != 0 {
		result.blockRecall = float64(blockSuccesses) / float64(result.blockGroups)
		result.blockRecallLower95, _ = wilson95(blockSuccesses, result.blockGroups)
	}
	if result.benignGroups != 0 {
		result.benignFPR = float64(benignFailures) / float64(result.benignGroups)
		_, result.benignFPRUpper95 = wilson95(benignFailures, result.benignGroups)
	}
	return result
}
