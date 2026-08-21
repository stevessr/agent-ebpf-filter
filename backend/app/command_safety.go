package app

import (
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section command_safety.go ----

type commandSafetyRequest struct {
	CommandLine string   `json:"commandLine"`
	Comm        string   `json:"comm"`
	Args        []string `json:"args"`
	User        string   `json:"user"`
	PID         uint32   `json:"pid"`
}

type commandSampleMatch struct {
	Index        int      `json:"index"`
	CommandLine  string   `json:"commandLine"`
	Comm         string   `json:"comm"`
	Args         []string `json:"args"`
	Label        string   `json:"label"`
	Category     string   `json:"category"`
	AnomalyScore float64  `json:"anomalyScore"`
	Timestamp    string   `json:"timestamp"`
	UserLabel    string   `json:"userLabel"`
}

type existingCommandCandidate struct {
	CommandLine string    `json:"commandLine"`
	Comm        string    `json:"comm"`
	Args        []string  `json:"args"`
	EventType   string    `json:"eventType"`
	Source      string    `json:"source"`
	Category    string    `json:"category"`
	Timestamp   string    `json:"timestamp"`
	Duplicate   bool      `json:"duplicate"`
	eventTime   time.Time `json:"-"`
}

func cmdsafetyAssessPost(c *gin.Context) {
	req, ok := bindCommandSafetyRequest(c)
	if !ok {
		return
	}

	result := assessCommandSafety(c.Request.Context(), req.Comm, req.Args, req.User, req.PID)
	if strings.TrimSpace(req.CommandLine) != "" {
		result["commandLine"] = req.CommandLine
	}
	c.JSON(http.StatusOK, result)
}

func cmdsafetyExistingCommandsGet(c *gin.Context) {
	limit := parseCommandDataLimit(c.Query("limit"))
	candidates, source, err := existingCommandCandidates(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	duplicates := 0
	for _, candidate := range candidates {
		if candidate.Duplicate {
			duplicates++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"source":     source,
		"limit":      limit,
		"total":      len(candidates),
		"duplicates": duplicates,
		"candidates": candidates,
	})
}

func cmdsafetyImportExistingPost(c *gin.Context) {
	var req struct {
		Limit     int    `json:"limit"`
		LabelMode string `json:"labelMode"`
	}
	if status, err := bindOptionalLLMJSON(c, &req, 16<<10); err != nil {
		c.JSON(status, gin.H{"error": "invalid request"})
		return
	}

	if ml.GlobalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	limit := parseCommandDataLimit(strconv.Itoa(req.Limit))
	labelMode := strings.ToLower(strings.TrimSpace(req.LabelMode))
	if labelMode == "" {
		labelMode = "unlabeled"
	}
	if labelMode != "unlabeled" && labelMode != "heuristic" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "labelMode must be unlabeled or heuristic"})
		return
	}

	candidates, source, err := existingCommandCandidates(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	imported := 0
	skipped := 0
	for _, candidate := range candidates {
		if c.Request.Context().Err() != nil {
			return
		}
		if candidate.Comm == "" || ml.GlobalTrainingStore.HasExactCommand(candidate.Comm, candidate.Args) {
			skipped++
			continue
		}

		label := int32(-1)
		userLabel := ""
		if labelMode == "heuristic" {
			assessment := assessCommandSafetyWithOptions(c.Request.Context(), candidate.Comm, candidate.Args, "", 0, commandSafetyAssessmentOptions{IncludeLLM: false})
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = ml.ActionFromLabel(action)
				userLabel = "import-heuristic"
			}
		}

		sample := buildCommandTrainingSample(candidate.Comm, candidate.Args, "", 0, label, userLabel, candidate.eventTime)
		ml.GlobalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}

	if err := ml.GlobalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "imported samples but failed to persist: " + err.Error()})
		return
	}

	total, labeled := ml.GlobalTrainingStore.Status()
	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"source":          source,
		"labelMode":       labelMode,
		"totalCandidates": len(candidates),
		"imported":        imported,
		"skipped":         skipped,
		"totalSamples":    total,
		"labeledSamples":  labeled,
	})
}

func bindCommandSafetyRequest(c *gin.Context) (commandSafetyRequest, bool) {
	var req commandSafetyRequest
	if status, err := bindLLMJSON(c, &req, maxLLMRequestBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": "invalid request"})
		return req, false
	}

	rawCommandLine := strings.TrimSpace(req.CommandLine)
	comm, args := behavior.NormalizeCommandInput(rawCommandLine, req.Comm, req.Args)
	if comm == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commandLine or comm is required"})
		return req, false
	}

	req.Comm = comm
	req.Args = args
	if rawCommandLine != "" {
		req.CommandLine = rawCommandLine
	} else {
		req.CommandLine = behavior.JoinCommandLine(comm, args)
	}
	if err := validateLLMCommandInput(req.CommandLine, req.Comm, req.Args); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return req, false
	}
	return req, true
}

type commandSafetyAssessmentOptions struct {
	IncludeLLM bool
}

func assessCommandSafety(ctx context.Context, comm string, args []string, user string, pid uint32) gin.H {
	return assessCommandSafetyWithOptions(ctx, comm, args, user, pid, commandSafetyAssessmentOptions{IncludeLLM: true})
}

func assessCommandSafetyWithOptions(ctx context.Context, comm string, args []string, user string, pid uint32, opts commandSafetyAssessmentOptions) gin.H {
	if ctx == nil {
		ctx = context.Background()
	}
	commandLine := behavior.JoinCommandLine(comm, args)

	classification := behavior.ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, user, pid)

	mlRuntime := ml.SnapshotMLRuntime()
	var mlPrediction ml.Prediction
	if mlRuntime.Enabled && mlRuntime.ModelLoaded && mlRuntime.Engine != nil {
		mlPrediction = mlRuntime.Engine.Predict(features)
	}

	simulatedAction, reason := resolveAction(
		&pb.WrapperRequest{Comm: comm, Args: args, User: user, Pid: pid},
		"", 0,
		classification, anomalyScore, mlPrediction, mlRuntime.Config, mlRuntime.Enabled, mlRuntime.ModelLoaded,
	)
	if strings.TrimSpace(reason) == "" {
		reason = "No blocking policy matched"
	}

	netAudit := AuditNetworkBehavior(comm, strings.Join(args, " "))
	riskScore := computeRiskScore(classification, anomalyScore, mlPrediction, netAudit, nil)
	recommendedAction := ml.ActionLabel[int32(simulatedAction)]

	var matches []ml.IndexedTrainingSample
	if ml.GlobalTrainingStore != nil {
		matches = ml.GlobalTrainingStore.ExactMatches(comm, args)
	}

	sampleMatches := sampleMatchesJSON(matches)
	sampleEvidence := summarizeSampleEvidence(matches)
	sampleEvidenceSummary := fmt.Sprintf(
		"matches=%v labeled=%v decision=%v confidence=%.2f",
		sampleEvidence["totalMatches"], sampleEvidence["labeledMatches"], sampleEvidence["decision"], sampleEvidence["confidence"],
	)
	var llmResult *llmAssessment
	if opts.IncludeLLM && llmScoringConfigured() {
		llmReq := llmScoreRequest{
			CommandLine:    commandLine,
			Comm:           comm,
			Args:           append([]string(nil), args...),
			Category:       classification.PrimaryCategory,
			AnomalyScore:   anomalyScore,
			Classification: classification,
			MlAction:       ml.ActionLabel[mlPrediction.Action],
			MlConfidence:   mlPrediction.Confidence,
			NetworkRisk:    netAudit.RiskLevel,
			NetworkScore:   netAudit.RiskScore,
			SampleEvidence: sampleEvidenceSummary,
			CurrentLabel:   fmt.Sprint(sampleEvidence["decision"]),
			Source:         "assessment",
		}
		if result, err := scoreBehaviorWithLLM(ctx, llmReq); err != nil {
			llmResult = llmAssessmentFromScore(nil, err)
		} else {
			llmResult = llmAssessmentFromScore(result, nil)
			riskScore = computeRiskScore(classification, anomalyScore, mlPrediction, netAudit, llmResult)
		}
	} else {
		riskScore = computeRiskScore(classification, anomalyScore, mlPrediction, netAudit, nil)
	}

	recommendedAction, reason, riskScore = applySampleEvidence(recommendedAction, reason, riskScore, sampleEvidence)

	return gin.H{
		"commandLine":       commandLine,
		"comm":              comm,
		"args":              args,
		"classification":    classification,
		"anomalyScore":      anomalyScore,
		"mlPrediction":      gin.H{"action": ml.ActionLabel[mlPrediction.Action], "confidence": mlPrediction.Confidence},
		"recommendedAction": recommendedAction,
		"reasoning":         reason,
		"riskScore":         riskScore,
		"riskLevel":         riskLevel(riskScore),
		"networkAudit":      netAudit,
		"sampleMatches":     sampleMatches,
		"sampleEvidence":    sampleEvidence,
		"llmAssessment":     llmResult,
		"modelLoaded":       mlRuntime.ModelLoaded,
		"mlEnabled":         mlRuntime.Enabled,
	}
}

func existingCommandCandidates(limit int) ([]existingCommandCandidate, string, error) {
	records, source, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		return nil, source, err
	}

	seen := make(map[string]struct{})
	candidates := make([]existingCommandCandidate, 0, len(records))
	for _, record := range records {
		candidate, ok := commandCandidateFromRecord(record, source)
		if !ok {
			continue
		}
		key := behavior.CommandKey(candidate.Comm, candidate.Args)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if ml.GlobalTrainingStore != nil {
			candidate.Duplicate = ml.GlobalTrainingStore.HasExactCommand(candidate.Comm, candidate.Args)
		}
		candidates = append(candidates, candidate)
	}

	return candidates, source, nil
}

func commandCandidateFromRecord(record CapturedEventRecord, source string) (existingCommandCandidate, bool) {
	if record.Event == nil {
		return existingCommandCandidate{}, false
	}
	event := record.Event
	eventType := strings.TrimSpace(event.Type)
	if eventType == "" {
		switch event.EventType {
		case pb.EventType_WRAPPER_INTERCEPT:
			eventType = "wrapper_intercept"
		case pb.EventType_NATIVE_HOOK:
			eventType = "native_hook"
		}
	}

	commandLine := ""
	switch eventType {
	case "wrapper_intercept":
		commandLine = strings.TrimSpace(event.Path)
		if commandLine == "" {
			commandLine = strings.TrimSpace(event.Comm)
		}
	case "native_hook":
		commandLine = strings.TrimSpace(event.Path)
	default:
		return existingCommandCandidate{}, false
	}
	if commandLine == "" {
		return existingCommandCandidate{}, false
	}

	comm, args := behavior.NormalizeCommandInput(commandLine, event.Comm, nil)
	if comm == "" {
		return existingCommandCandidate{}, false
	}
	category := ""
	if event.Behavior != nil {
		category = event.Behavior.PrimaryCategory
	}
	eventTime := record.ReceivedAt
	if eventTime.IsZero() {
		eventTime = time.Now()
	}

	return existingCommandCandidate{
		CommandLine: behavior.JoinCommandLine(comm, args),
		Comm:        comm,
		Args:        args,
		EventType:   eventType,
		Source:      source,
		Category:    category,
		Timestamp:   eventTime.Format(time.RFC3339),
		eventTime:   eventTime,
	}, true
}

func buildCommandTrainingSample(comm string, args []string, user string, pid uint32, label int32, userLabel string, timestamp time.Time) ml.TrainingSample {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	classification := behavior.ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, user, pid)

	return ml.TrainingSample{
		Features:     features,
		Label:        label,
		CommandLine:  behavior.JoinCommandLine(comm, args),
		Comm:         comm,
		Args:         args,
		Category:     classification.PrimaryCategory,
		AnomalyScore: anomalyScore,
		Timestamp:    timestamp,
		UserLabel:    userLabel,
	}
}

func recordCommandSampleSideEffects(sample ml.TrainingSample) {
	_, emb := globalEmbedder.ClassifyAndEmbed(sample.Comm, sample.Args)
	globalEmbedder.AddToCluster(emb)

	action := sampleLabelName(sample.Label)
	if action == "-" {
		action = "UNLABELED"
	}
	globalFeatureExtractor.AddHistory(sample.Comm, sample.Category, action, sample.AnomalyScore, 0, "", len(strings.Join(sample.Args, " ")), len(sample.Args))
}

func trainingSampleCommandLine(sample ml.TrainingSample) string {
	if strings.TrimSpace(sample.CommandLine) != "" {
		return strings.TrimSpace(sample.CommandLine)
	}
	return behavior.JoinCommandLine(sample.Comm, sample.Args)
}

func sampleMatchesJSON(matches []ml.IndexedTrainingSample) []commandSampleMatch {
	out := make([]commandSampleMatch, 0, len(matches))
	for _, match := range matches {
		sample := match.Sample
		out = append(out, commandSampleMatch{
			Index:        match.Index,
			CommandLine:  trainingSampleCommandLine(sample),
			Comm:         sample.Comm,
			Args:         sample.Args,
			Label:        sampleLabelName(sample.Label),
			Category:     sample.Category,
			AnomalyScore: sample.AnomalyScore,
			Timestamp:    sample.Timestamp.Format(time.RFC3339),
			UserLabel:    sample.UserLabel,
		})
	}
	return out
}

func summarizeSampleEvidence(matches []ml.IndexedTrainingSample) gin.H {
	labelCounts := map[string]int{}
	labeledMatches := 0
	for _, match := range matches {
		if match.Sample.Label < 0 {
			continue
		}
		label := sampleLabelName(match.Sample.Label)
		labelCounts[label]++
		labeledMatches++
	}

	bestLabel := ""
	bestCount := 0
	for _, label := range []string{"BLOCK", "ALERT", "REWRITE", "ALLOW"} {
		if labelCounts[label] > bestCount {
			bestLabel = label
			bestCount = labelCounts[label]
		}
	}

	confidence := 0.0
	if labeledMatches > 0 {
		confidence = float64(bestCount) / float64(labeledMatches)
	}

	return gin.H{
		"totalMatches":   len(matches),
		"labeledMatches": labeledMatches,
		"labelCounts":    labelCounts,
		"decision":       bestLabel,
		"confidence":     confidence,
	}
}

func applySampleEvidence(action string, reason string, riskScore float64, evidence gin.H) (string, string, float64) {
	decision, _ := evidence["decision"].(string)
	if decision == "" {
		return action, reason, riskScore
	}

	confidence, _ := evidence["confidence"].(float64)
	prefix := "Existing labeled data"
	if confidence > 0 {
		prefix = prefix + " (" + strconv.Itoa(int(confidence*100+0.5)) + "% exact-match confidence)"
	}

	switch decision {
	case "BLOCK":
		action = "BLOCK"
		riskScore = maxFloat(riskScore, 90)
		reason = prefix + " recommends BLOCK; " + reason
	case "ALERT":
		action = "ALERT"
		riskScore = maxFloat(riskScore, 70)
		reason = prefix + " recommends ALERT; " + reason
	case "REWRITE":
		action = "REWRITE"
		riskScore = maxFloat(riskScore, 50)
		reason = prefix + " recommends REWRITE; " + reason
	case "ALLOW":
		if action == "ALLOW" {
			reason = prefix + " agrees with ALLOW; " + reason
		} else {
			reason = prefix + " has ALLOW samples, but heuristic/ML risk remains elevated; " + reason
		}
	}

	if riskScore > 100 {
		riskScore = 100
	}
	return action, reason, riskScore
}

func sampleLabelName(label int32) string {
	if label < 0 {
		return "-"
	}
	if name, ok := ml.ActionLabel[label]; ok {
		return name
	}
	return "-"
}

func parseCommandDataLimit(raw string) int {
	limit := 200
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit < 10 {
		return 10
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
