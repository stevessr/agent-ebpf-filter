package main

import (
	"path/filepath"
	"strconv"
	"strings"
)

// Related-context navigation is read-only by default. It uses compact,
// already-captured evidence, not process-name guesses or hidden web requests.
// Each jump sets explicit filters so it remains reproducible on refresh.
func (a *renewApp) beginEventDrilldown() {
	if a.page != "事件" {
		a.eventReturnPage = a.page
	}
	a.clearEventFilters()
	a.page = "事件"
}

func (a *renewApp) navigatePIDEvents(pid int) bool {
	if pid <= 0 {
		return false
	}
	a.beginEventDrilldown()
	a.eventPIDFilter = pid
	return true
}

func (a *renewApp) navigateAgentEvents(rootPID int) bool {
	if rootPID <= 0 {
		return false
	}
	a.beginEventDrilldown()
	a.eventRootPIDFilter = rootPID
	return true
}

func (a *renewApp) navigateSessionEvents(sessionKey string, fileEditsOnly, delegatedOnly bool) bool {
	if strings.TrimSpace(sessionKey) == "" {
		return false
	}
	a.beginEventDrilldown()
	a.eventSessionFilter = sessionKey
	a.eventFileEditsOnly = fileEditsOnly || delegatedOnly
	a.eventDelegatedOnly = delegatedOnly
	return true
}

func (a *renewApp) navigateTargetEvents(target string) bool {
	if !usableEventTarget(target) {
		return false
	}
	a.beginEventDrilldown()
	a.eventTargetFilter = target
	return true
}

func (a *renewApp) navigateRecognition(pid int, comm string) bool {
	query := strings.TrimSpace(comm)
	if pid > 0 {
		query = strconv.Itoa(pid)
	}
	if query == "" {
		return false
	}
	a.agentSearch = query
	a.agentSelected = -1
	a.page = "Agent 识别"
	return true
}

func safeExactFilePath(target string) bool {
	// A file description, masked/redacted field or URI must not silently
	// become an exact LSM path policy draft.
	return filepath.IsAbs(target) && filepath.Clean(target) == target &&
		strings.TrimSpace(target) == target && !strings.ContainsAny(target, "\x00\r\n")
}

func (a *renewApp) navigatePathAccess(target string) bool {
	if !safeExactFilePath(target) {
		return false
	}
	a.pathAccessTarget = target
	a.pathAccessConfirm = false
	a.pathAccessConfirmText = ""
	a.pathAccessPending = fileAccessRule{}
	a.page = "路径权限"
	return true
}

func usableEventTarget(target string) bool {
	target = strings.TrimSpace(target)
	return target != "" && target != "-" && target != "目标路径或端点未记录" &&
		target != "无文件或网络操作对象"
}

// A related file edit must be a real mutation by a confirmed descendant.
// A generic executor tagged Shell/Runtime is never enough by itself.
func matchesEventDrilldown(e eventSummary, rootPID int, target string, fileOnly, delegatedOnly bool) bool {
	if rootPID > 0 && e.RootAgentPID != rootPID && e.PID != rootPID {
		return false
	}
	if target != "" && e.Target != target {
		return false
	}
	if (fileOnly || delegatedOnly) && !isFileMutationSummary(e) {
		return false
	}
	if delegatedOnly && !(e.RootAgentPID > 0 && e.PID > 0 && e.RootAgentPID != e.PID) {
		return false
	}
	return true
}
