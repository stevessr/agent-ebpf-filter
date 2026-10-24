package ml

import (
	"math/rand"
	"sort"
)

// buildTree recursively builds a decision tree using Gini impurity
func buildTree(samples []TrainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) []DecisionNode {
	if depth >= maxDepth || len(samples) < minSamplesLeaf*2 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	allSame := true
	firstLabel := samples[0].label
	for _, s := range samples[1:] {
		if s.label != firstLabel {
			allSame = false
			break
		}
	}
	if allSame {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: float32(firstLabel)}}
	}

	best := findBestSplit(samples, minSamplesLeaf, featureSampleCount, rng)
	if best.giniGain <= 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	var leftSamples, rightSamples []TrainSample
	for _, s := range samples {
		if s.features[best.featureIdx] < best.threshold {
			leftSamples = append(leftSamples, s)
		} else {
			rightSamples = append(rightSamples, s)
		}
	}

	if len(leftSamples) == 0 || len(rightSamples) == 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	leftNodes := buildTree(leftSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)
	rightNodes := buildTree(rightSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)

	leftOffset := 1
	rightOffset := 1 + len(leftNodes)

	for i := range leftNodes {
		n := &leftNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(leftOffset)
			n.RightChild += int16(leftOffset)
		}
	}
	for i := range rightNodes {
		n := &rightNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(rightOffset)
			n.RightChild += int16(rightOffset)
		}
	}

	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    int16(leftOffset),
		RightChild:   int16(rightOffset),
		LeafValue:    0,
	}

	nodes := []DecisionNode{root}
	nodes = append(nodes, leftNodes...)
	nodes = append(nodes, rightNodes...)

	return nodes
}

// findBestSplit scans each sorted feature once, using incremental class
// histograms. It enforces minSamplesLeaf at split selection time rather
// than only at recursion entry (which formerly allowed singleton leaves).
func findBestSplit(samples []TrainSample, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) splitPoint {
	best := splitPoint{giniGain: -1}
	n := len(samples)
	if minSamplesLeaf < 1 {
		minSamplesLeaf = 1
	}
	if n < 2*minSamplesLeaf {
		return best
	}
	parentGini := giniImpurity(samples)
	features := make([]int, FeatureDim)
	for i := range features {
		features[i] = i
	}
	rng.Shuffle(len(features), func(i, j int) { features[i], features[j] = features[j], features[i] })
	if featureSampleCount > FeatureDim {
		featureSampleCount = FeatureDim
	}
	if featureSampleCount <= 0 {
		return best
	}
	sorted := make([]TrainSample, n)
	for _, fi := range features[:featureSampleCount] {
		copy(sorted, samples)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].features[fi] < sorted[j].features[fi]
		})
		var leftCounts, rightCounts [4]int
		for _, sample := range sorted {
			if sample.label >= 0 && sample.label < 4 {
				rightCounts[sample.label]++
			}
		}
		for i := 1; i < n; i++ {
			if label := sorted[i-1].label; label >= 0 && label < 4 {
				leftCounts[label]++
				rightCounts[label]--
			}
			if i < minSamplesLeaf || n-i < minSamplesLeaf ||
				sorted[i].features[fi] == sorted[i-1].features[fi] {
				continue
			}
			threshold := (sorted[i].features[fi] + sorted[i-1].features[fi]) / 2
			leftWeight := float64(i) / float64(n)
			gain := parentGini - leftWeight*giniFromCounts(leftCounts, i) -
				(1-leftWeight)*giniFromCounts(rightCounts, n-i)
			if gain > best.giniGain {
				best = splitPoint{featureIdx: fi, threshold: threshold, giniGain: gain}
			}
		}
	}
	return best
}

// giniImpurity computes Gini impurity from the four known action classes.
func giniImpurity(samples []TrainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var counts [4]int
	for _, s := range samples {
		if s.label >= 0 && s.label < 4 {
			counts[s.label]++
		}
	}
	return giniFromCounts(counts, len(samples))
}

func giniFromCounts(counts [4]int, n int) float64 {
	if n <= 0 {
		return 0
	}
	impurity := 1.0
	for _, count := range counts {
		p := float64(count) / float64(n)
		impurity -= p * p
	}
	return impurity
}

// majorityClass resolves equal votes deterministically in ALLOW/BLOCK/REWRITE/ALERT
// class order. Hash-map iteration is intentionally excluded from model outcomes.
func majorityClass(samples []TrainSample) float32 {
	var counts [4]int
	for _, s := range samples {
		if s.label >= 0 && s.label < 4 {
			counts[s.label]++
		}
	}
	best := 0
	for c := 1; c < len(counts); c++ {
		if counts[c] > counts[best] {
			best = c
		}
	}
	return float32(best)
}

// classStratifiedBootstrap gives each observed class approximately equal
// representation (within one row), oversampling rare classes rather than
// allowing majority ALLOW events to dominate every tree. Class ordering and
// the RNG stream are deterministic for reproducible experiments.
func classStratifiedBootstrap(src, dst []TrainSample, rng *rand.Rand) {
	var groups [4][]TrainSample
	for _, sample := range src {
		if sample.label >= 0 && sample.label < 4 {
			groups[sample.label] = append(groups[sample.label], sample)
		}
	}
	active := make([]int, 0, 4)
	for c := range groups {
		if len(groups[c]) > 0 {
			active = append(active, c)
		}
	}
	if len(active) == 0 || len(dst) == 0 {
		return
	}
	start := rng.Intn(len(active))
	for i := range dst {
		group := groups[active[(start+i)%len(active)]]
		dst[i] = group[rng.Intn(len(group))]
	}
	// Avoid a fixed class cycle in downstream tree-building.
	rng.Shuffle(len(dst), func(i, j int) { dst[i], dst[j] = dst[j], dst[i] })
}

func EvaluateForest(forest *DecisionForest, testSet []TrainSample) float64 {
	if len(testSet) == 0 {
		return 0 // zero evidence must not be reported as perfect accuracy
	}
	correct := 0
	for _, s := range testSet {
		pred := forest.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(testSet))
}
