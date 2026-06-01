package main

import (
	"math/rand"
	"sort"
)

// buildTree recursively builds a decision tree using Gini impurity
func buildTree(samples []trainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) []DecisionNode {
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

	best := findBestSplit(samples, featureSampleCount, rng)
	if best.giniGain <= 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	var leftSamples, rightSamples []trainSample
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

// findBestSplit finds the best feature and threshold using Gini impurity
func findBestSplit(samples []trainSample, featureSampleCount int, rng *rand.Rand) splitPoint {
	best := splitPoint{giniGain: -1}
	parentGini := giniImpurity(samples)

	features := make([]int, FeatureDim)
	for i := range features {
		features[i] = i
	}
	rng.Shuffle(len(features), func(i, j int) { features[i], features[j] = features[j], features[i] })
	selectedFeatures := features[:featureSampleCount]

	for _, fi := range selectedFeatures {
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].features[fi] < samples[j].features[fi]
		})

		for i := 1; i < len(samples); i++ {
			if samples[i].features[fi] == samples[i-1].features[fi] {
				continue
			}
			threshold := (samples[i].features[fi] + samples[i-1].features[fi]) / 2.0

			leftSamples := samples[:i]
			rightSamples := samples[i:]

			if len(leftSamples) < 1 || len(rightSamples) < 1 {
				continue
			}

			leftWeight := float64(len(leftSamples)) / float64(len(samples))
			gain := parentGini - leftWeight*giniImpurity(leftSamples) -
				(1-leftWeight)*giniImpurity(rightSamples)

			if gain > best.giniGain {
				best = splitPoint{
					featureIdx: fi,
					threshold:  threshold,
					giniGain:   gain,
				}
			}
		}
	}
	return best
}

// giniImpurity computes Gini impurity for a set of samples
func giniImpurity(samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	counts := make(map[int32]float64)
	for _, s := range samples {
		counts[s.label]++
	}
	var impurity float64
	n := float64(len(samples))
	for _, c := range counts {
		p := c / n
		impurity += p * (1 - p)
	}
	return impurity
}

// majorityClass returns the most common class label as float32
func majorityClass(samples []trainSample) float32 {
	if len(samples) == 0 {
		return 0
	}
	counts := make(map[int32]int)
	for _, s := range samples {
		counts[s.label]++
	}
	best := int32(0)
	bestCount := 0
	for label, count := range counts {
		if count > bestCount {
			bestCount = count
			best = label
		}
	}
	return float32(best)
}

// classStratifiedBootstrap creates a bootstrap sample where each class
// is proportionally represented. Minority classes are upsampled to ensure
// they appear in every tree's training set.
func classStratifiedBootstrap(src, dst []trainSample, rng *rand.Rand) {
	groups := make(map[int32][]trainSample)
	for _, s := range src {
		groups[s.label] = append(groups[s.label], s)
	}

	nClasses := len(groups)
	if nClasses == 0 {
		return
	}
	perClass := len(dst) / nClasses
	if perClass < 1 {
		perClass = 1
	}

	i := 0
	for i < len(dst) {
		for _, group := range groups {
			if i >= len(dst) {
				break
			}
			dst[i] = group[rng.Intn(len(group))]
			i++
		}
	}
}

func evaluateForest(forest *DecisionForest, testSet []trainSample) float64 {
	if len(testSet) == 0 {
		return 1.0
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
