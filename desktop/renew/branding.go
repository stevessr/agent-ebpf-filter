package main

// The user-facing product name is independent from the long-lived renew
// executable, desktop identifier, and /renew browser route. Keep those
// technical identifiers stable for existing installs and integrations.
const (
	desktopBrandName        = "明镜高悬"
	desktopBrandSlogan      = "察微知著 · 守护 Agent 边界"
	desktopBrandDescription = "从内核事件到 Agent 会话与网络外联，让系统行为清晰可见、异常有迹可循。"
	desktopWindowTitle      = desktopBrandName + " · Agent eBPF Filter"
)
