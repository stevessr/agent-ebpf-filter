package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/core"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlershooksconfig.go ----

func HandleConfigHooksList(c *gin.Context) {
	res := []gin.H{}
	for _, h := range Deps.AvailableHooks() {
		res = append(res, gin.H{
			"id": h.ID, "name": h.Name, "description": h.Description,
			"target_cmd": h.TargetCmd, "hook_type": h.HookType,
			"installed": Deps.IsHookInstalled(h),
		})
	}
	c.JSON(200, res)
}

func HandleConfigHooksInstall(c *gin.Context) {
	var req struct {
		ID         string `json:"id"`
		Install    bool   `json:"install"`
		UseWrapper bool   `json:"use_wrapper"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target core.HookDef
	found := false
	for _, h := range Deps.AvailableHooks() {
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
		effectiveType = core.HookTypeWrapper
	}

	if req.Install {
		if effectiveType == core.HookTypeNative {
			if err := Deps.InstallNativeHook(target); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		} else {
			p := Deps.GetShellConfigPath()
			b, _ := os.ReadFile(p)
			content := string(b)
			aliasLine := fmt.Sprintf("\nalias %s='agent-wrapper %s' # agent-ebpf-hook\n", target.TargetCmd, target.TargetCmd)
			if !strings.Contains(content, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				newContent := content + aliasLine
				if err := platform.WriteFileAsRealUser(p, []byte(newContent), 0644); err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
			}
		}
	} else {
		if target.HookType == core.HookTypeNative {
			_ = Deps.UninstallNativeHook(target)
		}
		p := Deps.GetShellConfigPath()
		b, _ := os.ReadFile(p)
		lines := strings.Split(string(b), "\n")
		newLines := []string{}
		for _, l := range lines {
			if !strings.Contains(l, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				newLines = append(newLines, l)
			}
		}
		_ = platform.WriteFileAsRealUser(p, []byte(strings.Join(newLines, "\n")), 0644)
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleConfigHooksRawGet(c *gin.Context) {
	id := c.Param("id")
	var target core.HookDef
	found := false
	for _, h := range Deps.AvailableHooks() {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != core.HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	if target.ID == "kiro" {
		if err := Deps.EnsureKiroManagedAgentExists(); err != nil {
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

func HandleConfigHooksRawPost(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target core.HookDef
	found := false
	for _, h := range Deps.AvailableHooks() {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != core.HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	var js map[string]interface{}
	if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	if err := platform.MkdirAllAsRealUser(filepath.Dir(target.NativeConfigPath), 0755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := platform.WriteFileAsRealUser(target.NativeConfigPath, []byte(req.Content), 0644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
