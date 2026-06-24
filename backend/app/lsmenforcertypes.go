package app

import (
	"errors"
	"os"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// ---- moved from backend/zz_merged_backend.go section lsmenforcertypes.go ----

// ── BPF LSM enforcement map and link management ────────────────────────

const lsmEnforcerPinRoot = ebpfPinRoot + "/lsm_enforcer"
const lsmEnforcerMapsDir = lsmEnforcerPinRoot + "/maps"
const lsmEnforcerLinksDir = lsmEnforcerPinRoot + "/links"
const lsmEnforcerMapPinMode os.FileMode = 0600
const expectedLsmEnforcerLinks = 14

type lsmEnforcerRuntime struct {
	ExecPathBlocklist *ebpf.Map
	ExecNameBlocklist *ebpf.Map
	FileNameBlocklist *ebpf.Map
	Stats             *ebpf.Map
	Links             []link.Link
	LinkPins          []string
	LastError         string
}

type lsmPathKey struct {
	Path [256]byte
}

type lsmNameKey struct {
	Name [64]byte
}

type lsmEnforcerStats struct {
	ExecChecked uint64 `json:"execChecked"`
	ExecBlocked uint64 `json:"execBlocked"`
	FileChecked uint64 `json:"fileChecked"`
	FileBlocked uint64 `json:"fileBlocked"`
}

var lsmEnforcer lsmEnforcerRuntime
var lsmEnforcerMu sync.RWMutex
var errLsmEnforcerPinnedLinksMissing = errors.New("BPF LSM pinned links missing")

type lsmEnforcerSnapshot struct {
	ExecPathBlocklist *ebpf.Map
	ExecNameBlocklist *ebpf.Map
	FileNameBlocklist *ebpf.Map
	Stats             *ebpf.Map
	LinkCount         int
	LinkPins          []string
	LastError         string
}

func currentLsmEnforcerSnapshot() lsmEnforcerSnapshot {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerSnapshot{
		ExecPathBlocklist: lsmEnforcer.ExecPathBlocklist,
		ExecNameBlocklist: lsmEnforcer.ExecNameBlocklist,
		FileNameBlocklist: lsmEnforcer.FileNameBlocklist,
		Stats:             lsmEnforcer.Stats,
		LinkCount:         len(lsmEnforcer.Links),
		LinkPins:          append([]string(nil), lsmEnforcer.LinkPins...),
		LastError:         lsmEnforcer.LastError,
	}
}

func (s lsmEnforcerSnapshot) available() bool {
	return s.ExecPathBlocklist != nil && s.ExecNameBlocklist != nil && s.FileNameBlocklist != nil && s.Stats != nil
}

func (s lsmEnforcerSnapshot) attached() bool {
	return s.LinkCount >= expectedLsmEnforcerLinks
}

func lsmEnforcerAvailable() bool {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerAvailableLocked()
}

func lsmEnforcerAvailableLocked() bool {
	return lsmEnforcer.ExecPathBlocklist != nil &&
		lsmEnforcer.ExecNameBlocklist != nil &&
		lsmEnforcer.FileNameBlocklist != nil &&
		lsmEnforcer.Stats != nil
}

func lsmEnforcerAttached() bool {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerAttachedLocked()
}

func lsmEnforcerAttachedLocked() bool {
	return len(lsmEnforcer.Links) >= expectedLsmEnforcerLinks
}

func closeLsmEnforcerLinks() {
	for _, existing := range lsmEnforcer.Links {
		_ = existing.Close()
	}
	lsmEnforcer.Links = nil
	lsmEnforcer.LinkPins = nil
}

func replaceLsmEnforcerLinks(links []link.Link) {
	closeLsmEnforcerLinks()
	lsmEnforcer.Links = links
}
