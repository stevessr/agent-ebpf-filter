package handlers

import (
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"bytes"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlersexportconfig.go ----

func HandleConfigExportGet(c *gin.Context) {
	runtimeSnapshot := Deps.RuntimeSettings.Snapshot()
	cfg := core.ExportConfig{
		Comms:   make(map[string]string),
		Paths:   make(map[string]string),
		Rules:   make(map[string]core.WrapperRule),
		Runtime: &runtimeSnapshot,
	}
	for _, name := range Deps.ConfigTagNames() {
		cfg.Tags = append(cfg.Tags, name)
	}

	var k16 [16]byte
	var k256 [256]byte
	var tid uint32
	i1 := Deps.TrackerMaps.TrackedCommsIterate()
	for i1.Next(&k16, &tid) {
		cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = Deps.GetTagName(tid)
	}
	i2 := Deps.TrackerMaps.TrackedPathsIterate()
	for i2.Next(&k256, &tid) {
		cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = Deps.GetTagName(tid)
	}
	for _, r := range Deps.ConfigRules() {
		cfg.Rules[r.Comm] = core.WrapperRule{
			Comm:         r.Comm,
			Action:       r.Action,
			RewrittenCmd: r.RewrittenCmd,
			Regex:        r.Regex,
			Replacement:  r.Replacement,
			Priority:     int(r.Priority),
		}
	}

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
	Deps.WriteProtoOrJSON(c, 200, protoCfg, cfg)
}

func HandleConfigImportPost(c *gin.Context) {
	var cfg core.ExportConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"error": "invalid import data"})
		return
	}
	if cfg.Runtime != nil {
		settings, err := Deps.RuntimeSettingsReplace(*cfg.Runtime)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		// After replacement, run app-level post-processing via Deps
		Deps.ApplyRetentionConfig(settings)
		Deps.ApplyRuntimeDomainForwardProxy(settings)
	}
	for _, t := range cfg.Tags {
		Deps.GetTagID(t)
	}
	for comm, tag := range cfg.Comms {
		var k [16]byte
		copy(k[:], comm)
		_ = Deps.TrackerMaps.TrackedCommsPut(k, Deps.GetTagID(tag))
	}
	for p, tag := range cfg.Paths {
		var k [256]byte
		copy(k[:], p)
		_ = Deps.TrackerMaps.TrackedPathsPut(k, Deps.GetTagID(tag))
	}
	for _, rule := range cfg.Rules {
		Deps.UpsertConfigRule(rule.Comm, rule.Action, "", rule.Regex, rule.Replacement, int32(rule.Priority))
	}
	c.JSON(200, gin.H{"status": "ok"})
}