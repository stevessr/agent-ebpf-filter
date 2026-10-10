package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/agentscope"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

// Agent scope policy is independent of HTTP and kernel-enforcement control.
// Keep these aliases for the existing app tests, routes and JSON contract.
type agentScopeList = agentscope.List
type agentScopePolicy = agentscope.Policy

const (
	agentScopeBlacklist  = agentscope.ModeBlacklist
	agentScopeWhitelist  = agentscope.ModeWhitelist
	agentScopeMaxEntries = agentscope.MaxEntries
	agentScopeFileName   = "agent-scopes.json"
)

var agentScopes struct {
	loadOnce sync.Once
	writeMu  sync.Mutex
	snapshot atomic.Pointer[agentScopePolicy]
}

func defaultAgentScopePolicy() agentScopePolicy {
	return agentscope.Default()
}

func agentScopePath() string {
	return filepath.Join(platform.RuntimeSettingsDir(), agentScopeFileName)
}

func validateAgentScopeList(list agentScopeList) (agentScopeList, error) {
	return agentscope.ValidateList(list)
}

func validateAgentScopePolicy(input agentScopePolicy) (agentScopePolicy, error) {
	return agentscope.Validate(input)
}

func currentAgentScopePolicy() *agentScopePolicy {
	agentScopes.loadOnce.Do(func() {
		cfg := defaultAgentScopePolicy()
		stored, err := agentscope.LoadFile(agentScopePath())
		if err == nil {
			cfg = stored
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("[WARN] invalid or unreadable agent scopes: %v (using defaults)", err)
		}
		agentScopes.snapshot.Store(&cfg)
	})
	return agentScopes.snapshot.Load()
}

// User-facing scope matching lives in a transport-independent component.
func agentScopeAllows(list agentScopeList, comm, tag string) bool {
	return agentscope.Allows(list, comm, tag)
}

func captureAgentEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	// The event has already passed EnrichEventContext. Remember trustworthy
	// root identity before whitelist admission so subsequent script-driven
	// edits can be matched by the originating Agent (e.g. codex -> python).
	agentRootScopes.Observe(event)
	return agentScopeAllowsWithRoot(currentAgentScopePolicy().Capture, event, agentRootScopes.OwnerComm(event))
}

func monitorAgentEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return agentScopeAllowsWithRoot(currentAgentScopePolicy().Monitor, event, agentRootScopes.OwnerComm(event))
}

func persistAgentScopePolicy(cfg agentScopePolicy) error {
	return agentscope.SaveFile(agentScopePath(), cfg)
}

func handleAgentScopesGet(c *gin.Context) {
	c.JSON(200, currentAgentScopePolicy())
}

func handleAgentScopesPut(c *gin.Context) {
	var requested agentScopePolicy
	if err := c.ShouldBindJSON(&requested); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	normalized, err := validateAgentScopePolicy(requested)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	agentScopes.writeMu.Lock()
	defer agentScopes.writeMu.Unlock()
	// Ensure the initial lazy load cannot overwrite a newly accepted update.
	currentAgentScopePolicy()
	if err := persistAgentScopePolicy(normalized); err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("could not save agent scopes: %v", err)})
		return
	}
	agentScopes.snapshot.Store(&normalized)
	c.JSON(200, normalized)
}
