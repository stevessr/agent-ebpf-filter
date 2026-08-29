package tls

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrTLSCaptureDisabled   = errors.New("TLS capture is disabled")
	ErrBpfTSTLSModeConflict = errors.New("conflicting TLS capture backend is already active")
)

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
	bpfTSBridge        *BpfTSOpenSSLBridgeRuntime
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
		bpfTSBridge: NewBpfTSOpenSSLBridgeRuntime(store, rules, broadcaster),
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
	// Shadow mode is explicitly observational and may coexist with the legacy
	// perf path. The data-bearing bpf-ts bridge publishes into the same store,
	// broadcaster and AgentSight pipeline, so starting legacy capture alongside
	// an active bridge would duplicate every captured TLS event.
	if c.bpfTSBridge != nil && c.bpfTSBridge.Status().Active {
		c.lastError = ErrBpfTSTLSModeConflict.Error()
		return nil, ErrBpfTSTLSModeConflict
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
// captured plaintext. Shadow and bridge modes are intentionally mutually
// exclusive so the generated OpenSSL probes cannot be attached twice.
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
	bridge := c.bpfTSBridge
	c.mu.Unlock()
	if !accepting || !enabled {
		c.setLastError(ErrTLSCaptureDisabled)
		return ErrTLSCaptureDisabled
	}
	if bridge != nil && bridge.Status().Active {
		c.setLastError(ErrBpfTSTLSModeConflict)
		return ErrBpfTSTLSModeConflict
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

// StartBpfTSOpenSSLBridge starts the data-bearing bpf-ts OpenSSL path. Shadow
// mode is the A/B comparison path; bridge mode is an actual publisher cutover
// and therefore cannot coexist with either shadow or the legacy perf manager.
func (c *TLSCaptureController) StartBpfTSOpenSSLBridge(config BpfTSOpenSSLBridgeConfig) error {
	if c == nil {
		return errors.New("TLS capture controller is unavailable")
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()

	c.mu.Lock()
	accepting := c.accepting
	enabled := c.enabledCheck == nil || c.enabledCheck()
	legacyActive := c.manager != nil
	shadow := c.bpfTSShadow
	bridge := c.bpfTSBridge
	store := c.store
	rules := c.rules
	broadcaster := c.broadcaster
	c.mu.Unlock()
	if !accepting || !enabled {
		c.setLastError(ErrTLSCaptureDisabled)
		return ErrTLSCaptureDisabled
	}
	if legacyActive || (shadow != nil && shadow.Status().Active) {
		c.setLastError(ErrBpfTSTLSModeConflict)
		return ErrBpfTSTLSModeConflict
	}
	if bridge == nil {
		bridge = NewBpfTSOpenSSLBridgeRuntime(store, rules, broadcaster)
		c.mu.Lock()
		if c.bpfTSBridge == nil {
			c.bpfTSBridge = bridge
		} else {
			bridge = c.bpfTSBridge
		}
		c.mu.Unlock()
	}
	if err := bridge.Start(config); err != nil {
		c.setLastError(err)
		return err
	}
	c.setLastError(nil)
	return nil
}

func (c *TLSCaptureController) StopBpfTSOpenSSLBridge() error {
	if c == nil {
		return nil
	}
	c.transitionMu.Lock()
	defer c.transitionMu.Unlock()
	c.mu.Lock()
	bridge := c.bpfTSBridge
	c.mu.Unlock()
	if bridge == nil {
		return nil
	}
	if err := bridge.Stop(); err != nil {
		c.setLastError(err)
		return err
	}
	return nil
}

func (c *TLSCaptureController) BpfTSOpenSSLBridgeStatus() BpfTSOpenSSLBridgeStatus {
	if c == nil {
		return BpfTSOpenSSLBridgeStatus{LastError: "TLS capture controller is unavailable"}
	}
	c.mu.Lock()
	bridge := c.bpfTSBridge
	c.mu.Unlock()
	if bridge == nil {
		return BpfTSOpenSSLBridgeStatus{}
	}
	return bridge.Status()
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
	bridge := c.bpfTSBridge
	c.mu.Unlock()

	shadowStatus := BpfTSTLSShadowStatus{}
	if shadow != nil {
		shadowStatus = shadow.Status()
	}
	bridgeStatus := BpfTSOpenSSLBridgeStatus{}
	backpressureStatus := BpfTSOpenSSLBackpressureStatus{}
	if bridge != nil {
		bridgeStatus = bridge.Status()
		backpressureStatus = bridge.BackpressureStatus()
	}

	status := map[string]any{
		"enabled":                 manager != nil,
		"available":               manager != nil,
		"captureActive":           manager != nil || shadowStatus.Active || bridgeStatus.Active,
		"readStarted":             readStarted,
		"goDiscoveryStarted":      discoveryStarted,
		"autoDiscoveryIntervalMs": tlsAutoDiscoveryInterval.Milliseconds(),
		"error":                   lastError,
		"broadcast":               broadcaster.Status(),
		"bpfTsShadow":             shadowStatus,
		"bpfTsBridge":             bridgeStatus,
		"bpfTsWireEfficiency":     bpfTSOpenSSLWireEfficiency(bridgeStatus),
		"bpfTsBackpressure":       backpressureStatus,
	}
	if manager != nil {
		status["autoDiscovery"] = manager.AutoDiscoveryStatus()
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
	bridge := c.bpfTSBridge
	broadcaster := c.broadcaster
	readDone := c.readDone
	c.accepting = false
	c.manager = nil
	c.readStarted = false
	c.readDone = nil
	c.goDiscoveryStarted = false
	c.mu.Unlock()

	var errs []error
	// Stop bpf-ts readers before dismantling the legacy manager so no bpf-ts
	// runtime can continue to publish into a partially torn-down controller.
	if bridge != nil {
		errs = append(errs, bridge.Close())
	}
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
