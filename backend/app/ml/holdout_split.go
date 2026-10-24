package ml

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// HoldoutSplitSeed is fixed so AutoTune candidate comparisons observe the
// same samples even when other goroutines use math/rand.
const HoldoutSplitSeed int64 = 0x484f4c444f5554

type holdoutGroup struct {
	key     [32]byte
	indices []int
	counts  [4]int
}

// prepareGroupedHoldout keeps duplicate behavior signatures in one partition.
// This is an event-level, command/feature-aware holdout, NOT a substitute for
// run-disjoint or time-forward validation when run identities are available.
func prepareGroupedHoldout(labeled []TrainingSample, ratio float64) ([]TrainSample, []TrainSample, []TrainingSample, []TrainingSample, error) {
	if len(labeled) < 2 {
		return nil, nil, nil, nil, errors.New("need at least 2 labeled samples")
	}
	if ratio <= 0 || ratio >= 0.5 {
		ratio = 0.20
	}

	groupsByKey := make(map[[32]byte]*holdoutGroup, len(labeled))
	groups := make([]*holdoutGroup, 0)
	var classTotals [4]int
	for i, sample := range labeled {
		if sample.Label < 0 || sample.Label > 3 {
			return nil, nil, nil, nil, errors.New("holdout split needs labels in [0,3]")
		}
		label := int(sample.Label)
		classTotals[label]++
		key := behaviorHoldoutKey(sample)
		group := groupsByKey[key]
		if group == nil {
			group = &holdoutGroup{key: key}
			groupsByKey[key] = group
			groups = append(groups, group)
		}
		group.indices = append(group.indices, i)
		group.counts[label]++
	}
	if len(groups) < 2 {
		return nil, nil, nil, nil, errors.New("holdout needs at least two independent behavior groups")
	}

	// Stable ordering plus a local PRNG ensures that input iteration order or
	// unrelated training jobs cannot reshuffle the holdout membership.
	sort.Slice(groups, func(i, j int) bool {
		return bytes.Compare(groups[i].key[:], groups[j].key[:]) < 0
	})
	rng := rand.New(rand.NewSource(HoldoutSplitSeed))
	rng.Shuffle(len(groups), func(i, j int) { groups[i], groups[j] = groups[j], groups[i] })

	validationTarget := int(math.Round(float64(len(labeled)) * ratio))
	if validationTarget < 1 {
		validationTarget = 1
	}
	if validationTarget >= len(labeled) {
		validationTarget = len(labeled) - 1
	}
	var quotas [4]int
	for c, count := range classTotals {
		quotas[c] = int(math.Round(float64(count) * ratio))
		if count > 1 && quotas[c] == 0 {
			quotas[c] = 1
		}
		if quotas[c] >= count && count > 0 {
			quotas[c] = count - 1
		}
	}

	selected := make(map[[32]byte]bool)
	var validationClassCount [4]int
	validationCount := 0
	// First try to meet class-wise targets. Group boundaries take precedence
	// over the exact row count so repeated observations cannot contaminate eval.
	for _, group := range groups {
		if validationCount >= validationTarget {
			break
		}
		needed := false
		for c, count := range group.counts {
			if count > 0 && validationClassCount[c] < quotas[c] {
				needed = true
			}
			// Always retain at least one training sample of each seen class.
			if count > 0 && classTotals[c]-validationClassCount[c]-count < 1 {
				needed = false
				break
			}
		}
		if !needed {
			continue
		}
		next := validationCount + len(group.indices)
		if next > validationTarget && next-validationTarget > validationTarget-validationCount {
			continue
		}
		selected[group.key] = true
		validationCount = next
		for c, count := range group.counts {
			validationClassCount[c] += count
		}
	}
	// When low-frequency groups prevent class-quotas from filling the target,
	// use other independent groups while keeping every class represented in train.
	for _, group := range groups {
		if validationCount >= validationTarget {
			break
		}
		if selected[group.key] {
			continue
		}
		canSelect := true
		for c, count := range group.counts {
			if count > 0 && classTotals[c]-validationClassCount[c]-count < 1 {
				canSelect = false
				break
			}
		}
		if !canSelect {
			continue
		}
		next := validationCount + len(group.indices)
		if next > validationTarget && next-validationTarget > validationTarget-validationCount {
			continue
		}
		selected[group.key] = true
		validationCount = next
		for c, count := range group.counts {
			validationClassCount[c] += count
		}
	}
	if validationCount == 0 || validationCount >= len(labeled) {
		return nil, nil, nil, nil, errors.New("cannot make independent validation holdout while retaining training classes")
	}

	train := make([]TrainSample, 0, len(labeled)-validationCount)
	validation := make([]TrainSample, 0, validationCount)
	trainRaw := make([]TrainingSample, 0, len(labeled)-validationCount)
	validationRaw := make([]TrainingSample, 0, validationCount)
	for _, sample := range labeled {
		small := TrainSample{features: sample.Features, label: sample.Label}
		if selected[behaviorHoldoutKey(sample)] {
			validation = append(validation, small)
			validationRaw = append(validationRaw, sample)
		} else {
			train = append(train, small)
			trainRaw = append(trainRaw, sample)
		}
	}
	return train, validation, trainRaw, validationRaw, nil
}

// Keys are digests rather than retained command strings to bound key storage
// and avoid keeping raw tool arguments in partition metadata.
func behaviorHoldoutKey(sample TrainingSample) [32]byte {
	h := sha256.New()
	if command := strings.TrimSpace(sample.CommandLine); command != "" {
		writeHoldoutString(h, "command")
		writeHoldoutString(h, command)
	} else if len(sample.Args) > 0 {
		writeHoldoutString(h, "argv")
		writeHoldoutString(h, sample.Comm)
		for _, arg := range sample.Args {
			writeHoldoutString(h, arg)
		}
	} else {
		writeHoldoutString(h, "features")
		var buf [8]byte
		for _, feature := range sample.Features {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(feature))
			_, _ = h.Write(buf[:])
		}
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

func writeHoldoutString(h hash.Hash, s string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(s)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(s))
}
