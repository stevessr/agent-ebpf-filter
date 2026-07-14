package app

import (
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

func normalizeTrainingStoreCapacity(maxSamples int) int {
	if maxSamples <= 0 {
		return 1
	}
	if maxSamples > trainingStoreMaxSamples {
		return trainingStoreMaxSamples
	}
	return maxSamples
}

const (
	trainingSampleMaxCommandLineBytes = 4096
	trainingSampleMaxCommBytes        = 512
	trainingSampleMaxArgs             = 64
	trainingSampleMaxArgBytes         = 512
	trainingSampleMaxArgsBytes        = 1024
	trainingSampleMaxArgsJSONBytes    = 2048
	trainingSampleMaxCategoryBytes    = 128
	trainingSampleMaxUserLabelBytes   = 128
)

var (
	trainingSampleMinTimestamp = time.Unix(0, -1<<63).UTC()
	trainingSampleMaxTimestamp = time.Unix(0, 1<<63-1).UTC()
)

func normalizeTrainingSample(sample TrainingSample) TrainingSample {
	sample.Comm = normalizeTrainingMetadata(sample.Comm, trainingSampleMaxCommBytes, true)
	sample.Args = normalizeTrainingArgs(sample.Args)
	sample.CommandLine = normalizeTrainingMetadata(sample.CommandLine, trainingSampleMaxCommandLineBytes, true)
	if sample.CommandLine == "" {
		sample.CommandLine = normalizeTrainingMetadata(joinCommandLine(sample.Comm, sample.Args), trainingSampleMaxCommandLineBytes, true)
	}
	sample.Category = normalizeTrainingMetadata(sample.Category, trainingSampleMaxCategoryBytes, true)
	sample.UserLabel = normalizeTrainingMetadata(sample.UserLabel, trainingSampleMaxUserLabelBytes, true)
	if sample.Label < -1 || sample.Label > 3 {
		sample.Label = -1
	}
	sample.AnomalyScore = normalizeTrainingScore(sample.AnomalyScore)
	for index, value := range sample.Features {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			sample.Features[index] = 0
		}
	}
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now().UTC()
	} else {
		sample.Timestamp = sample.Timestamp.UTC()
		if sample.Timestamp.Before(trainingSampleMinTimestamp) {
			sample.Timestamp = trainingSampleMinTimestamp
		} else if sample.Timestamp.After(trainingSampleMaxTimestamp) {
			sample.Timestamp = trainingSampleMaxTimestamp
		}
	}
	return sample
}

func cloneTrainingSample(sample TrainingSample) TrainingSample {
	sample.Args = append([]string(nil), sample.Args...)
	return sample
}

func normalizeTrainingArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	limit := len(args)
	if limit > trainingSampleMaxArgs {
		limit = trainingSampleMaxArgs
	}
	out := make([]string, 0, limit)
	total := 0
	for _, arg := range args[:limit] {
		arg = truncateTrainingUTF8(arg, trainingSampleMaxArgBytes)
		remaining := trainingSampleMaxArgsBytes - total
		if remaining <= 0 {
			break
		}
		arg = truncateTrainingUTF8(arg, remaining)
		out = append(out, strings.Clone(arg))
		total += len(arg)
	}
	return fitTrainingArgsJSON(out)
}

func fitTrainingArgsJSON(args []string) []string {
	for len(args) > 0 {
		encoded, err := json.Marshal(args)
		if err == nil && len(encoded) <= trainingSampleMaxArgsJSONBytes {
			return args
		}
		last := len(args) - 1
		if args[last] == "" {
			args = args[:last]
			continue
		}
		excess := len(encoded) - trainingSampleMaxArgsJSONBytes
		if excess < 1 {
			excess = 1
		}
		keepBytes := len(args[last]) - excess
		args[last] = truncateTrainingUTF8(args[last], keepBytes)
	}
	return nil
}

func normalizeTrainingMetadata(value string, maxBytes int, trimSpace bool) string {
	if trimSpace {
		value = strings.TrimSpace(value)
	}
	value = truncateTrainingUTF8(value, maxBytes)
	return strings.Clone(value)
}

func normalizeTrainingScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func truncateTrainingUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "")
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
