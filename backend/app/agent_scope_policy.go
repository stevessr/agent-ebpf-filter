package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

// Agent scope lists are independent: capture controls event admission to the
// userspace archive/stream, while monitor controls semantic alerts and the
// downstream analysis queues. Neither list is an execution-deny policy.
type agentScopeList struct {
	Mode    string   `json:"mode"`
	Entries []string `json:"entries"`
}

type agentScopePolicy struct {
	Capture agentScopeList `json:"capture"`
	Monitor agentScopeList `json:"monitor"`
}

const (
	agentScopeBlacklist = "blacklist"
	agentScopeWhitelist = "whitelist"
	agentScopeMaxEntries = 256
	agentScopeFileName   = "agent-scopes.json"
)

var agentScopes struct {
	loadOnce sync.Once
	writeMu  sync.Mutex
	snapshot atomic.Pointer[agentScopePolicy]
}

func defaultAgentScopePolicy() agentScopePolicy {
	return agentScopePolicy{
		Capture: agentScopeList{Mode: agentScopeBlacklist, Entries: []string{}},
		Monitor: agentScopeList{Mode: agentScopeBlacklist, Entries: []string{}},
	}
}

func agentScopePath() string {
	return filepath.Join(platform.RuntimeSettingsDir(), agentScopeFileName)
}

func validateAgentScopeList(list agentScopeList) (agentScopeList, error) {
	if list.Mode != agentScopeBlacklist && list.Mode != agentScopeWhitelist {
		return agentScopeList{}, fmt.Errorf("mode must be blacklist or whitelist")
	}
	if len(list.Entries) > agentScopeMaxEntries {
		return agentScopeList{}, fmt.Errorf("too many entries (max %d)", agentScopeMaxEntries)
	}
	out := agentScopeList{Mode: list.Mode, Entries: make([]string, 0, len(list.Entries))}
	seen := make(map[string]struct{}, len(list.Entries))
	for _, raw := range list.Entries {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if !utf8.ValidString(entry) || len(entry) == 0 || len(entry) > 128 ||
			strings.ContainsAny(entry, "\x00\r\n\t") {
			return agentScopeList{}, errors.New("entries must be nonempty UTF-8 names (max 128 bytes)")
		}
		if _, found := seen[entry]; !found {
			seen[entry] = struct{}{}
			out.Entries = append(out.Entries, entry)
		}
	}
	return out, nil
}

func validateAgentScopePolicy(input agentScopePolicy) (agentScopePolicy, error) {
	capture, err := validateAgentScopeList(input.Capture)
	if err != nil {
		return agentScopePolicy{}, fmt.Errorf("capture: %w", err)
	}
	monitor, err := validateAgentScopeList(input.Monitor)
	if err != nil {
		return agentScopePolicy{}, fmt.Errorf("monitor: %w", err)
	}
	return agentScopePolicy{Capture: capture, Monitor: monitor}, nil
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

// Exact, case-insensitive match against the process comm or resolved Agent tag.
// No substring, regexp, or PID matching: PID reuse must not affect admission.
// In whitelist mode an empty list rejects all; empty blacklists allow all.
func agentScopeAllows(list agentScopeList, comm, tag string) bool {
	found := false
	for _, entry := range list.Entries {
		if strings.EqualFold(entry, strings.TrimSpace(comm)) ||
			strings.EqualFold(entry, strings.TrimSpace(tag)) {
			found = true
			break
		}
	}
	if list.Mode == agentScopeWhitelist {
		return found
	}
	return !found
}

func captureAgentEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return agentScopeAllows(currentAgentScopePolicy().Capture, event.GetComm(), event.GetTag())
}

func monitorAgentEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return agentScopeAllows(currentAgentScopePolicy().Monitor, event.GetComm(), event.GetTag())
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
