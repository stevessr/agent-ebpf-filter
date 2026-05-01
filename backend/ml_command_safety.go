package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

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

func handleMLAssessPost(c *gin.Context) {
	req, ok := bindCommandSafetyRequest(c)
	if !ok {
		return
	}

	result := assessCommandSafety(c.Request.Context(), req.Comm, req.Args, req.User, req.PID)
	c.JSON(http.StatusOK, result)
}

func handleMLExistingCommandsGet(c *gin.Context) {
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

func handleMLImportExistingPost(c *gin.Context) {
	var req struct {
		Limit     int    `json:"limit"`
		LabelMode string `json:"labelMode"`
	}
	_ = c.ShouldBindJSON(&req)

	if globalTrainingStore == nil {
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
		if candidate.Comm == "" || globalTrainingStore.HasExactCommand(candidate.Comm, candidate.Args) {
			skipped++
			continue
		}

		label := int32(-1)
		userLabel := ""
		if labelMode == "heuristic" {
			assessment := assessCommandSafety(context.Background(), candidate.Comm, candidate.Args, "", 0)
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = actionFromLabel(action)
				userLabel = "import-heuristic"
			}
		}

		sample := buildCommandTrainingSample(candidate.Comm, candidate.Args, "", 0, label, userLabel, candidate.eventTime)
		globalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}

	if err := globalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "imported samples but failed to persist: " + err.Error()})
		return
	}

	total, labeled := globalTrainingStore.Status()
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return req, false
	}

	comm, args := normalizeCommandInput(req.CommandLine, req.Comm, req.Args)
	if comm == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commandLine or comm is required"})
		return req, false
	}

	req.Comm = comm
	req.Args = args
	req.CommandLine = joinCommandLine(comm, args)
	return req, true
}

func assessCommandSafety(ctx context.Context, comm string, args []string, user string, pid uint32) gin.H {
	commandLine := joinCommandLine(comm, args)

	classification := ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, user, pid)

	var mlPrediction Prediction
	if mlEnabled && mlModelLoaded {
		mlPrediction = mlEngine.Predict(features)
	}

	simulatedAction, reason := resolveAction(
		&pb.WrapperRequest{Comm: comm, Args: args, User: user, Pid: pid},
		"", 0,
		classification, anomalyScore, mlPrediction, mlConfig,
	)
	if strings.TrimSpace(reason) == "" {
		reason = "No blocking policy matched"
	}

	netAudit := AuditNetworkBehavior(comm, strings.Join(args, " "))
	riskScore := computeRiskScore(classification, anomalyScore, mlPrediction, netAudit, nil)
	recommendedAction := actionLabel[int32(simulatedAction)]

	var matches []IndexedTrainingSample
	if globalTrainingStore != nil {
		matches = globalTrainingStore.ExactMatches(comm, args)
	}

	sampleMatches := sampleMatchesJSON(matches)
	sampleEvidence := summarizeSampleEvidence(matches)
	sampleEvidenceSummary := fmt.Sprintf(
		"matches=%v labeled=%v decision=%v confidence=%.2f",
		sampleEvidence["totalMatches"], sampleEvidence["labeledMatches"], sampleEvidence["decision"], sampleEvidence["confidence"],
	)
	var llmResult *llmAssessment
	if llmScoringConfigured() {
		llmReq := llmScoreRequest{
			CommandLine:    commandLine,
			Comm:           comm,
			Args:           append([]string(nil), args...),
			Category:       classification.PrimaryCategory,
			AnomalyScore:   anomalyScore,
			Classification: classification,
			MlAction:       actionLabel[mlPrediction.Action],
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
		"mlPrediction":      gin.H{"action": actionLabel[mlPrediction.Action], "confidence": mlPrediction.Confidence},
		"recommendedAction": recommendedAction,
		"reasoning":         reason,
		"riskScore":         riskScore,
		"riskLevel":         riskLevel(riskScore),
		"networkAudit":      netAudit,
		"sampleMatches":     sampleMatches,
		"sampleEvidence":    sampleEvidence,
		"llmAssessment":     llmResult,
		"modelLoaded":       mlModelLoaded,
		"mlEnabled":         mlEnabled,
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
		key := commandKey(candidate.Comm, candidate.Args)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if globalTrainingStore != nil {
			candidate.Duplicate = globalTrainingStore.HasExactCommand(candidate.Comm, candidate.Args)
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

	comm, args := normalizeCommandInput(commandLine, event.Comm, nil)
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
		CommandLine: joinCommandLine(comm, args),
		Comm:        comm,
		Args:        args,
		EventType:   eventType,
		Source:      source,
		Category:    category,
		Timestamp:   eventTime.Format(time.RFC3339),
		eventTime:   eventTime,
	}, true
}

func buildCommandTrainingSample(comm string, args []string, user string, pid uint32, label int32, userLabel string, timestamp time.Time) TrainingSample {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	classification := ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, user, pid)

	return TrainingSample{
		Features:     features,
		Label:        label,
		Comm:         comm,
		Args:         args,
		Category:     classification.PrimaryCategory,
		AnomalyScore: anomalyScore,
		Timestamp:    timestamp,
		UserLabel:    userLabel,
	}
}

func recordCommandSampleSideEffects(sample TrainingSample) {
	_, emb := globalEmbedder.ClassifyAndEmbed(sample.Comm, sample.Args)
	globalEmbedder.AddToCluster(emb)

	action := sampleLabelName(sample.Label)
	if action == "-" {
		action = "UNLABELED"
	}
	globalFeatureExtractor.AddHistory(sample.Comm, sample.Category, action, sample.AnomalyScore)
}

func normalizeCommandInput(commandLine string, comm string, args []string) (string, []string) {
	parts := splitCommandLine(commandLine)
	if len(parts) > 0 {
		return parts[0], parts[1:]
	}

	comm = strings.TrimSpace(comm)
	if comm == "" {
		return "", nil
	}
	cleanArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		cleanArgs = append(cleanArgs, arg)
	}
	return comm, cleanArgs
}

func splitCommandLine(commandLine string) []string {
	commandLine = strings.TrimSpace(strings.ReplaceAll(commandLine, "\x00", " "))
	if commandLine == "" {
		return nil
	}

	var parts []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	emit := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, b.String())
		b.Reset()
	}

	for _, r := range commandLine {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inSingle && !inDouble:
			emit()
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	emit()
	return parts
}

func joinCommandLine(comm string, args []string) string {
	parts := append([]string{strings.TrimSpace(comm)}, args...)
	compact := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		compact = append(compact, part)
	}
	return strings.Join(compact, " ")
}

func sampleMatchesJSON(matches []IndexedTrainingSample) []commandSampleMatch {
	out := make([]commandSampleMatch, 0, len(matches))
	for _, match := range matches {
		sample := match.Sample
		out = append(out, commandSampleMatch{
			Index:        match.Index,
			CommandLine:  joinCommandLine(sample.Comm, sample.Args),
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

func summarizeSampleEvidence(matches []IndexedTrainingSample) gin.H {
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
	if name, ok := actionLabel[label]; ok {
		return name
	}
	return "-"
}

func commandKey(comm string, args []string) string {
	return comm + "\x00" + strings.Join(args, "\x00")
}

func sameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
