package main

import "strings"

// An explicit address always wins over the experimental native collector.
// This also keeps Windows as a read-only remote Linux dashboard when desired.
func localWindowsMonitoringEnabled(explicitBackend, envBackend string) bool {
	return strings.TrimSpace(explicitBackend) == "" && strings.TrimSpace(envBackend) == ""
}

func windowsLocalPage(page string) bool {
	switch page {
	case "概览", "研判", "事件", "网络", "进程", "系统":
		return true
	default:
		return false
	}
}
