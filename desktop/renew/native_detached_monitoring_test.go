package main

import "testing"

func TestDetachableMonitoringPageWhitelist(t *testing.T) {
    for _, page := range []string{"概览", "研判", "事件", "会话", "网络", "域名", "进程", "系统"} {
        if !detachableMonitoringPage(page) {
            t.Fatalf("monitoring page %q should pop out", page)
        }
    }
    for _, page := range []string{"终端", "规则", "跟踪", "路径权限", "CCS", "未知"} {
        if detachableMonitoringPage(page) {
            t.Fatalf("privileged/tool page %q must not use passive snapshot detachment", page)
        }
    }
}

func TestDetachedMonitoringSnapshotKeepsWindowLocalFilters(t *testing.T) {
    parent := newRenewApp("http://127.0.0.1:8080")
    child := newRenewApp(parent.backend)
    parent.events = []eventSummary{{EventID: "one"}, {EventID: "two"}}
    parent.eventsVersion = 2
    parent.connected = true
    child.search = "my local query"
    child.eventRiskFilter = "高风险"
    parent.copyMonitoringSnapshot(child)
    if len(child.events) != 2 || !child.connected || child.eventsVersion != 2 {
        t.Fatal("new monitoring window did not get the shared compact snapshot")
    }
    if child.search != "my local query" || child.eventRiskFilter != "高风险" {
        t.Fatal("sync must not replace a floating window's local filters")
    }
    child.events[0].EventID = "changed"
    if parent.events[0].EventID != "one" {
        t.Fatal("snapshot events should not alias mutable parent slices")
    }
    parent.copyMonitoringSnapshot(child)
    if child.events[0].EventID != "changed" {
        t.Fatal("unchanged event versions must not overwrite window-local state")
    }
    parent.events = append(parent.events, eventSummary{EventID: "three"})
    parent.eventsVersion++
    parent.copyMonitoringSnapshot(child)
    if len(child.events) != 3 || child.events[2].EventID != "three" {
        t.Fatal("a changed event version must refresh the detached window")
    }
}
