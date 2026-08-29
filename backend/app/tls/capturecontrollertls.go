package tls

import (
	"errors"
	"sync"
	"time"
)

var ErrTLSCaptureDisabled = errors.New("TLS capture is disabled")

const tlsAutoDiscoveryInterval = 5 * time.Second

// TLSBuiltinExecutableAttachStatus reports the result of attaching to a builtin TLS executable.
type TLSBuiltinExecutableAttachStatus struct {
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

type TLSCaptureController struct {
	transitionMu       sync.Mutex
	mu                 sync.Mutex
	manager            *TLSProbeManager
	bpfTSShadow        *BpfTSTLSShadowRuntime
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
	return &TLSCaptureController{
		store:       store,
		rules:       rules,
		broadcaster: broadcaster,
		bpfTSShadow: NewBpfTSTLSShadowRuntime(),
		accepting:   true,
	}
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
	return []TLSBuiltinExecutableAttachStatus{}, nil
}

// StartBpfTSShadow starts an explicitly configured bpf-ts TLS runtime alongside
// the production capture path. It never replaces the production manager and the
// shadow only counts ringbuf records/bytes; it does not persist a second copy of
// captured plaintext.
func (c *TLSCaptureController) StartBpfTSShadow(config BpfTSTLSShadowConfig) error {
	if c == nil {
		return errors.New("TLS capture controller is unavailable")
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()

	c.mu.Lock()
	accepting := c.accepting
	enabled := c.enabledCheck == nil || c.enabledCheck()
	shadow := c.bpfTSShadow
	c.mu.Unlock()
	if !accepting || !enabled {
		c.setLastError(ErrTLSCaptureDisabled)
		return ErrTLSCaptureDisabled
	}
	if shadow == nil {
		shadow = NewBpfTSTLSShadowRuntime()
		c.mu.Lock()
		if c.bpfTSShadow == nil {
			c.bpfTSShadow = shadow
		} else {
			shadow = c.bpfTSShadow
		}
		c.mu.Unlock()
	}
	if err := shadow.Start(config); err != nil {
		c.setLastError(err)
		return err
	}
	c.setLastError(nil)
	return nil
}

func (c *TLSCaptureController) StopBpfTSShadow() error {
	if c == nil {
		return nil
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	c.mu.Lock()
	shadow := c.bpfTSShadow
	c.mu.Unlock()
	if shadow == nil {
		return nil
	}
	if err := shadow.Stop(); err != nil {
		c.setLastError(err)
		return err
	}
	return nil
}

func (c *TLSCaptureController) BpfTSShadowStatus() BpfTSTLSShadowStatus {
	if c == nil {
		return BpfTSTLSShadowStatus{LastError: "TLS capture controller is unavailable"}
	}
	c.mu.Lock()
	shadow := c.bpfTSShadow
	c.mu.Unlock()
	if shadow == nil {
		return BpfTSTLSShadowStatus{}
	}
	return shadow.Status()
}

func (c *TLSCaptureController) Status() map[string]any {
	if c == nil {
		return map[string]any{"enabled": false, "available": false, "error": "TLS capture controller is unavailable"}
	}
	c.mu.Lock()
	manager := c.manager
	readStarted := c.readStarted
	discoveryStarted := c.goDiscoveryStarted
	lastError := c.lastError
	broadcaster := c.broadcaster
	shadow := c.bpfTSShadow
	c.mu.Unlock()

	status := map[string]any{
		"enabled":                 manager != nil,
		"available":               manager != nil,
		"readStarted":             readStarted,
		"goDiscoveryStarted":      discoveryStarted,
		"autoDiscoveryIntervalMs": tlsAutoDiscoveryInterval.Milliseconds(),
		"error":                   lastError,
		"broadcast":               broadcaster.Status(),
	}
	if manager != nil {
		status["autoDiscovery"] = manager.AutoDiscoveryStatus()
	}
	if shadow != nil {
		status["bpfTsShadow"] = shadow.Status()
	} else {
		status["bpfTsShadow"] = BpfTSTLSShadowStatus{}
	}
	return status
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
	shadow := c.bpfTSShadow
	broadcaster := c.broadcaster
	readDone := c.readDone
	c.accepting = false
	c.manager = nil
	c.readStarted = false
	c.readDone = nil
	c.goDiscoveryStarted = false
	c.mu.Unlock()

	var errs []error
	// Stop the observational shadow first so it cannot outlive or observe a
	// partially torn-down production TLS runtime.
	if shadow != nil {
		errs = append(errs, shadow.Close())
	}
	if manager != nil {
		errs = append(errs, manager.Close())
	}
	if readDone != nil {
		<-readDone
	}
	broadcaster.Close()
	return errors.Join(errs...)
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
	manager.StartGoDiscoveryLoop(tlsAutoDiscoveryInterval)
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
