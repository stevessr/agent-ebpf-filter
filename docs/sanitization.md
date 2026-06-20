# 数据脱敏机制

## 概述

agent-ebpf-filter 实现了完整的数据脱敏机制，保护系统采集和传输过程中的敏感信息。脱敏机制覆盖从 eBPF 内核采集到前端展示的整个数据流，确保用户隐私和安全。

## 架构设计

### 四层架构

```mermaid
flowchart LR
    Collect["采集层<br/>eBPF"] --> Process["处理层<br/>归一化"]
    Process --> Redact["脱敏层<br/>脱敏引擎"]
    Redact --> Distribute["分发层<br/>WS / JSONL / MCP / UI"]
```

#### 1. 采集层（SanitizationInputAdapters）
- 接收来自 eBPF、HTTP 捕获、TLS 解析、ML 数据集和开发预览的原始事件
- 转换为统一的中间事件格式
- 附加来源元数据和追踪上下文

#### 2. 处理层（EventProcessingPipeline）
- 标准化事件结构
- 识别敏感字段位置
- 补全上下文信息
- 解析脱敏级别和出口策略

#### 3. 脱敏层（UnifiedRedactionEngine）
- 根据配置执行脱敏规则
- 支持 4 个脱敏级别
- 实现字段类别处理器
- 保证幂等性和可缓存

#### 4. 分发层（SanitizedOutputDistributors）
- 将脱敏结果分发到各出口
- 支持 WebSocket、JSONL、MCP、OTLP、前端视图
- 禁止绕过脱敏层直接访问原始数据

## 脱敏级别

### None - 无脱敏
**适用场景**：完全信任的开发/调试环境

- 保留所有原始数据
- 不应用任何脱敏规则
- 仅用于本地开发和排查问题

### Basic - 基础脱敏
**适用场景**：内部测试环境

**脱敏内容**：
- 命令行参数中的明显密码/token：
  - `password`, `passwd`, `pwd`, `secret`, `token`
  - `api_key`, `apikey`, `access_token`, `refresh_token`
- 明显的凭证格式：
  - `key=value`, `token=value`, `password=value`
- 环境变量中的敏感值

**不脱敏**：路径、IP地址、域名

### Standard - 标准脱敏（默认）
**适用场景**：生产环境、日常使用

**脱敏内容**（包含 Basic 的所有规则）：
- **路径脱敏**：
  - 用户主目录 → `~`
  - `/home/user/.config` → `<CONFIG>`
  - `/home/user/.cache` → `<CACHE>`
  - `/home/user/.local/share` → `<DATA>`
- **命令参数脱敏**：
  - 所有凭证类参数和值
  - Authorization、Bearer 头
- **网络脱敏**：
  - 内网 IP（10.x.x.x, 192.168.x.x, 172.16-31.x.x）→ `<PRIVATE_IP>`
  - 内部域名（*.internal, *.corp, *.local）→ `<INTERNAL_DOMAIN>`
- **自定义规则**：用户配置的正则表达式生效

### Strict - 严格脱敏
**适用场景**：高安全要求、合规审计、公开展示

**脱敏内容**（包含 Standard 的所有规则）：
- **路径最大化隐藏**：
  - 绝对路径只保留顶层目录和文件名
  - 中间路径 → `<PATH>`
- **命令参数收敛**：
  - 除白名单外的长参数 → `<ARG>`
  - 参数值截断或完全替换
- **网络地址最大化隐藏**：
  - 所有 IP → `<IP>`
  - 所有域名 → `<DOMAIN>`
  - 端口 → `<PORT>`（可选）
- **最小化信息保留**：
  - 仅保留事件类型、时间、进程类别、状态码

## 敏感字段分类

### 1. 路径字段
- `path`, `cwd`, `extra_path`, `related_path`
- 文件系统路径可能暴露用户目录结构和配置

### 2. 命令字段
- `command_line`, `args`, `argv_digest`
- 命令行参数可能包含密码、token、API密钥

### 3. 网络字段
- `src_ip`, `dst_ip`, `host`, `url`, `dns_name`, `sni`, `http_host`
- 网络数据可能暴露内网拓扑和访问目标

### 4. 凭证字段
- `headers`, `body`, `query_params`
- HTTP 请求/响应中的认证信息

### 5. 标识符字段
- `access_token`, `agent_run_id`, `conversation_id`, `tool_call_id`
- 会话和追踪标识符可能用于关联用户行为

## 配置方式

### 1. 通过前端 UI 配置

访问 **Config → Redaction** 标签页：

1. **选择脱敏级别**：None / Basic / Standard / Strict
2. **配置出口策略**：
   - WebSocket 实时推送
   - JSONL 持久化
   - MCP 接口暴露
   - OTLP 导出
   - 前端 UI 展示
3. **自定义规则**：
   - 添加正则表达式规则
   - 设置规则优先级
   - 启用/禁用特定规则

### 2. 通过配置文件

编辑 `~/.config/agent-ebpf-filter/runtime.json`：

```json
{
  "redaction": {
    "level": "standard",
    "enabled": true,
    "output_visibility": {
      "ws_enabled": true,
      "jsonl_enabled": true,
      "mcp_enabled": true,
      "otlp_enabled": true,
      "ui_enabled": true
    },
    "custom_rules": [
      {
        "category": "custom_regex",
        "pattern": "API_KEY_[A-Za-z0-9]+",
        "action": "replace",
        "priority": 1,
        "enabled": true
      }
    ]
  }
}
```

### 3. 通过环境变量

```bash
export AGENT_REDACTION_LEVEL=strict
export AGENT_REDACTION_ENABLED=true
```

## 脱敏规则详解

### 路径脱敏规则

#### Standard 级别示例
```
原始：/home/steve/.ssh/id_rsa
脱敏：~/.ssh/id_rsa

原始：/home/steve/.config/agent-ebpf-filter/runtime.json
脱敏：<CONFIG>/agent-ebpf-filter/runtime.json

原始：/Users/steve/Documents/secret.txt
脱敏：~/Documents/secret.txt
```

#### Strict 级别示例
```
原始：/home/steve/projects/myapp/src/config.py
脱敏：<HOME>/<PATH>/config.py

原始：/etc/systemd/system/myservice.service
脱敏：/etc/<PATH>/myservice.service
```

### 命令行脱敏规则

#### Basic 级别示例
```
原始：curl -H "Authorization: Bearer sk-abc123"
脱敏：curl -H "Authorization: Bearer [REDACTED]"

原始：mysql -u root -pMyPassword123
脱敏：mysql -u root -p[REDACTED]

原始：export API_KEY=ghp_abcdef123456
脱敏：export API_KEY=[REDACTED]
```

#### Standard 级别示例
```
原始：git clone https://user:token@github.com/org/repo.git
脱敏：git clone https://[REDACTED]:[REDACTED]@github.com/org/repo.git

原始：docker run -e AWS_ACCESS_KEY_ID=AKIA...
脱敏：docker run -e AWS_ACCESS_KEY_ID=[REDACTED]
```

#### Strict 级别示例
```
原始：python train.py --data /path/to/data --epochs 100 --lr 0.001
脱敏：python train.py --data <ARG> --epochs <ARG> --lr <ARG>
```

### 网络脱敏规则

#### Standard 级别示例
| 原始值 | 脱敏后 |
| --- | --- |
| `192.168.1.100:8080` | `<PRIVATE_IP>:8080` |
| `myapp.internal.corp:443` | `<INTERNAL_DOMAIN>:443` |
| `10.0.5.23 → api.internal` | `<PRIVATE_IP> → <INTERNAL_DOMAIN>` |

#### Strict 级别示例
```
原始：8.8.8.8:53
脱敏：<IP>:<PORT>

原始：https://api.example.com/v1/users
脱敏：https://<DOMAIN>/v1/users
```

### 凭证脱敏规则

所有级别（Basic+）都会脱敏以下内容：

| 类别 | 原始值 | 脱敏后 |
| --- | --- | --- |
| HTTP Header | `Authorization: Bearer xxx` | `Authorization: [REDACTED]` |
| HTTP Header | `Cookie: session=xxx` | `Cookie: [REDACTED]` |
| HTTP Header | `X-API-Key: xxx` | `X-API-Key: [REDACTED]` |
| HTTP Header | `Set-Cookie: xxx` | `Set-Cookie: [REDACTED]` |
| URL Query Parameters | `?token=xxx&key=yyy` | `?token=[REDACTED]&key=[REDACTED]` |
| JSON Body | `{"password": "xxx"}` | `{"password": "[REDACTED]"}` |
| JSON Body | `{"api_key": "xxx"}` | `{"api_key": "[REDACTED]"}` |
| Form Data | `password=xxx&token=yyy` | `password=[REDACTED]&token=[REDACTED]` |

## 自定义规则

### 规则结构

```json
{
  "category": "custom_regex",
  "pattern": "<正则表达式>",
  "action": "replace|truncate|hash",
  "priority": 1,
  "enabled": true,
  "replacement": "<CUSTOM>"
}
```

### 规则示例

#### 匹配自定义 API 密钥格式
```json
{
  "category": "custom_regex",
  "pattern": "myapp_[a-f0-9]{32}",
  "action": "replace",
  "priority": 1,
  "replacement": "<MYAPP_KEY>"
}
```

#### 匹配企业域名
```json
{
  "category": "network",
  "pattern": ".*\\.mycompany\\.(com|net|internal)",
  "action": "replace",
  "priority": 2,
  "replacement": "<CORP_DOMAIN>"
}
```

#### 匹配特定路径前缀
```json
{
  "category": "paths",
  "pattern": "/opt/myapp/secrets/.*",
  "action": "replace",
  "priority": 1,
  "replacement": "<SECRETS_DIR>"
}
```

### 规则优先级

规则按以下顺序应用：
1. **自定义规则**（按 priority 从高到低）
2. **凭证规则**（最高优先级的内置规则）
3. **路径规则**
4. **命令行规则**
5. **网络规则**

同一分类内，高 priority 值的规则先执行。

## 前端展示

### 脱敏状态徽章

在 Dashboard、Network、TLSCapture 等页面顶部显示当前脱敏级别：

- **None**: 灰色徽章 🔓
- **Basic**: 蓝色徽章 🔵
- **Standard**: 绿色徽章 ✅（默认）
- **Strict**: 红色徽章 🔒

### 脱敏字段显示

使用 `<SanitizedFieldViewer>` 组件显示敏感字段：

```vue
<SanitizedFieldViewer
  :value="event.path"
  :is-sanitized="event.redaction_level !== 'none'"
  field-name="path"
/>
```

**显示效果**：
- 脱敏字段显示替换后的值
- 鼠标悬停显示"已脱敏"提示
- 提供复制脱敏后值的按钮
- 不显示原始值（安全优先）

## 技术实现

### 后端实现

#### 目录结构
```mermaid
flowchart TD
    Root["backend/redaction/"]
    Root --> Types["types.go<br/>类型定义"]
    Root --> Engine["engine.go<br/>脱敏引擎核心"]
    Root --> Cache["cache.go<br/>缓存层"]
    Root --> Normalizer["normalizer.go<br/>事件归一化"]
    Root --> Processing["processing.go<br/>字段处理"]
    Root --> Distributor["distributor.go<br/>分发器"]
    Root --> Rules["rules/"]
    Rules --> Registry["registry.go<br/>规则注册表"]
    Rules --> PathRule["path.go<br/>路径脱敏"]
    Rules --> CommandRule["command.go<br/>命令行脱敏"]
    Rules --> NetworkRule["network.go<br/>网络脱敏"]
    Rules --> CredentialRule["credential.go<br/>凭证脱敏"]
    Rules --> Pseudonymizer["pseudonymizer.go<br/>标识符假名化"]
    Rules --> CustomRule["custom.go<br/>自定义规则"]
```

#### 核心函数

```go
// 创建脱敏引擎
engine := redaction.NewRedactionEngine(policy)

// 脱敏单个事件
sanitized, err := engine.RedactEvent(ctx, event)

// 批量脱敏
results := engine.RedactBatch(ctx, events)

// 应用规则到字符串
masked := engine.ApplyRules(value, redaction.FieldCategoryPath)
```

#### 集成点

1. **事件封装**（`backend/app/runtime__envelope_event.go`）
   ```go
   envelope := buildEventEnvelope(record)
   envelope = applyRedaction(envelope)
   return envelope
   ```

2. **持久化**（`backend/app/runtime__statepersistenceruntime.go`）
   ```go
   record = applyRedactionToRecord(record)
   runtimeState.AppendEvent(record)
   ```

3. **WebSocket 广播**（`backend/app/server__ws_api.go`）
   ```go
   sanitizedRecord := applyRedaction(record)
   broadcast(sanitizedRecord)
   ```

### 前端实现

#### 目录结构
```mermaid
flowchart TD
    Root["frontend/src/"]
    Root --> Composables["composables/config/"]
    Composables --> Policy["useRedactionPolicy.ts<br/>脱敏策略管理"]
    Root --> ConfigComponents["components/config/"]
    ConfigComponents --> ConfigTab["ConfigRedactionTab.vue<br/>配置页面"]
    Root --> CommonComponents["components/common/"]
    CommonComponents --> Badge["RedactionBadge.vue<br/>状态徽章"]
    CommonComponents --> Viewer["SanitizedFieldViewer.vue<br/>字段查看器"]
```

#### 组合式 API

```typescript
import { useRedactionPolicy } from '@/composables/config/useRedactionPolicy'

const { level, setLevel, rules, updateRules } = useRedactionPolicy()

// 切换级别
await setLevel('strict')

// 添加自定义规则
await updateRules([...rules.value, newRule])
```

## 性能考虑

### 缓存策略

1. **规则缓存**：编译后的正则表达式缓存，避免重复编译
2. **结果缓存**：相同输入的脱敏结果缓存（LRU）
3. **批量处理**：支持批量脱敏，减少函数调用开销

### 性能数据

在标准硬件上的性能指标：

- **单事件脱敏延迟**：< 1ms（缓存命中）
- **批量脱敏吞吐**：> 10,000 事件/秒
- **内存占用**：~10MB（引擎 + 缓存）
- **CPU 开销**：< 5%（正常负载下）

### 优化建议

1. **选择合适的级别**：Standard 是性能和安全的平衡点
2. **限制自定义规则**：过多复杂正则会影响性能
3. **使用缓存**：相似事件会命中缓存，大幅提升性能
4. **按需脱敏**：仅对必要的出口启用脱敏

## 安全注意事项

### ⚠️ 重要提醒

1. **脱敏不可逆**：脱敏后的数据无法还原，确保不影响业务需求
2. **默认安全**：系统默认 Standard 级别，除非明确需要否则不要降级
3. **日志审计**：脱敏配置变更会记录到审计日志
4. **Out-of-band 数据**：脱敏仅处理系统采集的数据，不影响原始系统行为

### 已知限制

1. **二进制数据**：当前不处理二进制协议（除 TLS/HTTP）
2. **编码变体**：Base64/URL编码的敏感数据可能绕过检测
3. **语义理解**：无法理解业务语义（如订单号、用户ID）
4. **历史数据**：仅影响新采集数据，历史 JSONL 不会追溯脱敏

## 故障排查

### 问题：脱敏级别切换不生效

**症状**：修改配置后，前端仍显示旧的脱敏级别

**解决**：
1. 检查后端日志确认配置已加载
2. 刷新 WebSocket 连接：断开并重新连接
3. 清除浏览器缓存
4. 重启后端服务

### 问题：自定义规则不匹配

**症状**：添加的正则表达式规则没有生效

**解决**：
1. 验证正则表达式语法（使用 [regex101.com](https://regex101.com)）
2. 检查规则优先级，确保高于默认规则
3. 确认规则 `enabled: true`
4. 查看后端日志中的规则应用统计

### 问题：性能下降

**症状**：启用脱敏后事件处理变慢

**解决**：
1. 降低脱敏级别（Strict → Standard → Basic）
2. 减少自定义规则数量
3. 简化复杂的正则表达式
4. 检查缓存命中率（后端指标）
5. 考虑只对必要的出口启用脱敏

## 最佳实践

### 1. 选择合适的级别

- **开发环境**：None（便于调试）
- **测试环境**：Basic（保护明显敏感信息）
- **预发布环境**：Standard（默认推荐）
- **生产环境**：Standard 或 Strict（根据合规要求）
- **公开演示**：Strict（最大化隐藏）

### 2. 渐进式启用

1. 从 None 开始，确认系统正常工作
2. 升级到 Basic，验证基础脱敏
3. 升级到 Standard，观察影响
4. 按需升级到 Strict

### 3. 定期审查

- 每月检查脱敏统计指标
- 审查自定义规则的有效性
- 更新规则匹配新的敏感模式
- 根据安全审计调整配置

### 4. 测试验证

在生产部署前：
1. 使用测试事件验证脱敏效果
2. 检查 JSONL 日志文件内容
3. 抓取 WebSocket 流量验证
4. 导出配置并在测试环境重放

## API 参考

### 后端 API

#### GET /config/redaction-policy
获取当前脱敏策略

**响应**：
```json
{
  "level": "standard",
  "enabled": true,
  "custom_rules": [...],
  "output_visibility": {...}
}
```

#### PUT /config/redaction-policy
更新脱敏策略

**请求**：
```json
{
  "level": "strict",
  "enabled": true,
  "custom_rules": [...]
}
```

#### GET /config/redaction-stats
获取脱敏统计信息

**响应**：
```json
{
  "events_processed": 12345,
  "values_redacted": 678,
  "rules_applied": 234,
  "cache_hit_rate": 0.85
}
```

### Proto 定义

```protobuf
enum RedactionLevel {
  REDACTION_LEVEL_NONE = 0;
  REDACTION_LEVEL_BASIC = 1;
  REDACTION_LEVEL_STANDARD = 2;
  REDACTION_LEVEL_STRICT = 3;
}

message RedactionPolicy {
  RedactionLevel level = 1;
  bool enabled = 2;
  repeated RedactionRule custom_rules = 3;
  OutputVisibilityConfig output_visibility = 4;
}

message Event {
  // ... 现有字段 ...
  RedactionLevel redaction_level = 100;
  repeated string sanitized_fields = 101;
}
```

## 相关文档

- [架构文档](architecture/overview.md) - 系统整体架构
- [Runtime Gates 与 Auth](security/runtime-gates-auth.md) - 配置、环境变量与认证边界
- [构建与运行](operations/build-and-run.md) - 开发者运行入口
- [安全模型](security/model.md) - 安全最佳实践与边界

## 更新日志

### v1.0.0 (2026-06-08)
- 初始版本
- 实现四层脱敏架构
- 支持 4 个脱敏级别
- 提供前端配置界面
- 支持自定义规则

---

如有问题或建议，请提交 Issue 或 PR。
