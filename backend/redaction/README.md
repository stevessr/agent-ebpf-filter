# Redaction Module

数据脱敏与路径映射引擎，用于在事件流、日志和 API 响应中自动检测和清理敏感信息。

## 概述

Redaction 模块提供多层数据保护机制：
- **敏感信息检测**：自动识别凭据、密钥、token、PII
- **路径映射**：将真实文件路径映射为安全的虚拟路径
- **网络地址脱敏**：清理敏感 IP/域名/端口
- **统计跟踪**：记录脱敏操作的命中率和类别分布

## 核心类型

### `Engine`
主脱敏引擎，协调所有规则的应用。

```go
engine := redaction.NewEngine(config)
result := engine.Redact(event)
stats := engine.GetStatistics()
```

### `Rule` 接口
所有脱敏规则实现此接口：
```go
type Rule interface {
    Apply(event *pb.Event) bool
    Category() string
}
```

### 规则分类
- **CredentialRule**: 检测 API keys, tokens, passwords
- **PathMappingRule**: 映射文件路径到虚拟路径
- **NetworkRule**: 脱敏 IP 地址和端口
- **PIIRule**: 清理个人身份信息（邮箱、电话）

## 配置

### 默认规则集
```go
config := &redaction.Config{
    EnableCredentialDetection: true,
    EnablePathMapping:         true,
    EnableNetworkRedaction:    true,
    PathMappingRules: []PathMappingRule{
        {Pattern: "/home/*/.*", Replacement: "/home/<USER>/$1"},
        {Pattern: "/tmp/.*", Replacement: "/tmp/<TEMP>"},
    },
}
```

### 运行时配置
通过 `/config/redaction` API 动态更新规则：
```bash
curl -X POST http://localhost:8080/config/redaction \
  -H "X-API-KEY: $TOKEN" \
  -d '{
    "enableCredentialDetection": true,
    "pathMappingRules": [...]
  }'
```

## 路径映射详解

### 映射规则语法
```go
type PathMappingRule struct {
    Pattern     string  // 正则表达式匹配真实路径
    Replacement string  // 替换模板，支持捕获组
    Priority    int     // 规则优先级（越高越先匹配）
}
```

### 示例
```go
// 映射用户主目录
{Pattern: `/home/([^/]+)/(.*)`, Replacement: `/home/<USER>/$2`}

// 隐藏临时文件路径
{Pattern: `/tmp/agent-.*`, Replacement: `/tmp/<AGENT_TEMP>`}

// 保护配置文件
{Pattern: `.*/\.config/.*`, Replacement: `<CONFIG_DIR>`}
```

### 反向映射
对于需要真实路径的操作（如文件读取），使用反向映射：
```go
realPath := engine.ReverseMapPath(virtualPath)
```

## 数据流集成

### 事件流脱敏
```go
// backend/app/events__events.go
func processEvent(event *pb.Event) {
    if redactionEngine.Redact(event) {
        event.Redacted = true
        event.RedactionState = "applied"
    }
}
```

### WebSocket 输出
所有通过 `/ws` 发送的事件自动经过脱敏管道。

### API 响应
`/events/recent` 和 `/events/archive` 返回的事件已脱敏。

### JSONL 日志
持久化到 `~/.config/agent-ebpf-filter/events.jsonl` 的事件记录已脱敏。

## 统计与监控

### 获取统计信息
```bash
curl http://localhost:8080/config/redaction/stats \
  -H "X-API-KEY: $TOKEN"
```

返回：
```json
{
  "totalEvents": 10000,
  "redactedEvents": 342,
  "redactionRate": 0.0342,
  "categoryHits": {
    "credential": 120,
    "path": 180,
    "network": 42
  }
}
```

## 性能考量

- **缓存**: 路径映射结果自动缓存，避免重复正则匹配
- **优先级**: 高优先级规则先匹配，命中后跳过低优先级规则
- **批处理**: 事件批量脱敏时共享编译后的正则表达式

## 测试

```bash
# 单元测试
cd backend/redaction
go test ./...

# 集成测试
cd backend/backend/redaction
go test -v

# 性能基准
go test -bench=. -benchmem
```

## 扩展规则

### 自定义规则示例
```go
type CustomRule struct {
    pattern *regexp.Regexp
}

func (r *CustomRule) Apply(event *pb.Event) bool {
    if r.pattern.MatchString(event.Path) {
        event.Path = "<CUSTOM_REDACTED>"
        return true
    }
    return false
}

func (r *CustomRule) Category() string {
    return "custom"
}

// 注册到引擎
engine.AddRule(&CustomRule{pattern: regexp.MustCompile(`sensitive-.*`)})
```

## 安全注意事项

1. **规则顺序**: 确保最敏感的规则优先级最高
2. **泄漏风险**: 反向映射表存储在内存中，重启后丢失
3. **性能影响**: 每个事件都会应用所有启用的规则，复杂规则影响吞吐量
4. **日志安全**: 统计日志本身不应包含脱敏前的原始数据

## 相关文档

- [脱敏增强 v2 实现报告](./docs/SANITIZATION_ENHANCEMENTS_v2.md)
- [路径映射指南](./docs/path_mapping_guide.md)
- [路径映射实现细节](./docs/PATH_MAPPING_IMPLEMENTATION.md)
- [后端架构](../README.md)
- [配置 API](../../README.md#api-endpoints)
