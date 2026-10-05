package domainforwardproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	nativeInferenceModelVersion = 1
	defaultInferenceDimension    = 256
	defaultInferenceMaxToken     = 256
	maxInferenceDimension        = 4096
	maxInferenceLabels           = 32
	maxInferenceTokenBytes       = 4096
)

type NativeInferenceSettings struct {
	Enabled       bool   `json:"enabled"`
	ModelFile     string `json:"modelFile,omitempty"`
	Direction     string `json:"direction,omitempty"`
	Host          string `json:"host,omitempty"`
	PathPrefix    string `json:"pathPrefix,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	MinTokenBytes int    `json:"minTokenBytes,omitempty"`
	MaxTokenBytes int    `json:"maxTokenBytes,omitempty"`
}

type nativeInferenceModelFile struct {
	Version   int                    `json:"version"`
	Dimension int                    `json:"dimension"`
	Seed      uint64                 `json:"seed,omitempty"`
	Labels    []nativeInferenceLabel `json:"labels"`
}

type nativeInferenceLabel struct {
	Name        string `json:"name"`
	Bias        int32  `json:"bias,omitempty"`
	Threshold   int32  `json:"threshold"`
	Replacement string `json:"replacement"`
	Weights     []int8 `json:"weights"`
}

type NativeInferenceKernel struct {
	dimension   int
	seed        uint64
	direction   string
	host        string
	pathPrefix  string
	contentType string
	minToken    int
	maxToken    int
	labels      []nativeInferenceLabel
}

func LoadNativeInferenceKernel(settings NativeInferenceSettings) (*NativeInferenceKernel, error) {
	if !settings.Enabled {
		return nil, nil
	}
	modelPath := strings.TrimSpace(settings.ModelFile)
	if modelPath == "" {
		return nil, errors.New("native rewrite inference requires modelFile")
	}
	payload, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("read native rewrite inference model: %w", err)
	}
	var model nativeInferenceModelFile
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, fmt.Errorf("parse native rewrite inference model: %w", err)
	}
	if model.Version != nativeInferenceModelVersion {
		return nil, fmt.Errorf("unsupported native rewrite inference model version %d", model.Version)
	}
	if model.Dimension <= 0 {
		model.Dimension = defaultInferenceDimension
	}
	if model.Dimension > maxInferenceDimension {
		return nil, fmt.Errorf("native rewrite inference dimension %d exceeds %d", model.Dimension, maxInferenceDimension)
	}
	if len(model.Labels) == 0 || len(model.Labels) > maxInferenceLabels {
		return nil, fmt.Errorf("native rewrite inference labels must contain 1-%d entries", maxInferenceLabels)
	}
	for i := range model.Labels {
		label := &model.Labels[i]
		label.Name = strings.TrimSpace(label.Name)
		if label.Name == "" {
			return nil, fmt.Errorf("native rewrite inference label %d has empty name", i)
		}
		if len(label.Weights) != model.Dimension {
			return nil, fmt.Errorf("native rewrite inference label %q has %d weights, want %d", label.Name, len(label.Weights), model.Dimension)
		}
		if label.Replacement == "" {
			return nil, fmt.Errorf("native rewrite inference label %q has empty replacement", label.Name)
		}
		if strings.ContainsAny(label.Replacement, "\"\\\r\n\t") {
			return nil, fmt.Errorf("native rewrite inference label %q replacement must be a JSON-safe token", label.Name)
		}
	}
	minToken := settings.MinTokenBytes
	if minToken <= 0 {
		minToken = 3
	}
	maxToken := settings.MaxTokenBytes
	if maxToken <= 0 {
		maxToken = defaultInferenceMaxToken
	}
	if maxToken > maxInferenceTokenBytes {
		maxToken = maxInferenceTokenBytes
	}
	if minToken > maxToken {
		return nil, errors.New("native rewrite inference minTokenBytes exceeds maxTokenBytes")
	}
	return &NativeInferenceKernel{
		dimension:   model.Dimension,
		seed:        model.Seed,
		direction:   normalizeRewriteDirection(settings.Direction),
		host:        NormalizeDomainPattern(settings.Host),
		pathPrefix:  strings.TrimSpace(settings.PathPrefix),
		contentType: strings.ToLower(strings.TrimSpace(settings.ContentType)),
		minToken:    minToken,
		maxToken:    maxToken,
		labels:      model.Labels,
	}, nil
}

func (k *NativeInferenceKernel) Matches(direction, host, path, contentType string) bool {
	if k == nil {
		return false
	}
	if k.direction != "both" && k.direction != direction {
		return false
	}
	if !rewriteHostMatches(k.host, host) {
		return false
	}
	if k.pathPrefix != "" && !strings.HasPrefix(path, k.pathPrefix) {
		return false
	}
	if k.contentType != "" && !strings.Contains(strings.ToLower(contentType), k.contentType) {
		return false
	}
	return true
}

func (k *NativeInferenceKernel) Rewrite(contentType string, body []byte) ([]byte, bool) {
	if k == nil || len(body) == 0 {
		return body, false
	}
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "json") || looksLikeJSONPayload(body) {
		return k.rewriteJSONStrings(body)
	}
	if strings.HasPrefix(lower, "text/") ||
		strings.Contains(lower, "xml") ||
		strings.Contains(lower, "x-www-form-urlencoded") {
		return k.rewriteText(body)
	}
	return body, false
}

func (k *NativeInferenceKernel) rewriteText(body []byte) ([]byte, bool) {
	return k.rewriteRange(body, 0, len(body))
}

func (k *NativeInferenceKernel) rewriteJSONStrings(body []byte) ([]byte, bool) {
	out := body
	changed := false
	offset := 0
	for offset < len(out) {
		start, end, isKey, ok := nextJSONString(out, offset)
		if !ok {
			break
		}
		offset = end
		if isKey || end-start <= 2 {
			continue
		}
		innerStart := start + 1
		innerEnd := end - 1
		rewritten, localChanged := k.rewriteRange(out, innerStart, innerEnd)
		if !localChanged {
			continue
		}
		out = rewritten
		changed = true
		offset = innerStart
		for offset < len(out) && out[offset] != '"' {
			offset++
		}
		if offset < len(out) {
			offset++
		}
	}
	return out, changed
}

func (k *NativeInferenceKernel) rewriteRange(body []byte, start, end int) ([]byte, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(body) {
		end = len(body)
	}
	if start >= end {
		return body, false
	}
	var out []byte
	cursor := start
	changed := false
	for cursor < end {
		for cursor < end && !isInferenceTokenByte(body[cursor]) {
			cursor++
		}
		tokenStart := cursor
		for cursor < end && isInferenceTokenByte(body[cursor]) {
			cursor++
		}
		tokenEnd := cursor
		tokenLen := tokenEnd - tokenStart
		if tokenLen < k.minToken || tokenLen > k.maxToken {
			continue
		}
		replacement, ok := k.classify(body[tokenStart:tokenEnd])
		if !ok {
			continue
		}
		if out == nil {
			out = make([]byte, 0, len(body)+32)
			out = append(out, body[:tokenStart]...)
		} else {
			out = append(out, body[start:tokenStart]...)
		}
		out = append(out, replacement...)
		body = body[tokenEnd:]
		end -= tokenEnd
		start = 0
		cursor = 0
		changed = true
	}
	if !changed {
		return body, false
	}
	out = append(out, body...)
	return out, true
}

func (k *NativeInferenceKernel) classify(token []byte) ([]byte, bool) {
	if k == nil || len(token) == 0 || len(k.labels) == 0 {
		return nil, false
	}
	var scores [maxInferenceLabels]int32
	for i := range k.labels {
		scores[i] = k.labels[i].Bias
	}
	for n := 1; n <= 3; n++ {
		if len(token) < n {
			break
		}
		for i := 0; i+n <= len(token); i++ {
			bucket := int(hashFeature(token[i:i+n], k.seed) % uint64(k.dimension))
			for labelIndex := range k.labels {
				scores[labelIndex] += int32(k.labels[labelIndex].Weights[bucket])
			}
		}
	}
	best := -1
	bestScore := int32(-1 << 31)
	for i := range k.labels {
		score := scores[i]
		if score < k.labels[i].Threshold {
			continue
		}
		if best < 0 || score > bestScore {
			best = i
			bestScore = score
		}
	}
	if best < 0 {
		return nil, false
	}
	return []byte(k.labels[best].Replacement), true
}

func hashFeature(feature []byte, seed uint64) uint64 {
	hash := uint64(1469598103934665603) ^ seed
	for _, value := range feature {
		hash ^= uint64(value)
		hash *= 1099511628211
	}
	return hash
}

func isInferenceTokenByte(value byte) bool {
	if value >= 0x80 {
		return true
	}
	switch {
	case value >= 'a' && value <= 'z',
		value >= 'A' && value <= 'Z',
		value >= '0' && value <= '9':
		return true
	}
	switch value {
	case '@', '.', '_', '-', ':', '/', '+', '=', '%':
		return true
	default:
		return false
	}
}

func looksLikeJSONPayload(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func nextJSONString(body []byte, from int) (start, end int, isKey bool, ok bool) {
	inString := false
	escaped := false
	start = -1
	for i := from; i < len(body); i++ {
		value := body[i]
		if !inString {
			if value == '"' {
				inString = true
				start = i
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		if value == '\\' {
			escaped = true
			continue
		}
		if value != '"' {
			continue
		}
		end = i + 1
		j := end
		for j < len(body) && isJSONSpace(body[j]) {
			j++
		}
		isKey = j < len(body) && body[j] == ':'
		return start, end, isKey, true
	}
	return 0, 0, false, false
}
