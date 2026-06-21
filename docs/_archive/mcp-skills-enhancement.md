# MCP 服务与 Skills 优化说明

## 优化内容

### 1. 扩展 MCP 服务工具集

在 `backend/app/server__server_mcp.go` 中新增了以下 MCP 工具：

#### 配置管理工具
- `add_tracked_command` — 添加追踪命令到 eBPF map
- `add_tracked_path` — 添加追踪路径到 eBPF map

#### 事件查询工具
- `query_events` — 按条件过滤事件（支持 eventType、comm、pid 过滤）

#### 监控工具
- `get_network_flows` — 获取当前网络流量摘要（TCP/UDP 连接状态）
- `get_system_health` — 获取系统健康状态（collector、bootstrap、OTLP、enforcement）

#### 安全策略工具（需要 policyManagementEnabled）
- `block_network_destination` — 使用 cgroup sandbox 阻止 IP 地址或端口
- `block_process_cgroup` — 阻止特定 PID 所在 cgroup 的所有网络流量
- `block_file_access` — 使用 BPF LSM 阻止文件访问或程序执行

### 2. 创建三个 Claude Code Skills

#### configure-security
- **路径**: `.claude/skills/configure-security/SKILL.md`
- **功能**: 配置安全策略，包括：
  - 追踪命令和路径管理
  - Wrapper 规则配置
  - Cgroup 网络拦截（IP/端口/进程）
  - BPF LSM 文件/执行拦截
- **使用场景**: 
  - 阻止 AI Agent 访问外网
  - 保护敏感文件
  - 追踪特定命令的文件访问

#### analyze-network
- **路径**: `.claude/skills/analyze-network/SKILL.md`
- **功能**: 分析网络流量，包括：
  - 查看活跃网络连接
  - 追踪进程网络行为
  - 识别异常出站连接
  - 网络指纹分析
- **使用场景**:
  - 按进程分析网络活动
  - 检测端口扫描行为
  - 分析失败的连接
  - 与安全策略集成实现自动化响应

#### monitor-process
- **路径**: `.claude/skills/monitor-process/SKILL.md`
- **功能**: 深度监控进程行为，包括：
  - 文件访问模式分析
  - 网络行为监控
  - 进程创建链追踪
  - 异常检测规则
- **使用场景**:
  - 调试 Agent 的文件访问问题
  - 审计进程安全行为
  - 检测未授权文件访问
  - 识别提权尝试和数据外泄

### 3. 更新项目文档

#### README.md
- 添加了 MCP 工具列表表格
- 说明了工具的参数和使用要求
- 介绍了三个 skills 及其用途

#### AGENTS.md
- 在 Auth model 部分补充了 `/mcp` 端点的认证说明
- 新增 "MCP 服务和 Skills" 章节
- 详细列出了所有 MCP 工具分类
- 提供了修改 MCP 工具的注意事项

## 技术细节

### MCP 工具实现特点

1. **类型安全**: 所有工具都定义了严格的输入/输出类型
2. **权限控制**: 安全策略工具会检查 `policyManagementEnabled` 标志
3. **错误处理**: 所有操作都有完善的错误返回
4. **数据一致性**: 直接调用底层函数，不重复实现逻辑

### Skills 设计原则

1. **场景驱动**: 每个 skill 都针对具体使用场景设计
2. **完整文档**: 包含使用场景、操作方法、示例和验证方法
3. **最佳实践**: 提供了异常检测规则和响应策略
4. **关联集成**: 说明了如何与其他功能（wrapper、TLS capture 等）集成

## 新增文件清单

```
.claude/skills/configure-security/SKILL.md
.claude/skills/analyze-network/SKILL.md
.claude/skills/monitor-process/SKILL.md
```

## 修改文件清单

```
backend/app/server__server_mcp.go  — 新增 8 个 MCP 工具
README.md                           — 新增 MCP 工具说明和 Skills 介绍
AGENTS.md                           — 补充 MCP 和 Skills 开发指南
```

## 使用示例

### 通过 MCP 工具监控进程

```javascript
// 1. 添加追踪命令
mcp.call('add_tracked_command', { command: 'python3', tag: 'ai-agent' })

// 2. 查询该命令的事件
const events = mcp.call('query_events', { 
  comm: 'python3', 
  eventType: 'openat',
  limit: 100 
})

// 3. 分析文件访问模式
events.forEach(e => {
  if (e.event.path.includes('.ssh')) {
    console.warn('敏感文件访问:', e)
  }
})
```

### 通过 MCP 工具阻止异常连接

```javascript
// 1. 获取网络流量
const flows = mcp.call('get_network_flows')

// 2. 识别可疑连接
const suspicious = flows.flows.filter(f => 
  f.state === 'ESTABLISHED' && 
  f.dstPort === 8443 &&
  !isWhitelisted(f.dstIP)
)

// 3. 阻止可疑 IP
suspicious.forEach(f => {
  mcp.call('block_network_destination', { ip: f.dstIP })
})
```

## 后续改进建议

1. **批量操作**: 添加批量添加/删除追踪命令和路径的工具
2. **规则导入导出**: 支持导入导出完整的安全策略配置
3. **告警集成**: 将异常检测结果通过 MCP 工具推送到外部告警系统
4. **统计分析**: 添加事件统计和趋势分析工具
5. **实时订阅**: 支持通过 MCP 订阅实时事件流（目前需要轮询）

## 验证记录

- MCP 工具代码已实现
- Skills 文件已创建
- 文档已更新
- ⚠️  编译检查发现其他文件的问题（`tls__capturestartuptls.go` 中 CodexSyscallTracker 未定义），但与本次 MCP 优化无关
- ⏳ 需要启动后端进行运行时测试

## 注意事项

1. **认证要求**: 所有 MCP 工具都需要有效的 access token
2. **权限检查**: `block_*` 工具需要 `policyManagementEnabled=true`
3. **精确匹配**: tracked_comms 是 16 字节精确匹配，tracked_paths 是 256 字节精确匹配
4. **持久化**: eBPF maps 会被 pin 到 `/sys/fs/bpf/agent-ebpf/`，重启后保留
5. **性能影响**: 大量安全规则可能影响 LSM hooks 的性能

## 能力概览

MCP 服务新增 8 个工具，覆盖配置管理、事件查询、监控和安全策略；配套 3 个 Claude Code skills，提供安全配置、网络分析和进程监控的操作入口。

---

## 相关导航

- [MCP、External API 与 OTLP](integrations/mcp-external-otlp.md)
- [MCP Streamable 迁移完成说明](mcp-migration-complete.md)
- [External API](external-api.md)
- [Runtime Gates 与 Auth](security/runtime-gates-auth.md)
- [Native Hooks](integrations/native-hooks.md)
