package main

import "testing"

func TestWindowsLocalMonitoringRequiresNoExplicitBackend(t *testing.T) {
	if !localWindowsMonitoringEnabled("", "") ||
		localWindowsMonitoringEnabled("127.0.0.1:8080", "") ||
		localWindowsMonitoringEnabled("", "http://localhost:8080") {
		t.Fatal("an explicitly selected backend must not be replaced by the local collector")
	}
}
func TestWindowsLocalPagesAreReadOnly(t *testing.T) {
	for _, page := range []string{"概览", "事件", "研判", "网络", "进程", "系统"} {
		if !windowsLocalPage(page) { t.Fatalf("missing read-only page %s", page) }
	}
	for _, page := range []string{"会话", "监控", "规则", "跟踪", "Agent 识别", "eBPF 模块", "路径权限", "终端", "域名", "CCS"} {
		if windowsLocalPage(page) { t.Fatalf("unsupported local capability exposed: %s", page) }
	}
}
