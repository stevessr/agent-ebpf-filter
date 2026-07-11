package tls

import (
	"errors"
	"sync"
	"time"
)

var ErrTLSCaptureDisabled = errors.New("TLS capture is disabled")

// ── Builtin executable attach status (deprecated, kept for API compatibility) ─

// TLSBuiltinExecutableAttachStatus reports the result of attaching to a builtin TLS executable.
type TLSBuiltinExecutableAttachStatus struct {
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

// ---- moved from backend/zz_merged_backend.go section capturecontrollertls.go ----

type TLSCaptureController struct {
	transitionMu       sync.Mutex
	mu                 sync.Mutex
	manager            *TLSProbeManager
	store              *TLSCaptureStore
	rules              *TLSCaptureRuleStore
	broadcaster        *TLSBroadcaster
	enabledCheck       func() bool
	accepting          bool
	readStarted        bool
	readDone           chan struct{}
	goDiscoveryStarted bool
	lastError          string
}

func (c *TLSCaptureController) SetEnabledCheck(enabled func() bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.enabledCheck = enabled
	broadcaster := c.broadcaster
	c.mu.Unlock()
	broadcaster.SetEnabledCheck(enabled)
}

func (c *TLSCaptureController) SetAccepting(accepting bool) {
	if c == nil {
		return
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	c.mu.Lock()
	c.accepting = accepting
	broadcaster := c.broadcaster
	c.mu.Unlock()
	broadcaster.SetAccepting(accepting)
}

func (c *TLSCaptureController) RunIfEnabled(run func()) bool {
	if c == nil || run == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.accepting || (c.enabledCheck != nil && !c.enabledCheck()) {
		return false
	}
	run()
	return true
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
	return &TLSCaptureController{store: store, rules: rules, broadcaster: broadcaster, accepting: true}
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
	if !c.accepting || (c.enabledCheck != nil && !c.enabledCheck()) {
		c.lastError = ErrTLSCaptureDisabled.Error()
		return nil, ErrTLSCaptureDisabled
	}
	if c.manager != nil {
		if !c.readStarted {
			c.startReadLoopLocked(c.manager)
		}
		return c.manager, nil
	}
	manager, err := NewTLSProbeManager(c.store, c.broadcaster, c.rules)
	if err != nil {
		c.lastError = err.Error()
		return nil, err
	}
	c.manager = manager
	c.lastError = ""
	c.startReadLoopLocked(manager)
	return manager, nil
}

func (c *TLSCaptureController) AttachDefaults() error {
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
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
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
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

func (c *TLSCaptureController) AttachExecutable(input string, pid int, libraryHint string) TLSExecutableAttachResult {
	if c == nil {
		return TLSExecutableAttachResult{Error: "TLS capture controller is unavailable"}
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	manager, err := c.EnsureStarted()
	if err != nil {
		return TLSExecutableAttachResult{Error: err.Error()}
	}
	result := manager.AttachExecutable(input, pid, libraryHint)
	if result.Error != "" {
		c.setLastError(errors.New(result.Error))
	} else {
		c.setLastError(nil)
	}
	return result
}

func (c *TLSCaptureController) AttachGoUprobes(path string, pid int) error {
	if c == nil {
		return errors.New("TLS capture controller is unavailable")
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	manager, err := c.EnsureStarted()
	if err != nil {
		return err
	}
	if err := manager.AttachGoUprobes(path, pid); err != nil {
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
		"broadcast":          c.broadcaster.Status(),
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
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	c.mu.Lock()
	manager := c.manager
	broadcaster := c.broadcaster
	readDone := c.readDone
	c.accepting = false
	c.manager = nil
	c.readStarted = false
	c.readDone = nil
	c.goDiscoveryStarted = false
	c.mu.Unlock()
	var err error
	if manager != nil {
		err = manager.Close()
	}
	if readDone != nil {
		<-readDone
	}
	broadcaster.Close()
	return err
}

func (c *TLSCaptureController) startReadLoopLocked(manager *TLSProbeManager) {
	if c.readStarted || manager == nil {
		return
	}
	c.readStarted = true
	done := make(chan struct{})
	c.readDone = done
	go func() {
		defer func() {
			c.mu.Lock()
			if c.readDone == done {
				c.readStarted = false
				c.readDone = nil
			}
			c.mu.Unlock()
			close(done)
		}()
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
