package tls

import (
	"errors"
	"sync"
	"time"
)

// ── Builtin executable attach status (deprecated, kept for API compatibility) ─

// TLSBuiltinExecutableAttachStatus reports the result of attaching to a builtin TLS executable.
type TLSBuiltinExecutableAttachStatus struct {
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

// ---- moved from backend/zz_merged_backend.go section capturecontrollertls.go ----

type TLSCaptureController struct {
	mu                 sync.Mutex
	manager            *TLSProbeManager
	store              *TLSCaptureStore
	rules              *TLSCaptureRuleStore
	broadcaster        *TLSBroadcaster
	readStarted        bool
	goDiscoveryStarted bool
	lastError          string
}

func NewTLSCaptureController(store *TLSCaptureStore, rules *TLSCaptureRuleStore, broadcaster *TLSBroadcaster) *TLSCaptureController {
	if store == nil {
		store = NewTLSCaptureStore(2000)
	}
	if rules == nil {
		rules = NewTLSCaptureRuleStore()
	}
	if broadcaster == nil {
		broadcaster = NewTLSCaptureBroadcaster()
	}
	return &TLSCaptureController{store: store, rules: rules, broadcaster: broadcaster}
}

func (c *TLSCaptureController) Manager() *TLSProbeManager {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manager
}

func (c *TLSCaptureController) EnsureStarted() (*TLSProbeManager, error) {
	if c == nil {
		return nil, errors.New("TLS capture controller is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.manager != nil {
		return c.manager, nil
	}
	manager, err := NewTLSProbeManager(c.store, c.broadcaster, c.rules)
	if err != nil {
		c.lastError = err.Error()
		return nil, err
	}
	c.manager = manager
	c.startReadLoopLocked(manager)
	return manager, nil
}

func (c *TLSCaptureController) AttachDefaults() error {
	manager, err := c.EnsureStarted()
	if err != nil {
		return err
	}
	err = manager.AttachStaticLibs()
	c.startGoDiscovery(manager)
	if err != nil && c.store != nil {
		for _, library := range c.store.LibraryStatuses() {
			if library.Attached {
				c.setLastError(err)
				return nil
			}
		}
		c.setLastError(err)
		return err
	}
	c.setLastError(nil)
	return nil
}

func (c *TLSCaptureController) AttachLibrary(path, library string) error {
	manager, err := c.EnsureStarted()
	if err != nil {
		return err
	}
	if err := manager.AttachLibrary(path, library); err != nil {
		c.setLastError(err)
		return err
	}
	c.setLastError(nil)
	return nil
}

func (c *TLSCaptureController) AttachBuiltinExecutables(pid int) ([]TLSBuiltinExecutableAttachStatus, error) {
	// Deprecated: built-in executable list was replaced by auto-discovery
	return []TLSBuiltinExecutableAttachStatus{}, nil
}

func (c *TLSCaptureController) Status() map[string]any {
	if c == nil {
		return map[string]any{"enabled": false, "available": false, "error": "TLS capture controller is unavailable"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]any{
		"enabled":            c.manager != nil,
		"available":          c.manager != nil,
		"readStarted":        c.readStarted,
		"goDiscoveryStarted": c.goDiscoveryStarted,
		"error":              c.lastError,
	}
}

func (c *TLSCaptureController) AttachedPIDs() []AttachedPIDInfo {
	manager := c.Manager()
	if manager == nil {
		return nil
	}
	return manager.AttachedPIDs()
}
func (c *TLSCaptureController) ProbeHitCounters() map[string]uint64 {
	manager := c.Manager()
	if manager == nil {
		return nil
	}
	return manager.ProbeHitCounters()
}

func (c *TLSCaptureController) ReadLoopStatsSnapshot() ReadLoopStats {
	manager := c.Manager()
	if manager == nil {
		return ReadLoopStats{}
	}
	return manager.ReadLoopStatsSnapshot()
}

func (c *TLSCaptureController) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	manager := c.manager
	c.manager = nil
	c.readStarted = false
	c.goDiscoveryStarted = false
	c.mu.Unlock()
	if manager != nil {
		return manager.Close()
	}
	return nil
}

func (c *TLSCaptureController) startReadLoopLocked(manager *TLSProbeManager) {
	if c.readStarted || manager == nil {
		return
	}
	c.readStarted = true
	go func() {
		if err := manager.ReadLoop(); err != nil {
			c.setLastError(err)
		}
	}()
}

func (c *TLSCaptureController) startGoDiscovery(manager *TLSProbeManager) {
	c.mu.Lock()
	if c.goDiscoveryStarted || manager == nil {
		c.mu.Unlock()
		return
	}
	c.goDiscoveryStarted = true
	c.mu.Unlock()
	manager.StartGoDiscoveryLoop(time.Minute)
}

func (c *TLSCaptureController) setLastError(err error) {
	c.mu.Lock()
	if err == nil {
		c.lastError = ""
	} else {
		c.lastError = err.Error()
	}
	c.mu.Unlock()
}
