package main

// overviewSnapshotHint explains whether the home screen shows live or stale
// evidence. A quiet summary is never evidence that an unobserved action is safe.
func (a *renewApp) overviewSnapshotHint() string {
	switch {
	case a.starting:
		return "正在建立连接。采集状态确认前，请勿将空白记录视为安全。"
	case !a.connected:
		return "连接已中断；保留的记录可能过时，新的活动暂时无法确认。"
	case !a.healthReady:
		return "正在确认采集器状态，当前记录可能尚未完整同步。"
	case a.paused:
		return "您暂停的是桌面事件展示，不是后台采集；恢复画面后可继续查看更新。"
	case !a.health.CaptureHealthy:
		return "采集链路异常，可能漏记活动；建议打开系统诊断。"
	case !a.eventStreamConnected:
		return "事件流正在使用回退同步方式，新活动可能延迟显示。"
	case a.lastSync.IsZero():
		return "监控服务已连接，但尚未记录最近同步时间。"
	default:
		return "最近同步 " + a.lastSync.Format("15:04:05") + " · 这里只显示近期摘要，不是完整的安全结论。"
	}
}

// The personal homepage shows the last few unfiltered summaries. Hidden
// filters from a previous Events visit must not masquerade as inactivity.
func (a *renewApp) overviewRecentEvents() []eventSummary {
	return a.events[:min(len(a.events), 5)]
}
