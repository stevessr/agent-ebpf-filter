package app

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func researchEventsJSONLBytes(events []ResearchEvent) []byte {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		_ = encoder.Encode(event)
	}
	return buf.Bytes()
}

func researchEventsCSVBytes(events []ResearchEvent) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	headers := []string{"id", "timestamp", "time", "source", "event_type", "pid", "ppid", "comm", "trace_id", "span_id", "target", "risk_score", "decision", "redaction_level"}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, event := range events {
		row := []string{event.ID, strconv.FormatInt(event.Timestamp, 10), event.Time, event.Source, event.EventType, researchUint32String(event.PID), researchUint32String(event.PPID), event.Comm, event.TraceID, event.SpanID, event.Target, researchFloatString(event.RiskScore), event.Decision, event.RedactionLevel}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func researchBundleZipBytes(session ResearchSession, events []ResearchEvent, results ResearchResults, settings ResearchProcessingSettings) ([]byte, error) {
	var buf bytes.Buffer
	zipw := zip.NewWriter(&buf)
	jsonl := researchEventsJSONLBytes(events)
	csvBytes, err := researchEventsCSVBytes(events)
	if err != nil {
		return nil, err
	}
	training := buildResearchTrainingDataset(session.ID, events, researchTrainingPolicyHeuristic, true)
	trainingJSONL := researchTrainingDatasetJSONLBytes(training)
	trainingCSV, err := researchTrainingDatasetCSVBytes(training)
	if err != nil {
		return nil, err
	}
	trainingManifestJSON, err := json.MarshalIndent(struct {
		SchemaVersion   string                     `json:"schemaVersion"`
		LabelPolicy     string                     `json:"labelPolicy"`
		FeatureSpace    string                     `json:"featureSpace"`
		FeatureVersion  string                     `json:"featureVersion"`
		FeatureDim      int                        `json:"featureDim"`
		FeatureNames    []string                   `json:"featureNames"`
		RedactionLevels []researchCount            `json:"redactionLevels"`
		SampleCount     int                        `json:"sampleCount"`
		LabeledCount    int                        `json:"labeledCount"`
		ByLabel         []researchCount            `json:"byLabel"`
		ByCategory      []researchCount            `json:"byCategory"`
		BySource        []researchCount            `json:"bySource"`
		Normalization   FeatureNormalizationReport `json:"normalization"`
		Quality         DatasetQualitySummary      `json:"quality"`
	}{
		SchemaVersion:   training.SchemaVersion,
		LabelPolicy:     training.LabelPolicy,
		FeatureSpace:    "agent-command-128-bounded-0-1",
		FeatureVersion:  "feature-extractor.v1",
		FeatureDim:      training.FeatureDim,
		FeatureNames:    training.FeatureNames,
		RedactionLevels: researchRedactionLevelCounts(events),
		SampleCount:     training.SampleCount,
		LabeledCount:    training.LabeledCount,
		ByLabel:         training.ByLabel,
		ByCategory:      training.ByCategory,
		BySource:        training.BySource,
		Normalization:   training.Normalization,
		Quality:         training.Quality,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	resultsJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return nil, err
	}
	sessionJSON, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, err
	}
	payloads := map[string][]byte{"events.jsonl": jsonl, "events.csv": csvBytes, "training.jsonl": trainingJSONL, "training.csv": trainingCSV, "training-manifest.json": trainingManifestJSON, "results.json": resultsJSON, "session.json": sessionJSON}
	if results.SecurityEvaluation != nil {
		securityJSON, err := researchSecurityEvaluationJSONBytes(results.SecurityEvaluation)
		if err != nil {
			return nil, err
		}
		securityJSONL := researchSecurityEvaluationJSONLBytes(results.SecurityEvaluation)
		securityCSV, err := researchSecurityEvaluationCSVBytes(results.SecurityEvaluation)
		if err != nil {
			return nil, err
		}
		payloads["security-evaluation.json"] = securityJSON
		payloads["security-evaluation.jsonl"] = securityJSONL
		payloads["security-evaluation.csv"] = securityCSV
	}
	manifest := researchBuildManifest(session, events, settings, payloads, nil)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files := payloads
	files["manifest.json"] = manifestJSON
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zipw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := zipw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func researchBuildManifest(session ResearchSession, events []ResearchEvent, settings ResearchProcessingSettings, payloads map[string][]byte, artifacts map[string]ResearchArtifactRef) ResearchManifest {
	hashes := map[string]string{}
	for name, payload := range payloads {
		hashes[name] = researchSHA256Hex(payload)
	}
	return ResearchManifest{SchemaVersion: researchManifestVersion, GeneratedAt: time.Now().UTC(), SessionID: session.ID, SessionName: session.Name, SourceFilter: session.SourceFilter, TimeRange: session.TimeRange, RedactionLevel: researchSessionRedactionLevel(events), EventCount: len(events), Artifacts: artifacts, Hashes: hashes, ResearchSchema: researchSchemaVersion, ExportedBy: "agent-ebpf-filter", RetentionDays: settings.ArtifactRetentionDays, MaxSessionEvents: settings.MaxSessionEvents, ConfiguredFormats: splitResearchFormats(settings.ExportFormats)}
}

func researchSessionRedactionLevel(events []ResearchEvent) string {
	if len(events) == 0 {
		return "metadata_only"
	}
	levels := map[string]struct{}{}
	for _, event := range events {
		if event.RedactionLevel != "" {
			levels[event.RedactionLevel] = struct{}{}
		}
	}
	if len(levels) == 0 {
		return "metadata_only"
	}
	items := make([]string, 0, len(levels))
	for level := range levels {
		items = append(items, level)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func researchSHA256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func researchUint32String(value uint32) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(value), 10)
}

func researchFloatString(value float64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func researchRiskAssociation(event ResearchEvent) string {
	if event.TraceID != "" {
		return "trace:" + event.TraceID
	}
	if event.PID != 0 {
		return fmt.Sprintf("pid:%d", event.PID)
	}
	if event.Comm != "" {
		return "comm:" + event.Comm
	}
	return ""
}

func researchSafeFeatureMap(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || researchZeroValue(value) {
			continue
		}
		if researchSensitiveFeatureKey(trimmed) {
			out[trimmed] = "[redacted]"
			continue
		}
		out[trimmed] = researchSafeFeatureValue(value)
	}
	return out
}

func researchSafeFeatureValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return researchSafeFeatureMap(typed)
	case map[string]string:
		m := make(map[string]any, len(typed))
		for key, value := range typed {
			m[key] = value
		}
		return researchSafeFeatureMap(m)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, researchSafeFeatureValue(item))
		}
		return out
	default:
		return typed
	}
}

func researchSensitiveFeatureKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	sensitive := []string{"authorization", "cookie", "token", "secret", "password", "credential", "api_key", "apikey", "x-api-key", "body", "raw", "rawhexdump", "raw_hex_dump", "payload", "headers", "cmdline", "argv", "args"}
	for _, marker := range sensitive {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func researchZeroValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case uint32:
		return typed == 0
	case uint64:
		return typed == 0
	case float64:
		return typed == 0
	case bool:
		return !typed
	case []string:
		return len(typed) == 0
	}
	return false
}

func cloneResearchSession(session *ResearchSession) ResearchSession {
	if session == nil {
		return ResearchSession{}
	}
	out := *session
	out.Tags = append([]string(nil), session.Tags...)
	out.ArtifactRefs = cloneArtifactRefs(session.ArtifactRefs)
	return out
}

func cloneArtifactRefs(in map[string]ResearchArtifactRef) map[string]ResearchArtifactRef {
	out := map[string]ResearchArtifactRef{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func researchStringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func researchFloatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func researchGenerateID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}

func researchStableID(prefix string, parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		payload, err := json.Marshal(part)
		if err != nil {
			payload = []byte(fmt.Sprint(part))
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))[:20]
}

func ptrTime(t time.Time) *time.Time { return &t }
