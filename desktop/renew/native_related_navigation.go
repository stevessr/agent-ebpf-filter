package main

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
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
	return target != "" && target != "-" && target != "file write" &&
		target != "socket write" && target != "目标路径或端点未记录" &&
		target != "无文件或网络操作对象" && !strings.ContainsAny(target, "\x00\r\n")
}

// A related file edit must be a real mutation by a confirmed descendant.
// A generic executor tagged Shell/Runtime is never enough by itself.
func matchesEventDrilldown(e eventSummary, rootPID int, target string, fileOnly, delegatedOnly bool) bool {
	if rootPID > 0 &&
		e.RootAgentPID != rootPID &&
		!(e.PID == rootPID && e.RootAgentPID == 0) {
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

 
// eventQuickLinks exposes actionable, evidence-backed relationships at the
// point of investigation. The actual executor and the owning Agent get
// separate buttons; a same-PID drilldown is not a substitute for root lineage.
func (a *renewApp) eventQuickLinks(c *ui.Context, event eventSummary, withPolicy bool) {
	owner := buildAgentOwnershipIndex(a.events, nil).attribution(event)
	ui.Row(c).Wrap().Gap(6).Children(func() {
		if event.PID > 0 && ui.Button(c, "执行 PID").Tooltip("精确过滤本次系统调用的实际进程").Clicked() {
			a.navigatePIDEvents(event.PID)
		}
		if event.PPID > 0 && event.PPID != event.PID &&
			ui.Button(c, "父 PID").Tooltip("查看直接父进程的事件，不等同于根 Agent").Clicked() {
			a.navigatePIDEvents(event.PPID)
		}
		if owner.Indirect && owner.OwnerPID > 0 &&
			ui.Button(c, "归属 Agent").Tooltip("查看此根 Agent 及其子进程的事件").Clicked() {
			a.navigateAgentEvents(owner.OwnerPID)
		}
		if isAgentSummary(event) && eventSessionKey(event) != "" &&
			ui.Button(c, "同会话").Tooltip("按运行 ID 或已知根 PID 关联会话").Clicked() {
			a.navigateSessionEvents(eventSessionKey(event), false, false)
		}
		if isFileMutationSummary(event) && isAgentSummary(event) {
			if ui.Button(c, "本会话文件修改").Clicked() {
				a.navigateSessionEvents(eventSessionKey(event), true, false)
			}
			if owner.Indirect && ui.Button(c, "委托编辑").Clicked() {
				a.navigateSessionEvents(eventSessionKey(event), true, true)
			}
		}
		if usableEventTarget(event.Target) && ui.Button(c, "同目标").Tooltip("按摘要目标精确匹配，不做全文模糊搜索").Clicked() {
			a.navigateTargetEvents(event.Target)
		}
		if owner.IsAgent && owner.OwnerPID > 0 &&
			ui.Button(c, "Agent 识别").Tooltip("定位 Agent 根 PID 的识别与捕获范围").Clicked() {
			a.navigateRecognition(owner.OwnerPID, owner.OwnerComm)
		}
		if withPolicy && isFileMutationSummary(event) && safeExactFilePath(event.Target) &&
			ui.Button(c, "路径权限").Tooltip("仅填入文件路径，不会创建或启用阻断策略").Clicked() {
			a.navigatePathAccess(event.Target)
		}
	})
}

 
// Modal links close the detail and release its sensitive payload before
// changing pages/filters. Nothing navigates to an unknown or inferred value.
func (a *renewApp) eventDetailQuickLinks(c *ui.Context, event eventSummary) {
	owner := buildAgentOwnershipIndex(a.events, nil).attribution(event)
	ui.Row(c).Wrap().Gap(6).Children(func() {
		if event.PID > 0 && ui.Button(c, "执行 PID 事件").Clicked() {
			a.navigatePIDEvents(event.PID)
			a.closeEventDetail()
		}
		if event.PPID > 0 && event.PPID != event.PID &&
			ui.Button(c, "父 PID 事件").Tooltip("按真实记录的 PPID 跳转").Clicked() {
			a.navigatePIDEvents(event.PPID)
			a.closeEventDetail()
		}
		if owner.Indirect && owner.OwnerPID > 0 &&
			ui.Button(c, "根 Agent 全部活动").Clicked() {
			a.navigateAgentEvents(owner.OwnerPID)
			a.closeEventDetail()
		}
		if isAgentSummary(event) && eventSessionKey(event) != "" &&
			ui.Button(c, "同 Agent 会话").Clicked() {
			a.navigateSessionEvents(eventSessionKey(event), false, false)
			a.closeEventDetail()
		}
		if isFileMutationSummary(event) && isAgentSummary(event) &&
			ui.Button(c, "会话文件修改").Clicked() {
			a.navigateSessionEvents(eventSessionKey(event), true, false)
			a.closeEventDetail()
		}
		if isFileMutationSummary(event) && owner.Indirect &&
			ui.Button(c, "委托编辑记录").Clicked() {
			a.navigateSessionEvents(eventSessionKey(event), true, true)
			a.closeEventDetail()
		}
		if usableEventTarget(event.Target) && ui.Button(c, "同目标事件").Clicked() {
			a.navigateTargetEvents(event.Target)
			a.closeEventDetail()
		}
		if event.Type != "" && ui.Button(c, "同类型事件").Clicked() {
			a.openEventFilter(event, "type")
			a.closeEventDetail()
		}
		if owner.IsAgent && owner.OwnerPID > 0 && ui.Button(c, "定位 Agent 识别").Clicked() {
			a.navigateRecognition(owner.OwnerPID, owner.OwnerComm)
			a.closeEventDetail()
		}
		if isFileMutationSummary(event) && safeExactFilePath(event.Target) &&
			ui.Button(c, "路径访问策略").Tooltip("只预填目标路径，不会立即阻断").Clicked() {
			a.navigatePathAccess(event.Target)
			a.closeEventDetail()
		}
	})
}
