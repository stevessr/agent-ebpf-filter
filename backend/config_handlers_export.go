package main

import (
	"bytes"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

func handleConfigExportGet(c *gin.Context) {
	runtimeSnapshot := runtimeSettingsStore.Snapshot()
	cfg := core.ExportConfig{
		Comms:   make(map[string]string),
		Paths:   make(map[string]string),
		Rules:   make(map[string]core.WrapperRule),
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
	var cfg core.ExportConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"error": "invalid import data"})
		return
	}
	if cfg.Runtime != nil {
		settings, err := runtimeSettingsStore.Replace(*cfg.Runtime)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		applyRetentionConfig(settings)
		applyRuntimeDomainForwardProxy(settings)
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
	wrapperRules = make(map[string]core.WrapperRule, len(cfg.Rules))
	for comm, rule := range cfg.Rules {
		wrapperRules[comm] = rule
	}
	rulesMu.Unlock()
	c.JSON(200, gin.H{"status": "ok"})
}
