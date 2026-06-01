package main

import (
	"encoding/json"
	"os"
)

type sweepCoverage struct {
	Summary  map[string]any         `json:"summary"`
	Datasets []map[string]any       `json:"datasets"`
	Profiles []sweepCoverageProfile `json:"profiles"`
}

type sweepCoverageProfile struct {
	Dataset                  string `json:"dataset"`
	Profile                  string `json:"profile"`
	ModelType                string `json:"modelType"`
	Parameter                string `json:"parameter"`
	ParameterKind            string `json:"parameterKind"`
	RequiredDiscretePoints   int    `json:"requiredDiscretePoints"`
	ConfiguredDiscretePoints int    `json:"configuredDiscretePoints"`
	TestedRows               int    `json:"testedRows"`
	Passed                   bool   `json:"passed"`
}

func buildSweepCoverage(datasets []sweepDataset, profiles []sweepProfile, results []sweepResult, pointsPerParam int) sweepCoverage {
	rowCounts := make(map[string]int)
	for _, result := range results {
		rowCounts[result.Profile]++
	}
	entries := make([]sweepCoverageProfile, 0, len(datasets)*len(profiles))
	passed := 0
	required := 0
	for _, dataset := range datasets {
		for _, profile := range profiles {
			scoped := profileForDataset(profile, dataset)
			configured := configuredProfilePointCount(profile)
			req := profile.RequiredDiscretePoints
			if req < 1 {
				req = configured
			}
			ok := rowCounts[scoped.Name] >= configured && configured >= req
			if profile.ParameterKind == "categorical" || profile.ParameterKind == "fixed" {
				ok = rowCounts[scoped.Name] >= configured && configured == req
			}
			required++
			if ok {
				passed++
			}
			entries = append(entries, sweepCoverageProfile{
				Dataset:                  dataset.Name,
				Profile:                  scoped.Name,
				ModelType:                string(profile.ModelType),
				Parameter:                profile.ParameterName,
				ParameterKind:            profile.ParameterKind,
				RequiredDiscretePoints:   req,
				ConfiguredDiscretePoints: configured,
				TestedRows:               rowCounts[scoped.Name],
				Passed:                   ok,
			})
		}
	}
	datasetRows := make([]map[string]any, 0, len(datasets))
	for _, dataset := range datasets {
		datasetRows = append(datasetRows, map[string]any{
			"name":        dataset.Name,
			"description": dataset.Description,
			"samples":     len(dataset.Samples),
		})
	}
	return sweepCoverage{
		Summary: map[string]any{
			"datasets":               len(datasets),
			"profiles":               len(profiles),
			"coverageEntries":        required,
			"passedEntries":          passed,
			"pointsPerParam":         pointsPerParam,
			"numericRequirementNote": "numeric comprehensive axis profiles require at least pointsPerParam unique tested values per tunable parameter; categorical/fixed profiles enumerate all meaningful values",
		},
		Datasets: datasetRows,
		Profiles: entries,
	}
}

func writeCoverageJSON(path string, coverage sweepCoverage) error {
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func uniqueIntCount(values []int) int {
	if len(values) == 0 {
		return 0
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	return len(seen)
}
