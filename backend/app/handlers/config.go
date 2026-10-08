package handlers

import (
	"bytes"
	"encoding/json"
	"strconv"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

func HandleConfigTagsGet(c *gin.Context) {
	t := Deps.Config.TagNames()
	Deps.WriteProtoOrJSON(c, 200, &pb.ConfigTagList{Names: t}, t)
}

func HandleConfigTagsPost(c *gin.Context) {
	var r struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&r)
	Deps.Config.TagID(r.Name)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigCommsGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedCommList{}
	iter := Deps.TrackerMaps.TrackedCommsIterate()
	var k [16]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		comm := string(bytes.TrimRight(k[:], "\x00"))
		tag := Deps.Config.TagName(tid)
		disabled := Deps.Config.IsCommDisabled(comm)
		items = append(items, gin.H{"comm": comm, "tag": tag, "disabled": disabled})
		list.Items = append(list.Items, &pb.TrackedComm{Comm: comm, Tag: tag, Disabled: disabled})
	}
	Deps.WriteProtoOrJSON(c, 200, list, items)
}

func HandleConfigCommsPost(c *gin.Context) {
	var r struct {
		Comm string `json:"comm"`
		Tag  string `json:"tag"`
	}
	_ = c.ShouldBindJSON(&r)
	var k [16]byte
	copy(k[:], r.Comm)
	_ = Deps.TrackerMaps.TrackedCommsPut(k, Deps.Config.TagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigCommsDelete(c *gin.Context) {
	var k [16]byte
	comm := c.Param("comm")
	copy(k[:], comm)
	_ = Deps.TrackerMaps.TrackedCommsDelete(k)
	Deps.Config.RemoveDisabledComm(comm)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigCommsDisable(c *gin.Context) {
	Deps.Config.AddDisabledComm(c.Param("comm"))
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigCommsEnable(c *gin.Context) {
	Deps.Config.RemoveDisabledComm(c.Param("comm"))
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigEventTypesGet(c *gin.Context) {
	disabled := Deps.Config.DisabledEventTypes()
	c.JSON(200, gin.H{"disabled_event_types": disabled})
}

func persistDisabledEventTypes(c *gin.Context, disabled []uint32) bool {
	runtimeSettingsMutationMu.Lock()
	defer runtimeSettingsMutationMu.Unlock()

	settings := Deps.RuntimeSettings.Snapshot()
	settings.DisabledEventTypes = append([]uint32(nil), disabled...)
	updated, err := Deps.RuntimeSettingsReplace(settings)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return false
	}
	// RuntimeSettingsReplace applies the live app gates in production. Keep the
	// explicit config update as well so handler tests and alternate injectors
	// observe the same state transition.
	Deps.Config.ReplaceDisabledEventTypes(updated.DisabledEventTypes)
	return true
}

func HandleConfigEventTypesPut(c *gin.Context) {
	var request struct {
		DisabledEventTypes []uint32 `json:"disabled_event_types"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"error": "invalid event type configuration"})
		return
	}
	seen := make(map[uint32]struct{}, len(request.DisabledEventTypes))
	disabled := make([]uint32, 0, len(request.DisabledEventTypes))
	for _, eventType := range request.DisabledEventTypes {
		// EventType currently occupies a compact 0..43 enum. Leave headroom for
		// compatible additions while rejecting obviously accidental values.
		if eventType > 255 {
			c.JSON(400, gin.H{"error": "event type out of supported range"})
			return
		}
		if _, exists := seen[eventType]; exists {
			continue
		}
		seen[eventType] = struct{}{}
		disabled = append(disabled, eventType)
	}
	if !persistDisabledEventTypes(c, disabled) {
		return
	}
	c.JSON(200, gin.H{"status": "ok", "disabled_event_types": Deps.Config.DisabledEventTypes()})
}

func HandleConfigEventTypeDisable(c *gin.Context) {
	typeID, err := strconv.Atoi(c.Param("type"))
	if err != nil || typeID < 0 || typeID > 255 {
		c.JSON(400, gin.H{"error": "invalid event type"})
		return
	}
	disabled := Deps.Config.DisabledEventTypes()
	target := uint32(typeID)
	seen := false
	for _, eventType := range disabled {
		if eventType == target {
			seen = true
			break
		}
	}
	if !seen {
		disabled = append(disabled, target)
	}
	if !persistDisabledEventTypes(c, disabled) {
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigEventTypeEnable(c *gin.Context) {
	typeID, err := strconv.Atoi(c.Param("type"))
	if err != nil || typeID < 0 || typeID > 255 {
		c.JSON(400, gin.H{"error": "invalid event type"})
		return
	}
	target := uint32(typeID)
	current := Deps.Config.DisabledEventTypes()
	disabled := make([]uint32, 0, len(current))
	for _, eventType := range current {
		if eventType != target {
			disabled = append(disabled, eventType)
		}
	}
	if !persistDisabledEventTypes(c, disabled) {
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigPathsGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedPathList{}
	iter := Deps.TrackerMaps.TrackedPathsIterate()
	var k [256]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		path := string(bytes.TrimRight(k[:], "\x00"))
		tag := Deps.Config.TagName(tid)
		items = append(items, gin.H{"path": path, "tag": tag})
		list.Items = append(list.Items, &pb.TrackedPath{Path: path, Tag: tag})
	}
	Deps.WriteProtoOrJSON(c, 200, list, items)
}

func HandleConfigPathsPost(c *gin.Context) {
	var r struct {
		Path string `json:"path"`
		Tag  string `json:"tag"`
	}
	_ = c.ShouldBindJSON(&r)
	var k [256]byte
	copy(k[:], r.Path)
	_ = Deps.TrackerMaps.TrackedPathsPut(k, Deps.Config.TagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigPathsDelete(c *gin.Context) {
	p := c.Param("path")
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	var k [256]byte
	copy(k[:], p)
	_ = Deps.TrackerMaps.TrackedPathsDelete(k)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigPrefixesGet(c *gin.Context) {
	items := []gin.H{}
	list := &pb.TrackedPrefixList{}
	iter := Deps.TrackerMaps.TrackedPrefixesIterate()
	if iter == nil {
		Deps.WriteProtoOrJSON(c, 200, list, items)
		return
	}
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
		tag := Deps.Config.TagName(tid)
		items = append(items, gin.H{"prefix": prefix, "tag": tag})
		list.Items = append(list.Items, &pb.TrackedPrefix{Prefix: prefix, Tag: tag})
	}
	Deps.WriteProtoOrJSON(c, 200, list, items)
}

func HandleConfigPrefixesPost(c *gin.Context) {
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
	_ = Deps.TrackerMaps.TrackedPrefixesPut(k, Deps.Config.TagID(r.Tag))
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigPrefixesDelete(c *gin.Context) {
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
	_ = Deps.TrackerMaps.TrackedPrefixesDelete(k)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigRulesGet(c *gin.Context) {
	rules := Deps.Config.Rules()
	list := &pb.WrapperRuleList{}
	for _, r := range rules {
		list.Items = append(list.Items, r)
	}
	Deps.WriteProtoOrJSON(c, 200, list, rules)
}

type rewrittenCommand []string

func (r *rewrittenCommand) UnmarshalJSON(data []byte) error {
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*r = values
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	if single == "" {
		*r = nil
	} else {
		*r = []string{single}
	}
	return nil
}

func HandleConfigRulesPost(c *gin.Context) {
	var r struct {
		Comm         string           `json:"comm"`
		Action       string           `json:"action"`
		RewrittenCmd rewrittenCommand `json:"rewritten_cmd"`
		Regex        string           `json:"regex"`
		Replacement  string           `json:"replacement"`
		Priority     int32            `json:"priority"`
	}
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(400, gin.H{"error": "invalid rule"})
		return
	}
	Deps.Config.UpsertRule(r.Comm, r.Action, []string(r.RewrittenCmd), r.Regex, r.Replacement, r.Priority)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigRulesDelete(c *gin.Context) {
	Deps.Config.DeleteRule(c.Param("comm"))
	c.JSON(200, gin.H{"status": "ok"})
}
