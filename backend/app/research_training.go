package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/behavior"

	"github.com/gin-gonic/gin"
)

const (
	researchTrainingSchemaVersion   = "research-training.v1"
	researchTrainingPolicyHeuristic = "heuristic"
	researchTrainingPolicyDecision  = "decision"
	researchTrainingPolicyUnlabeled = "unlabeled"
)

type ResearchTrainingDataset struct {
	SchemaVersion string                     `json:"schemaVersion"`
	SessionID     string                     `json:"sessionId"`
	GeneratedAt   time.Time                  `json:"generatedAt"`
	LabelPolicy   string                     `json:"labelPolicy"`
	FeatureDim    int                        `json:"featureDim"`
	FeatureNames  []string                   `json:"featureNames"`
	SampleCount   int                        `json:"sampleCount"`
	LabeledCount  int                        `json:"labeledCount"`
	ByLabel       []researchCount            `json:"byLabel"`
	Normalization FeatureNormalizationReport `json:"normalization"`
	Samples       []ResearchTrainingSample   `json:"samples,omitempty"`
}

type ResearchTrainingSample struct {
	SampleID       string         `json:"sampleId"`
	EventID        string         `json:"eventId"`
	Timestamp      int64          `json:"timestamp"`
	Time           string         `json:"time"`
	Source         string         `json:"source"`
	EventType      string         `json:"eventType"`
	PID            uint32         `json:"pid,omitempty"`
	Comm           string         `json:"comm"`
	CommandLine    string         `json:"commandLine"`
	Args           []string       `json:"args"`
	Category       string         `json:"category"`
	Target         string         `json:"target,omitempty"`
	TraceID        string         `json:"traceId,omitempty"`
	SpanID         string         `json:"spanId,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	RiskScore      float64        `json:"riskScore,omitempty"`
	Label          int32          `json:"label"`
	LabelName      string         `json:"labelName"`
	LabelSource    string         `json:"labelSource"`
	AnomalyScore   float64        `json:"anomalyScore"`
	FeatureVector  []float64      `json:"featureVector"`
	FeatureSpace   string         `json:"featureSpace"`
	FeatureVersion string         `json:"featureVersion"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	trainingSample TrainingSample `json:"-"`
}

type ResearchTrainingImportResponse struct {
	SessionID       string                     `json:"sessionId"`
	LabelPolicy     string                     `json:"labelPolicy"`
	Total           int                        `json:"total"`
	Imported        int                        `json:"imported"`
	Skipped         int                        `json:"skipped"`
	TotalSamples    int                        `json:"totalSamples"`
	LabeledSamples  int                        `json:"labeledSamples"`
	Normalization   FeatureNormalizationReport `json:"normalization"`
	ImportedSamples []ResearchTrainingSample   `json:"importedSamples,omitempty"`
}

func buildResearchTrainingDataset(sessionID string, events []ResearchEvent, labelPolicy string, includeSamples bool) ResearchTrainingDataset {
	labelPolicy = normalizeResearchTrainingLabelPolicy(labelPolicy)
	trainingSamples := make([]ResearchTrainingSample, 0, len(events))
	mlSamples := make([]TrainingSample, 0, len(events))
	byLabel := map[string]int{}
	labeled := 0
	for _, event := range events {
		sample, ok := researchTrainingSampleFromEvent(event, labelPolicy)
		if !ok {
			continue
		}
		trainingSamples = append(trainingSamples, sample)
		mlSamples = append(mlSamples, sample.trainingSample)
		incrementResearchCount(byLabel, sample.LabelName)
		if sample.Label >= 0 && sample.Label <= 3 {
			labeled++
		}
	}
	dataset := ResearchTrainingDataset{
		SchemaVersion: researchTrainingSchemaVersion,
		SessionID:     sessionID,
		GeneratedAt:   time.Now().UTC(),
		LabelPolicy:   labelPolicy,
		FeatureDim:    FeatureDim,
		FeatureNames:  researchTrainingFeatureNames(),
		SampleCount:   len(trainingSamples),
		LabeledCount:  labeled,
		ByLabel:       topResearchCounts(byLabel, 0),
		Normalization: summarizeFeatureNormalization(mlSamples),
	}
	if includeSamples {
		dataset.Samples = trainingSamples
	}
	return dataset
}

func researchTrainingSampleFromEvent(event ResearchEvent, labelPolicy string) (ResearchTrainingSample, bool) {
	comm, args, commandLine := researchCommandPartsFromEvent(event)
	if strings.TrimSpace(comm) == "" {
		return ResearchTrainingSample{}, false
	}
	label, labelName, labelSource := researchTrainingLabelForEvent(event, labelPolicy)
	classification := behavior.ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, "", event.PID)
	timestamp := time.UnixMilli(event.Timestamp).UTC()
	if event.Timestamp <= 0 {
		timestamp = time.Now().UTC()
	}
	mlSample := TrainingSample{
		Features:     features,
		Label:        label,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         append([]string(nil), args...),
		Category:     classification.PrimaryCategory,
		AnomalyScore: anomalyScore,
		Timestamp:    timestamp,
		UserLabel:    labelSource,
	}
	if label < 0 {
		mlSample.UserLabel = ""
	}
	return ResearchTrainingSample{
		SampleID:       researchStableID("train", event.ID, commandLine, labelName),
		EventID:        event.ID,
		Timestamp:      timestamp.UnixMilli(),
		Time:           timestamp.Format(time.RFC3339Nano),
		Source:         event.Source,
		EventType:      event.EventType,
		PID:            event.PID,
		Comm:           comm,
		CommandLine:    commandLine,
		Args:           append([]string(nil), args...),
		Category:       classification.PrimaryCategory,
		Target:         event.Target,
		TraceID:        event.TraceID,
		SpanID:         event.SpanID,
		Decision:       event.Decision,
		RiskScore:      event.RiskScore,
		Label:          label,
		LabelName:      labelName,
		LabelSource:    labelSource,
		AnomalyScore:   anomalyScore,
		FeatureVector:  featureVectorSlice(features),
		FeatureSpace:   "agent-command-128-bounded-0-1",
		FeatureVersion: "feature-extractor.v1",
		Metadata: map[string]any{
			"redactionLevel": event.RedactionLevel,
			"source":         event.Source,
			"eventType":      event.EventType,
		},
		trainingSample: mlSample,
	}, true
}

func researchCommandPartsFromEvent(event ResearchEvent) (string, []string, string) {
	comm := strings.TrimSpace(event.Comm)
	commandLine := strings.TrimSpace(researchStringFromMap(event.Features, "commandLine"))
	if commandLine == "" {
		commandLine = strings.TrimSpace(researchStringFromMap(event.Features, "cmdline"))
	}
	if commandLine == "" {
		if strings.Contains(strings.ToLower(event.EventType), "wrapper") || strings.Contains(strings.ToLower(event.EventType), "hook") {
			commandLine = strings.TrimSpace(event.Target)
		}
	}
	if commandLine == "" && strings.TrimSpace(event.Target) != "" && researchTargetLooksCommandish(event) {
		commandLine = strings.TrimSpace(event.Target)
	}
	if commandLine == "" && comm != "" && strings.TrimSpace(event.Target) != "" {
		commandLine = joinCommandLine(comm, []string{event.Target})
	}
	if commandLine == "" {
		commandLine = comm
	}
	parsedComm, args := normalizeCommandInput(commandLine, comm, nil)
	if parsedComm != "" {
		comm = parsedComm
	}
	if len(args) == 0 && strings.TrimSpace(event.Target) != "" && !strings.EqualFold(strings.TrimSpace(event.Target), comm) {
		args = []string{event.Target}
		commandLine = joinCommandLine(comm, args)
	}
	if commandLine == "" {
		commandLine = joinCommandLine(comm, args)
	}
	return comm, args, commandLine
}

func researchTargetLooksCommandish(event ResearchEvent) bool {
	if strings.TrimSpace(event.Target) == "" {
		return false
	}
	typeText := strings.ToLower(event.EventType + " " + event.Source)
	if strings.Contains(typeText, "exec") || strings.Contains(typeText, "wrapper") || strings.Contains(typeText, "hook") || strings.Contains(typeText, "process") {
		return true
	}
	return strings.Contains(event.Target, " ") || strings.HasPrefix(event.Target, "/")
}

func researchTrainingLabelForEvent(event ResearchEvent, labelPolicy string) (int32, string, string) {
	labelPolicy = normalizeResearchTrainingLabelPolicy(labelPolicy)
	decision := strings.ToUpper(strings.TrimSpace(event.Decision))
	switch labelPolicy {
	case researchTrainingPolicyUnlabeled:
		return -1, "UNLABELED", "research-unlabeled"
	case researchTrainingPolicyDecision:
		if decision == "" {
			return -1, "UNLABELED", "research-decision-missing"
		}
		return actionFromLabel(decision), sampleLabelName(actionFromLabel(decision)), "research-decision"
	default:
		if decision != "" {
			label := actionFromLabel(decision)
			return label, sampleLabelName(label), "research-decision"
		}
		if event.RiskScore >= 90 || strings.Contains(strings.ToLower(event.EventType), "alert") {
			return actionFromLabel("ALERT"), "ALERT", "research-risk-heuristic"
		}
		return actionFromLabel("ALLOW"), "ALLOW", "research-low-risk-heuristic"
	}
}

func normalizeResearchTrainingLabelPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "decision", "decision_only":
		return researchTrainingPolicyDecision
	case "unlabeled", "none", "manual":
		return researchTrainingPolicyUnlabeled
	default:
		return researchTrainingPolicyHeuristic
	}
}

func featureVectorSlice(features [FeatureDim]float64) []float64 {
	out := make([]float64, FeatureDim)
	for i, value := range features {
		out[i] = value
	}
	return out
}

func researchTrainingFeatureNames() []string {
	names := make([]string, FeatureDim)
	for i := 0; i < FeatureDim; i++ {
		names[i] = fmt.Sprintf("feature_%03d", i)
	}
	known := map[int]string{
		15:  "is_shell",
		16:  "is_package_manager",
		17:  "is_agent_cli",
		18:  "is_root_user",
		19:  "has_network_args",
		20:  "has_file_args",
		21:  "has_redirection",
		22:  "has_pipe",
		23:  "many_args",
		24:  "dev_access",
		25:  "sudo_in_args",
		26:  "classification_high_confidence",
		27:  "classification_medium_confidence",
		28:  "comm_length_norm",
		29:  "args_count_norm",
		32:  "arg_mean_length_norm",
		33:  "arg_std_length_norm",
		34:  "arg_total_bytes_norm",
		35:  "arg_entropy",
		36:  "flag_count_norm",
		37:  "positional_count_norm",
		58:  "has_url_pattern",
		59:  "has_ip_pattern",
		60:  "redirect_count_norm",
		61:  "pipe_count_norm",
		116: "hour_sin_norm",
		117: "hour_cos_norm",
	}
	for index, name := range known {
		if index >= 0 && index < len(names) {
			names[index] = name
		}
	}
	return names
}

func researchTrainingDatasetJSONLBytes(dataset ResearchTrainingDataset) []byte {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, sample := range dataset.Samples {
		_ = encoder.Encode(sample)
	}
	return buf.Bytes()
}

func researchTrainingDatasetCSVBytes(dataset ResearchTrainingDataset) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"sample_id", "event_id", "timestamp", "time", "source", "event_type", "pid", "comm", "command_line", "category", "label", "label_name", "label_source", "risk_score", "anomaly_score", "trace_id", "span_id", "target"}
	for _, name := range dataset.FeatureNames {
		header = append(header, name)
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, sample := range dataset.Samples {
		row := []string{sample.SampleID, sample.EventID, strconv.FormatInt(sample.Timestamp, 10), sample.Time, sample.Source, sample.EventType, researchUint32String(sample.PID), sample.Comm, sample.CommandLine, sample.Category, strconv.FormatInt(int64(sample.Label), 10), sample.LabelName, sample.LabelSource, researchFloatString(sample.RiskScore), researchFloatString(sample.AnomalyScore), sample.TraceID, sample.SpanID, sample.Target}
		for _, value := range sample.FeatureVector {
			row = append(row, strconv.FormatFloat(value, 'f', -1, 64))
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func handleResearchSessionTraining(c *gin.Context) {
	sessionID := c.Param("id")
	events, err := researchSessionsStore.LoadEvents(sessionID)
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	labelPolicy := normalizeResearchTrainingLabelPolicy(c.Query("labelPolicy"))
	dataset := buildResearchTrainingDataset(sessionID, events, labelPolicy, true)
	switch normalizeResearchTrainingFormat(c.Query("format")) {
	case "jsonl":
		c.Data(http.StatusOK, "application/x-ndjson; charset=utf-8", researchTrainingDatasetJSONLBytes(dataset))
	case "csv":
		payload, err := researchTrainingDatasetCSVBytes(dataset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "text/csv; charset=utf-8", payload)
	default:
		c.JSON(http.StatusOK, dataset)
	}
}

func handleResearchSessionTrainingImport(c *gin.Context) {
	sessionID := c.Param("id")
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}
	events, err := researchSessionsStore.LoadEvents(sessionID)
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	var req struct {
		LabelPolicy string `json:"labelPolicy"`
		Limit       int    `json:"limit"`
		Preview     bool   `json:"preview"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid training import payload"})
			return
		}
	}
	labelPolicy := normalizeResearchTrainingLabelPolicy(req.LabelPolicy)
	if labelPolicy == researchTrainingPolicyHeuristic && strings.TrimSpace(req.LabelPolicy) == "" {
		labelPolicy = researchTrainingPolicyDecision
	}
	dataset := buildResearchTrainingDataset(sessionID, events, labelPolicy, true)
	limit := req.Limit
	if limit <= 0 || limit > len(dataset.Samples) {
		limit = len(dataset.Samples)
	}
	imported := 0
	skipped := 0
	importedSamples := make([]ResearchTrainingSample, 0)
	seen := map[string]struct{}{}
	for _, sample := range dataset.Samples[:limit] {
		if sample.Label < 0 || sample.Label > 3 {
			skipped++
			continue
		}
		key := commandKey(sample.trainingSample.Comm, sample.trainingSample.Args) + "\x00" + sample.LabelName
		if _, ok := seen[key]; ok {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if globalTrainingStore.HasExactCommand(sample.trainingSample.Comm, sample.trainingSample.Args) {
			skipped++
			continue
		}
		if req.Preview {
			importedSamples = append(importedSamples, sample)
			continue
		}
		globalTrainingStore.Add(sample.trainingSample)
		recordCommandSampleSideEffects(sample.trainingSample)
		imported++
		importedSamples = append(importedSamples, sample)
	}
	if !req.Preview {
		if err := globalTrainingStore.Flush(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	total, labeled := globalTrainingStore.Status()
	c.JSON(http.StatusOK, ResearchTrainingImportResponse{SessionID: sessionID, LabelPolicy: labelPolicy, Total: len(dataset.Samples), Imported: imported, Skipped: skipped, TotalSamples: total, LabeledSamples: labeled, Normalization: dataset.Normalization, ImportedSamples: importedSamples})
}

func normalizeResearchTrainingFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "jsonl", "ndjson":
		return "jsonl"
	case "csv":
		return "csv"
	default:
		return "json"
	}
}
