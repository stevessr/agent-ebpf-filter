package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

func handleConfigTagsGet(c *gin.Context) {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	t := []string{}
	for _, n := range tagMap {
		t = append(t, n)
	}
	writeProtoOrJSON(c, 200, &pb.ConfigTagList{Names: t}, t)
}

func handleConfigTagsPost(c *gin.Context) {
	var r struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&r)
	getTagID(r.Name)
	c.JSON(200, gin.H{"status": "ok"})
}

func isCommDisabled(comm string) bool {
	disabledCommsMu.RLock()
	defer disabledCommsMu.RUnlock()
	_, ok := disabledComms[comm]
	return ok
}

func isEventTypeDisabled(et uint32) bool {
	disabledEventTypesMu.RLock()
	defer disabledEventTypesMu.RUnlock()
	_, ok := disabledEventTypes[et]
	return ok
}

func handleConfigCommsGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedCommList{}
	iter := trackerMaps.TrackedComms.Iterate()
	var k [16]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		comm := string(bytes.TrimRight(k[:], "\x00"))
		tag := getTagName(tid)
		disabled := isCommDisabled(comm)
		items = append(items, gin.H{"comm": comm, "tag": tag, "disabled": disabled})
		list.Items = append(list.Items, &pb.TrackedComm{Comm: comm, Tag: tag, Disabled: disabled})
	}
	writeProtoOrJSON(c, 200, list, items)
}

func handleConfigCommsPost(c *gin.Context) {
	var r struct {
		Comm string `json:"comm"`
		Tag  string `json:"tag"`
	}
	_ = c.ShouldBindJSON(&r)
	var k [16]byte
	copy(k[:], r.Comm)
	_ = trackerMaps.TrackedComms.Put(k, getTagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigCommsDelete(c *gin.Context) {
	var k [16]byte
	copy(k[:], c.Param("comm"))
	_ = trackerMaps.TrackedComms.Delete(k)
	// also remove from disabled set
	disabledCommsMu.Lock()
	delete(disabledComms, c.Param("comm"))
	disabledCommsMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigCommsDisable(c *gin.Context) {
	comm := c.Param("comm")
	disabledCommsMu.Lock()
	disabledComms[comm] = struct{}{}
	disabledCommsMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigCommsEnable(c *gin.Context) {
	comm := c.Param("comm")
	disabledCommsMu.Lock()
	delete(disabledComms, comm)
	disabledCommsMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigEventTypesGet(c *gin.Context) {
	disabledEventTypesMu.RLock()
	defer disabledEventTypesMu.RUnlock()
	disabled := make([]uint32, 0, len(disabledEventTypes))
	for et := range disabledEventTypes {
		disabled = append(disabled, et)
	}
	c.JSON(200, gin.H{"disabled_event_types": disabled})
}

func handleConfigEventTypeDisable(c *gin.Context) {
	typeID, err := strconv.Atoi(c.Param("type"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid event type"})
		return
	}
	disabledEventTypesMu.Lock()
	disabledEventTypes[uint32(typeID)] = struct{}{}
	disabledEventTypesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigEventTypeEnable(c *gin.Context) {
	typeID, err := strconv.Atoi(c.Param("type"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid event type"})
		return
	}
	disabledEventTypesMu.Lock()
	delete(disabledEventTypes, uint32(typeID))
	disabledEventTypesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigPathsGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedPathList{}
	iter := trackerMaps.TrackedPaths.Iterate()
	var k [256]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		path := string(bytes.TrimRight(k[:], "\x00"))
		tag := getTagName(tid)
		items = append(items, gin.H{"path": path, "tag": tag})
		list.Items = append(list.Items, &pb.TrackedPath{Path: path, Tag: tag})
	}
	writeProtoOrJSON(c, 200, list, items)
}

func handleConfigPathsPost(c *gin.Context) {
	var r struct {
		Path string `json:"path"`
		Tag  string `json:"tag"`
	}
	_ = c.ShouldBindJSON(&r)
	var k [256]byte
	copy(k[:], r.Path)
	_ = trackerMaps.TrackedPaths.Put(k, getTagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigPathsDelete(c *gin.Context) {
	p := c.Param("path")
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	var k [256]byte
	copy(k[:], p)
	_ = trackerMaps.TrackedPaths.Delete(k)
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigPrefixesGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedPrefixList{}
	if trackerMaps.TrackedPrefixes == nil {
		writeProtoOrJSON(c, 200, list, items)
		return
	}
	iter := trackerMaps.TrackedPrefixes.Iterate()
	var k struct {
		PrefixLen uint32
		Data      [64]byte
	}
	var tid uint32
	for iter.Next(&k, &tid) {
		prefix := string(bytes.TrimRight(k.Data[:], "\x00"))
		prefixLen := k.PrefixLen / 8
		if prefixLen > 0 && uint32(len(prefix)) > prefixLen {
			prefix = prefix[:prefixLen]
		}
		tag := getTagName(tid)
		items = append(items, gin.H{"prefix": prefix, "tag": tag})
		list.Items = append(list.Items, &pb.TrackedPrefix{Prefix: prefix, Tag: tag})
	}
	writeProtoOrJSON(c, 200, list, items)
}

func handleConfigPrefixesPost(c *gin.Context) {
	var r struct {
		Prefix string `json:"prefix"`
		Tag    string `json:"tag"`
	}
	_ = c.ShouldBindJSON(&r)
	if r.Prefix == "" {
		c.JSON(400, gin.H{"error": "prefix is required"})
		return
	}
	var k struct {
		PrefixLen uint32
		Data      [64]byte
	}
	plen := len(r.Prefix)
	if plen > 63 {
		plen = 63
	}
	k.PrefixLen = uint32(plen * 8)
	copy(k.Data[:], r.Prefix[:plen])
	_ = trackerMaps.TrackedPrefixes.Put(k, getTagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigPrefixesDelete(c *gin.Context) {
	prefix := c.Query("prefix")
	if prefix == "" {
		c.JSON(400, gin.H{"error": "prefix query parameter is required"})
		return
	}
	var k struct {
		PrefixLen uint32
		Data      [64]byte
	}
	plen := len(prefix)
	if plen > 63 {
		plen = 63
	}
	k.PrefixLen = uint32(plen * 8)
	copy(k.Data[:], prefix[:plen])
	_ = trackerMaps.TrackedPrefixes.Delete(k)
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigRulesGet(c *gin.Context) {
	rulesMu.RLock()
	defer rulesMu.RUnlock()
	list := &pb.WrapperRuleList{}
	for _, r := range wrapperRules {
		list.Items = append(list.Items, &pb.WrapperRule{
			Comm:         r.Comm,
			Action:       r.Action,
			RewrittenCmd: r.RewrittenCmd,
			Regex:        r.Regex,
			Replacement:  r.Replacement,
			Priority:     int32(r.Priority),
		})
	}
	writeProtoOrJSON(c, 200, list, wrapperRules)
}

func handleConfigRulesPost(c *gin.Context) {
	var r WrapperRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(400, gin.H{"error": "invalid rule"})
		return
	}
	rulesMu.Lock()
	wrapperRules[r.Comm] = r
	rulesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigRulesDelete(c *gin.Context) {
	rulesMu.Lock()
	delete(wrapperRules, c.Param("comm"))
	rulesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigRuntimeGet(c *gin.Context) {
	rc := buildRuntimeConfigResponse()
	protoResp := &pb.RuntimeConfigResponse{
		Runtime: &pb.RuntimeSettings{
			LogPersistenceEnabled:   rc.Runtime.LogPersistenceEnabled,
			LogFilePath:             rc.Runtime.LogFilePath,
			AccessToken:             rc.Runtime.AccessToken,
			MaxEventCount:           int32(rc.Runtime.MaxEventCount),
			MaxEventAge:             rc.Runtime.MaxEventAge,
			ShellSessionsEnabled:    rc.Runtime.ShellSessionsEnabled,
			SystemRunEnabled:        rc.Runtime.SystemRunEnabled,
			HookManagementEnabled:   rc.Runtime.HookManagementEnabled,
			PolicyManagementEnabled: rc.Runtime.PolicyManagementEnabled,
			OtlpEnabled:             rc.Runtime.OtlpEnabled,
			OtlpEndpoint:            rc.Runtime.OtlpEndpoint,
			OtlpServiceName:         rc.Runtime.OtlpServiceName,
			OtlpHeaders:             rc.Runtime.OtlpHeaders,
		},
		McpEndpoint:            rc.MCPEndpoint,
		AuthHeaderName:         rc.AuthHeaderName,
		BearerAuthHeaderName:   rc.BearerAuthHeaderName,
		PersistedEventLogPath:  rc.PersistedEventLogPath,
		PersistedEventLogAlive: rc.PersistedEventLogAlive,
	}
	writeProtoOrJSON(c, http.StatusOK, protoResp, rc)
}

func handleConfigRuntimePut(c *gin.Context) {
	var req runtimeSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings"})
		return
	}

	settings := runtimeSettingsStore.Snapshot()
	if req.LogPersistenceEnabled != nil {
		settings.LogPersistenceEnabled = *req.LogPersistenceEnabled
	}
	if req.LogFilePath != nil {
		settings.LogFilePath = strings.TrimSpace(*req.LogFilePath)
	}
	if req.AccessToken != nil {
		settings.AccessToken = strings.TrimSpace(*req.AccessToken)
	}
	if req.MaxEventCount != nil {
		settings.MaxEventCount = *req.MaxEventCount
	}
	if req.MaxEventAge != nil {
		settings.MaxEventAge = strings.TrimSpace(*req.MaxEventAge)
	}
	if req.ShellSessionsEnabled != nil {
		settings.ShellSessionsEnabled = *req.ShellSessionsEnabled
	}
	if req.SystemRunEnabled != nil {
		settings.SystemRunEnabled = *req.SystemRunEnabled
	}
	if req.HookManagementEnabled != nil {
		settings.HookManagementEnabled = *req.HookManagementEnabled
	}
	if req.PolicyManagementEnabled != nil {
		settings.PolicyManagementEnabled = *req.PolicyManagementEnabled
	}
	if req.OtlpEnabled != nil {
		settings.OtlpEnabled = *req.OtlpEnabled
	}
	if req.OtlpEndpoint != nil {
		settings.OtlpEndpoint = strings.TrimSpace(*req.OtlpEndpoint)
	}
	if req.OtlpServiceName != nil {
		settings.OtlpServiceName = strings.TrimSpace(*req.OtlpServiceName)
	}
	if req.OtlpHeaders != nil {
		settings.OtlpHeaders = make(map[string]string, len(req.OtlpHeaders))
		for key, value := range req.OtlpHeaders {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			settings.OtlpHeaders[trimmedKey] = strings.TrimSpace(value)
		}
	}
	applyMLConfigPatch(&settings.MLConfig, req.MLConfigPatch)

	settings, err := runtimeSettingsStore.Replace(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	applyRetentionConfig(settings)
	c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
}

type runtimeSettingsPatch struct {
	LogPersistenceEnabled   *bool             `json:"logPersistenceEnabled,omitempty"`
	LogFilePath             *string           `json:"logFilePath,omitempty"`
	AccessToken             *string           `json:"accessToken,omitempty"`
	MaxEventCount           *int              `json:"maxEventCount,omitempty"`
	MaxEventAge             *string           `json:"maxEventAge,omitempty"`
	ShellSessionsEnabled    *bool             `json:"shellSessionsEnabled,omitempty"`
	SystemRunEnabled        *bool             `json:"systemRunEnabled,omitempty"`
	HookManagementEnabled   *bool             `json:"hookManagementEnabled,omitempty"`
	PolicyManagementEnabled *bool             `json:"policyManagementEnabled,omitempty"`
	OtlpEnabled             *bool             `json:"otlpEnabled,omitempty"`
	OtlpEndpoint            *string           `json:"otlpEndpoint,omitempty"`
	OtlpServiceName         *string           `json:"otlpServiceName,omitempty"`
	OtlpHeaders             map[string]string `json:"otlpHeaders,omitempty"`
	MLConfigPatch
}

type MLConfigPatch struct {
	Enabled                  *bool    `json:"enabled,omitempty"`
	BlockConfidenceThreshold *float64 `json:"blockConfidenceThreshold,omitempty"`
	MlMinConfidence          *float64 `json:"mlMinConfidence,omitempty"`
	LowAnomalyThreshold      *float64 `json:"lowAnomalyThreshold,omitempty"`
	HighAnomalyThreshold     *float64 `json:"highAnomalyThreshold,omitempty"`
	RuleOverridePriority     *int     `json:"ruleOverridePriority,omitempty"`
	ModelType                *string  `json:"modelType,omitempty"`
	ModelPath                *string  `json:"modelPath,omitempty"`
	AutoTrain                *bool    `json:"autoTrain,omitempty"`
	TrainInterval            *string  `json:"trainInterval,omitempty"`
	MinSamplesForTraining    *int     `json:"minSamplesForTraining,omitempty"`
	ActiveLearningEnabled    *bool    `json:"activeLearningEnabled,omitempty"`
	FeatureHistorySize       *int     `json:"featureHistorySize,omitempty"`
	NumTrees                 *int     `json:"numTrees,omitempty"`
	MaxDepth                 *int     `json:"maxDepth,omitempty"`
	MinSamplesLeaf           *int     `json:"minSamplesLeaf,omitempty"`
	ValidationSplitRatio     *float64 `json:"validationSplitRatio,omitempty"`
	BalanceClasses           *bool    `json:"balanceClasses,omitempty"`
	LlmEnabled               *bool    `json:"llmEnabled,omitempty"`
	LlmBaseURL               *string  `json:"llmBaseUrl,omitempty"`
	LlmAPIKey                *string  `json:"llmApiKey,omitempty"`
	LlmModel                 *string  `json:"llmModel,omitempty"`
	LlmTimeoutSeconds        *int     `json:"llmTimeoutSeconds,omitempty"`
	LlmTemperature           *float64 `json:"llmTemperature,omitempty"`
	LlmMaxTokens             *int     `json:"llmMaxTokens,omitempty"`
	LlmSystemPrompt          *string  `json:"llmSystemPrompt,omitempty"`
}

func applyMLConfigPatch(dst *MLConfig, patch MLConfigPatch) {
	if patch.Enabled != nil {
		dst.Enabled = *patch.Enabled
	}
	if patch.BlockConfidenceThreshold != nil {
		dst.BlockConfidenceThreshold = *patch.BlockConfidenceThreshold
	}
	if patch.MlMinConfidence != nil {
		dst.MlMinConfidence = *patch.MlMinConfidence
	}
	if patch.LowAnomalyThreshold != nil {
		dst.LowAnomalyThreshold = *patch.LowAnomalyThreshold
	}
	if patch.HighAnomalyThreshold != nil {
		dst.HighAnomalyThreshold = *patch.HighAnomalyThreshold
	}
	if patch.RuleOverridePriority != nil {
		dst.RuleOverridePriority = *patch.RuleOverridePriority
	}
	if patch.ModelType != nil {
		t := ModelType(strings.TrimSpace(*patch.ModelType))
		if _, ok := modelRegistry[t]; ok {
			dst.ModelType = t
			currentModelType = t
		}
	}
	if patch.ModelPath != nil {
		dst.ModelPath = strings.TrimSpace(*patch.ModelPath)
	}
	if patch.AutoTrain != nil {
		dst.AutoTrain = *patch.AutoTrain
	}
	if patch.TrainInterval != nil {
		dst.TrainInterval = strings.TrimSpace(*patch.TrainInterval)
	}
	if patch.MinSamplesForTraining != nil {
		dst.MinSamplesForTraining = *patch.MinSamplesForTraining
	}
	if patch.ActiveLearningEnabled != nil {
		dst.ActiveLearningEnabled = *patch.ActiveLearningEnabled
	}
	if patch.FeatureHistorySize != nil {
		dst.FeatureHistorySize = *patch.FeatureHistorySize
	}
	if patch.NumTrees != nil {
		dst.NumTrees = *patch.NumTrees
	}
	if patch.MaxDepth != nil {
		dst.MaxDepth = *patch.MaxDepth
	}
	if patch.MinSamplesLeaf != nil {
		dst.MinSamplesLeaf = *patch.MinSamplesLeaf
	}
	if patch.ValidationSplitRatio != nil {
		dst.ValidationSplitRatio = *patch.ValidationSplitRatio
	}
	if patch.BalanceClasses != nil {
		dst.BalanceClasses = *patch.BalanceClasses
	}
	if patch.LlmEnabled != nil {
		dst.LlmEnabled = *patch.LlmEnabled
	}
	if patch.LlmBaseURL != nil {
		dst.LlmBaseURL = strings.TrimSpace(*patch.LlmBaseURL)
	}
	if patch.LlmAPIKey != nil {
		if key := strings.TrimSpace(*patch.LlmAPIKey); key != "" {
			dst.LlmAPIKey = key
		}
	}
	if patch.LlmModel != nil {
		dst.LlmModel = strings.TrimSpace(*patch.LlmModel)
	}
	if patch.LlmTimeoutSeconds != nil {
		dst.LlmTimeoutSeconds = *patch.LlmTimeoutSeconds
	}
	if patch.LlmTemperature != nil {
		dst.LlmTemperature = *patch.LlmTemperature
	}
	if patch.LlmMaxTokens != nil {
		dst.LlmMaxTokens = *patch.LlmMaxTokens
	}
	if patch.LlmSystemPrompt != nil {
		dst.LlmSystemPrompt = *patch.LlmSystemPrompt
	}
}

func handleConfigAccessTokenPost(c *gin.Context) {
	settings, err := runtimeSettingsStore.RotateAccessToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
}

func handleConfigExportGet(c *gin.Context) {
	runtimeSnapshot := runtimeSettingsStore.Snapshot()
	cfg := ExportConfig{
		Comms:   make(map[string]string),
		Paths:   make(map[string]string),
		Rules:   make(map[string]WrapperRule),
		Runtime: &runtimeSnapshot,
	}
	tagsMu.RLock()
	for _, n := range tagMap {
		cfg.Tags = append(cfg.Tags, n)
	}
	tagsMu.RUnlock()

	var k16 [16]byte
	var k256 [256]byte
	var tid uint32
	i1 := trackerMaps.TrackedComms.Iterate()
	for i1.Next(&k16, &tid) {
		cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = getTagName(tid)
	}
	i2 := trackerMaps.TrackedPaths.Iterate()
	for i2.Next(&k256, &tid) {
		cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = getTagName(tid)
	}
	rulesMu.RLock()
	for comm, rule := range wrapperRules {
		cfg.Rules[comm] = rule
	}
	rulesMu.RUnlock()

	protoCfg := &pb.ExportConfigData{
		Tags:  cfg.Tags,
		Comms: make([]*pb.TrackedComm, 0, len(cfg.Comms)),
		Paths: make([]*pb.TrackedPath, 0, len(cfg.Paths)),
		Rules: make([]*pb.WrapperRule, 0, len(cfg.Rules)),
	}
	for comm, tag := range cfg.Comms {
		protoCfg.Comms = append(protoCfg.Comms, &pb.TrackedComm{Comm: comm, Tag: tag})
	}
	for path, tag := range cfg.Paths {
		protoCfg.Paths = append(protoCfg.Paths, &pb.TrackedPath{Path: path, Tag: tag})
	}
	for _, rule := range cfg.Rules {
		protoCfg.Rules = append(protoCfg.Rules, &pb.WrapperRule{
			Comm:         rule.Comm,
			Action:       rule.Action,
			RewrittenCmd: rule.RewrittenCmd,
			Regex:        rule.Regex,
			Replacement:  rule.Replacement,
			Priority:     int32(rule.Priority),
		})
	}
	if cfg.Runtime != nil {
		protoCfg.Runtime = &pb.RuntimeSettings{
			LogPersistenceEnabled: cfg.Runtime.LogPersistenceEnabled,
			LogFilePath:           cfg.Runtime.LogFilePath,
			AccessToken:           cfg.Runtime.AccessToken,
			MaxEventCount:         int32(cfg.Runtime.MaxEventCount),
			MaxEventAge:           cfg.Runtime.MaxEventAge,
		}
	}
	writeProtoOrJSON(c, 200, protoCfg, cfg)
}

func handleConfigImportPost(c *gin.Context) {
	var cfg ExportConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"error": "invalid import data"})
		return
	}
	if cfg.Runtime != nil {
		if _, err := runtimeSettingsStore.Replace(*cfg.Runtime); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	for _, t := range cfg.Tags {
		getTagID(t)
	}
	for comm, tag := range cfg.Comms {
		var k [16]byte
		copy(k[:], comm)
		_ = trackerMaps.TrackedComms.Put(k, getTagID(tag))
	}
	for p, tag := range cfg.Paths {
		var k [256]byte
		copy(k[:], p)
		_ = trackerMaps.TrackedPaths.Put(k, getTagID(tag))
	}
	rulesMu.Lock()
	wrapperRules = make(map[string]WrapperRule, len(cfg.Rules))
	for comm, rule := range cfg.Rules {
		wrapperRules[comm] = rule
	}
	rulesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigHooksList(c *gin.Context) {
	res := []gin.H{}
	for _, h := range availableHooks {
		res = append(res, gin.H{
			"id": h.ID, "name": h.Name, "description": h.Description,
			"target_cmd": h.TargetCmd, "hook_type": h.HookType,
			"installed": isHookInstalled(h),
		})
	}
	c.JSON(200, res)
}

func handleConfigHooksInstall(c *gin.Context) {
	var req struct {
		ID         string `json:"id"`
		Install    bool   `json:"install"`
		UseWrapper bool   `json:"use_wrapper"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == req.ID {
			target = h
			found = true
			break
		}
	}
	if !found {
		c.JSON(404, gin.H{"error": "hook not found"})
		return
	}

	effectiveType := target.HookType
	if req.UseWrapper {
		effectiveType = HookTypeWrapper
	}

	if req.Install {
		if effectiveType == HookTypeNative {
			if err := installNativeHook(target); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		} else {
			p := getShellConfigPath()
			b, _ := os.ReadFile(p)
			content := string(b)
			aliasLine := fmt.Sprintf("\nalias %s='agent-wrapper %s' # agent-ebpf-hook\n", target.TargetCmd, target.TargetCmd)
			if !strings.Contains(content, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				f.WriteString(aliasLine)
				f.Close()
			}
		}
	} else {
		if target.HookType == HookTypeNative {
			_ = uninstallNativeHook(target)
		}
		p := getShellConfigPath()
		b, _ := os.ReadFile(p)
		lines := strings.Split(string(b), "\n")
		newLines := []string{}
		for _, l := range lines {
			if !strings.Contains(l, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				newLines = append(newLines, l)
			}
		}
		_ = os.WriteFile(p, []byte(strings.Join(newLines, "\n")), 0644)
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigHooksRawGet(c *gin.Context) {
	id := c.Param("id")
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	if target.ID == "kiro" {
		if err := ensureKiroManagedAgentExists(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	b, err := os.ReadFile(target.NativeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(200, gin.H{"content": "{}", "path": target.NativeConfigPath, "format": target.ConfigFormat})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"content": string(b), "path": target.NativeConfigPath, "format": target.ConfigFormat})
}

func handleConfigHooksRawPost(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	var js map[string]interface{}
	if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	if err := os.MkdirAll(filepath.Dir(target.NativeConfigPath), 0755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := os.WriteFile(target.NativeConfigPath, []byte(req.Content), 0644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func registerConfigRoutes(rg *gin.RouterGroup) {
	rg.GET("/tags", handleConfigTagsGet)
	rg.POST("/tags", policyManagementEnabledMiddleware(), handleConfigTagsPost)
	rg.GET("/comms", handleConfigCommsGet)
	rg.POST("/comms", policyManagementEnabledMiddleware(), handleConfigCommsPost)
	rg.DELETE("/comms/:comm", policyManagementEnabledMiddleware(), handleConfigCommsDelete)
	rg.POST("/comms/:comm/disable", policyManagementEnabledMiddleware(), handleConfigCommsDisable)
	rg.DELETE("/comms/:comm/disable", policyManagementEnabledMiddleware(), handleConfigCommsEnable)
	rg.GET("/event-types", handleConfigEventTypesGet)
	rg.POST("/event-types/:type/disable", policyManagementEnabledMiddleware(), handleConfigEventTypeDisable)
	rg.DELETE("/event-types/:type/disable", policyManagementEnabledMiddleware(), handleConfigEventTypeEnable)
	rg.GET("/paths", handleConfigPathsGet)
	rg.POST("/paths", policyManagementEnabledMiddleware(), handleConfigPathsPost)
	rg.DELETE("/paths/*path", policyManagementEnabledMiddleware(), handleConfigPathsDelete)
	rg.GET("/prefixes", handleConfigPrefixesGet)
	rg.POST("/prefixes", policyManagementEnabledMiddleware(), handleConfigPrefixesPost)
	rg.DELETE("/prefixes", policyManagementEnabledMiddleware(), handleConfigPrefixesDelete)
	rg.GET("/rules", handleConfigRulesGet)
	rg.POST("/rules", policyManagementEnabledMiddleware(), handleConfigRulesPost)
	rg.DELETE("/rules/:comm", policyManagementEnabledMiddleware(), handleConfigRulesDelete)
	rg.GET("/runtime", handleConfigRuntimeGet)
	rg.PUT("/runtime", handleConfigRuntimePut)
	rg.POST("/access-token", handleConfigAccessTokenPost)
	rg.GET("/export", handleConfigExportGet)
	rg.POST("/import", policyManagementEnabledMiddleware(), handleConfigImportPost)

	// ML classification endpoints
	ml := rg.Group("/ml")
	{
		ml.GET("/status", handleMLStatusGet)
		ml.GET("/logs", handleMLLogsGet)
		ml.GET("/history", handleMLHistoryGet)
		ml.POST("/train", handleMLTrainPost)
		ml.POST("/train/cancel", handleMLTrainCancelPost)
		ml.POST("/tune", handleMLTunePost)
		ml.POST("/feedback", handleMLFeedbackPost)
		ml.GET("/samples", handleMLSamplesGet)
		ml.POST("/samples", handleMLSamplesPost)
		ml.PUT("/samples/label", handleMLSampleLabelPut)
		ml.PUT("/samples/anomaly", handleMLSampleAnomalyPut)
		ml.DELETE("/samples/:index", handleMLSampleDelete)
		ml.GET("/existing-commands", handleMLExistingCommandsGet)
		ml.POST("/import-existing", handleMLImportExistingPost)
		ml.POST("/assess", handleMLAssessPost)
		ml.POST("/llm/score", handleMLLLMScorePost)
		ml.POST("/llm/batch-score", handleMLLLMBatchScorePost)
		ml.POST("/llm/production-dataset/pull", handleMLLLMProductionDatasetPullPost)
		ml.POST("/datasets/pull", handleMLDatasetPullPost)
		ml.POST("/datasets/import", handleMLDatasetImportPost)
		ml.GET("/datasets/export", handleMLDatasetExportGet)
		ml.DELETE("/datasets", handleMLDatasetClearDelete)
		ml.POST("/backtest", handleMLBacktestPost)
	}

	hooks := rg.Group("/hooks")
	{
		hooks.GET("", handleConfigHooksList)
		hooks.POST("", hookManagementEnabledMiddleware(), handleConfigHooksInstall)
		hooks.GET("/:id/raw", handleConfigHooksRawGet)
		hooks.POST("/:id/raw", hookManagementEnabledMiddleware(), handleConfigHooksRawPost)
	}
}
