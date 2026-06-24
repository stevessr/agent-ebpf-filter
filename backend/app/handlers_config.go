package app

import (
	"agent-ebpf-filter/pb"
	"bytes"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlers_config.go ----

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
