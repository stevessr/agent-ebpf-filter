package app

// TLSBuiltinExecutableAttachStatus 内置可执行文件附加状态（已废弃，保留以兼容 API）
type TLSBuiltinExecutableAttachStatus struct {
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

// AttachBuiltinExecutables 附加内置可执行文件（已废弃，现在使用自动发现）
func (m *TLSProbeManager) AttachBuiltinExecutables(pid int) []TLSBuiltinExecutableAttachStatus {
	// 旧的内置列表功能已被自动发现取代
	// 返回空列表表示使用自动发现机制
	return []TLSBuiltinExecutableAttachStatus{}
}

// builtinTLSExecutableTargetList 返回内置目标列表（已废弃）
func builtinTLSExecutableTargetList() []map[string]string {
	return []map[string]string{
		{"name": "node", "description": "Node.js (auto-discovered)"},
		{"name": "codex", "description": "OpenAI Codex (auto-discovered via rustls)"},
		{"name": "claude-code", "description": "Claude Code (auto-discovered)"},
	}
}
