package app

import (
	"encoding/json"
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
	agentScopeBlacklist = agentscope.ModeBlacklist
	agentScopeWhitelist = agentscope.ModeWhitelist
	agentScopeMaxEntries = agentscope.MaxEntries
	agentScopeFileName = "agent-scopes.json"
)

var agentScopes struct {
	loadOnce sync.Once
	writeMu  sync.Mutex
	snapshot atomic.Pointer[agentScopePolicy]
}

func defaultAgentScopePolicy() agentScopePolicy {
	return agentscope.Default()
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
		raw, err := os.ReadFile(agentScopePath())
		if err == nil {
			var stored agentScopePolicy
			if json.Unmarshal(raw, &stored) == nil {
				if normalized, validationErr := validateAgentScopePolicy(stored); validationErr == nil {
					cfg = normalized
				} else {
					log.Printf("[WARN] invalid agent scopes: %v (using defaults)", validationErr)
				}
			} else {
				log.Printf("[WARN] cannot decode agent scopes (using defaults)")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("[WARN] cannot read agent scopes: %v (using defaults)", err)
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := agentScopePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".agent-scopes-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
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
