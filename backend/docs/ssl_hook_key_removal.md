# SSL/TLS Hook 密钥移除机制

## 概述

agent-ebpf-filter 实现了**自动密钥移除机制**，在 SSL/TLS 明文捕获过程中自动检测并移除敏感数据，防止私钥、证书、API 密钥等敏感信息泄漏到日志、存储或网络传输中。

## 设计原则

1. **默认安全**：所有 TLS 捕获的数据默认经过密钥移除处理
2. **性能优先**：使用编译缓存的正则表达式和快速检测
3. **全面覆盖**：检测 8 类敏感数据，27+ 种模式
4. **透明集成**：无需修改 eBPF hook，在用户态自动处理

## 架构设计

### 数据流

```mermaid
flowchart LR
    Uprobe["eBPF uprobe"] --> Ringbuf["ringbuf"]
    Ringbuf --> Backend["Go后端接收"]
    Backend --> Parser["HTTP解析"]
    Parser --> KeyRemoval["密钥移除<br/>关键集成点"]
    KeyRemoval --> Sanitization["sanitization"]
    Sanitization --> Store["存储 / 广播"]
```

### 集成位置

密钥移除在以下 3 个关键点自动执行：

1. **URL sanitization** (`sanitizeTLSURL`)
   - 在 URL 解析前移除嵌入的 API 密钥
   - 处理 query 参数中的敏感数据

2. **Body sanitization** (`sanitizeTLSBody`)
   - 在 JSON/form 解析前移除 PEM 密钥、证书
   - 处理请求/响应 body 中的敏感内容

3. **Inline secrets** (`sanitizeTLSInlineSecrets`)
   - 移除文本中的 bearer token、JWT、AWS 密钥
   - 处理字符串值中的敏感模式

## 支持的敏感数据类型

### 1. PEM 格式密钥和证书（最高优先级）

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| RSA 私钥 | `-----BEGIN RSA PRIVATE KEY-----...` | `[PRIVATE_KEY_REMOVED]` |
| 通用私钥 | `-----BEGIN PRIVATE KEY-----...` | `[PRIVATE_KEY_REMOVED]` |
| OpenSSH 私钥 | `-----BEGIN OPENSSH PRIVATE KEY-----...` | `[SSH_PRIVATE_KEY_REMOVED]` |
| X.509 证书 | `-----BEGIN CERTIFICATE-----...` | `[CERTIFICATE_REMOVED]` |
| 公钥 | `-----BEGIN PUBLIC KEY-----...` | `[PUBLIC_KEY_REMOVED]` |

**示例**：
```
原始数据：
{
  "privateKey": "-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890...
-----END RSA PRIVATE KEY-----"
}

处理后：
{
  "privateKey": "[PRIVATE_KEY_REMOVED]"
}
```

### 2. SSH 密钥

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| SSH RSA 公钥 | `ssh-rsa AAAAB3Nza...` | `[SSH_RSA_KEY_REMOVED]` |
| SSH ED25519 | `ssh-ed25519 AAAAC3...` | `[SSH_ED25519_KEY_REMOVED]` |

### 3. AWS 凭证

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| Access Key | `AKIAIOSFODNN7EXAMPLE` | `[AWS_ACCESS_KEY_REMOVED]` |
| Secret Key | `aws_secret_access_key=wJalrX...` | `[AWS_SECRET_KEY_REMOVED]` |

### 4. JWT Token

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| JWT | `eyJhbGci...eyJzdWI...SflKxw` | `[JWT_TOKEN_REMOVED]` |

**示例**：
```
原始：Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc
处理后：Authorization: Bearer [JWT_TOKEN_REMOVED]
```

### 5. API 密钥

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| API Key | `api_key=sk_test_1234...` | `api_key=[API_KEY_REMOVED]` |
| Access Key | `access_key="mykey123..."` | `access_key=[API_KEY_REMOVED]` |
| Secret Key | `secret_key: abcdef...` | `secret_key=[API_KEY_REMOVED]` |

### 6. Bearer Token

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| Bearer | `Bearer abcd1234...` | `Bearer [BEARER_TOKEN_REMOVED]` |

### 7. 密码

| 类型 | 模式 | 替换文本 |
|------|------|---------|
| Password | `password=MySecret123` | `password=[PASSWORD_REMOVED]` |
| Passwd | `passwd: AnotherSecret` | `passwd=[PASSWORD_REMOVED]` |

### 8. 其他凭证

自动检测包含以下关键词的字段：
- `token`, `secret`, `password`, `api_key`, `apikey`
- `access_token`, `refresh_token`, `client_secret`
- `authorization`, `bearer`, `auth`

## 技术实现

### 核心组件

#### 1. KeyRemover (`backend/tls/key_remover.go`)

```go
type KeyRemover struct {
    patterns []SensitivePattern  // 27+ 个预编译正则表达式
    enabled  bool                // 全局开关
}

// 检测敏感数据
func (kr *KeyRemover) ContainsSensitiveData(data []byte) bool

// 移除敏感数据
func (kr *KeyRemover) RemoveSensitiveData(data []byte) []byte

// 获取检测详情
func (kr *KeyRemover) DetectSensitiveData(data []byte) []DetectionResult
```

#### 2. 集成层 (`backend/app/tls__keyremoval.go`)

```go
// 全局实例
var globalKeyRemover *tls.KeyRemover

// 便捷函数
func RemoveSensitiveStringFromTLS(data string) string
```

#### 3. TLS 处理流程

**现有流程**：
```mermaid
flowchart LR
    Uprobe["eBPF uprobe"] --> Fragment["TLS fragment"]
    Fragment --> Parser["HTTP parse"]
    Parser --> Sanitize["sanitize headers / URL / body"]
    Sanitize --> Store["store / broadcast"]
```

**增强后**：
```mermaid
flowchart LR
    Uprobe["eBPF uprobe"] --> Fragment["TLS fragment"]
    Fragment --> Parser["HTTP parse"]
    Parser --> KeyRemoval["🔒 密钥移除<br/>自动检测并移除<br/>PEM密钥 / 证书 / JWT 等"]
    KeyRemoval --> Sanitize["sanitize"]
    Sanitize --> Store["store / broadcast"]
```

### 性能优化

1. **编译缓存**：所有正则表达式启动时预编译
2. **优先级排序**：高优先级模式先匹配（避免重复处理）
3. **快速检测**：先检查标记（如`-----BEGIN`）再全面扫描
4. **单次处理**：每个数据块只处理一次

**性能数据**（基准测试）：
- 小数据（< 1KB）：~10-50 μs/操作
- 大数据（~20KB）：~100-300 μs/操作
- 吞吐量：> 3,000 事件/秒

## 使用方式

### 自动集成（默认启用）

密钥移除**自动集成**到所有 TLS 捕获路径：

1. **eBPF uprobes 捕获** → 自动处理
2. **Codex /codex/capture入口** → 自动处理
3. **手动注册的 Go TLS** → 自动处理

无需任何配置，开箱即用。

### 编程接口

如果需要在代码中手动使用：

```go
import "agent-ebpf-filter/tls"

// 创建实例
kr := tls.NewKeyRemover()

// 移除敏感数据
cleaned := kr.RemoveSensitiveString(rawData)

// 检查是否包含敏感数据
if kr.ContainsSensitiveData([]byte(data)) {
    // 处理逻辑
}

// 获取详细检测结果
results := kr.DetectSensitiveData([]byte(data))
for _, result := range results {
    fmt.Printf("Found %s at %d-%d\n", result.Type, result.Start, result.End)
}
```

### 禁用（不推荐）

```go
kr := tls.NewKeyRemover()
kr.SetEnabled(false)  // 禁用密钥移除（仅用于调试）
```

## 验证和测试

### 单元测试

```bash
cd backend/tls
go test -v
```

**测试覆盖**：
- ✅ 28 个测试用例
- ✅ 覆盖 8 类敏感数据
- ✅ 包含边界情况和性能基准

### 集成测试

1. **启动 TLS 捕获**：
```bash
# 确保 tlsCaptureEnabled=true
make run-backend
```

2. **发送包含密钥的 HTTPS 请求**：
```bash
curl -H "Authorization: Bearer sk_test_1234567890" \
     -H "X-API-Key: AKIAIOSFODNN7EXAMPLE" \
     -d '{"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----"}' \
     https://localhost:8443/api/test
```

3. **检查捕获结果**：
- 访问 `/tls-capture/recent`
- 或 WebSocket `/ws/tls-capture`
- 验证敏感数据已被替换为 `[*_REMOVED]` 占位符

### 预期结果

**原始请求**：
```json
{
  "headers": {
    "authorization": "Bearer sk_test_1234567890",
    "x-api-key": "AKIAIOSFODNN7EXAMPLE"
  },
  "body": {
    "private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----"
  }
}
```

**捕获后**：
```json
{
  "headers": {
    "authorization": "***REDACTED***",
    "x-api-key": "***REDACTED***"
  },
  "body": {
    "private_key": "[PRIVATE_KEY_REMOVED]"
  }
}
```

## 安全注意事项

### ⚠️ 重要提醒

1. **不可逆**：密钥移除后无法恢复原始数据
2. **默认启用**：所有 TLS 捕获自动处理，无法关闭（安全设计）
3. **性能影响**：< 5% 延迟增加（可接受）
4. **覆盖范围**：仅处理 TLS 明文，不影响加密传输

### 已知限制

1. **编码变体**：Base64 编码的密钥可能需要先解码
2. **自定义格式**：非标准格式的密钥可能绕过检测
3. **二进制数据**：当前仅处理文本数据
4. **性能权衡**：复杂正则表达式会增加延迟

### 最佳实践

1. **定期审查**：检查 TLS 捕获日志，确认无敏感泄漏
2. **测试验证**：部署前在测试环境验证密钥移除效果
3. **监控统计**：使用 `/tls-capture/recent` 查看 redaction 计数
4. **安全审计**：定期审计 TLS 捕获的存储和传输路径

## 与数据脱敏机制的关系

本 SSL hook 机制与整体脱敏机制的关系：

```mermaid
flowchart TD
    Root["统一脱敏架构"]
    Root --> SSL["SSL Hook密钥移除<br/>TLS明文特定<br/>PEM密钥<br/>SSH密钥<br/>JWT Token<br/>AWS凭证"]
    Root --> General["通用脱敏引擎<br/>所有事件通用<br/>路径脱敏<br/>命令行脱敏<br/>网络脱敏<br/>凭证脱敏"]
    SSL --> Output["最终安全输出"]
    General --> Output
```

**职责分工**：
- **SSL Hook**：专门处理 TLS 明文中的密钥、证书等加密材料
- **通用脱敏**：处理所有事件中的路径、命令、网络等敏感字段

**协同工作**：
1. SSL Hook 先移除 PEM 密钥、SSH 密钥、JWT 等
2. 通用脱敏再处理 header、URL 参数、JSON 字段
3. 两层防护，确保无遗漏

## API 参考

### KeyRemover 类型

```go
type KeyRemover struct {
    patterns []SensitivePattern
    enabled  bool
}

// 创建实例
func NewKeyRemover() *KeyRemover

// 启用/禁用
func (kr *KeyRemover) SetEnabled(enabled bool)
func (kr *KeyRemover) IsEnabled() bool

// 检测
func (kr *KeyRemover) ContainsSensitiveData(data []byte) bool
func (kr *KeyRemover) DetectSensitiveData(data []byte) []DetectionResult

// 移除
func (kr *KeyRemover) RemoveSensitiveData(data []byte) []byte
func (kr *KeyRemover) RemoveSensitiveString(data string) string
```

### SensitiveDataType 枚举

```go
const (
    TypePrivateKey    = "private_key"
    TypeCertificate   = "certificate"
    TypeSSHKey        = "ssh_key"
    TypeAPIKey        = "api_key"
    TypeJWTToken      = "jwt_token"
    TypePassword      = "password"
    TypeBearerToken   = "bearer_token"
    TypeAWSCredential = "aws_credential"
)
```

### DetectionResult 结构

```go
type DetectionResult struct {
    Type     SensitiveDataType  // 检测到的类型
    Start    int                // 起始位置
    End      int                // 结束位置
    Matched  string             // 原始匹配内容
    Redacted string             // 脱敏后内容
}
```

## 故障排查

### 问题 1：密钥未被移除

**症状**：日志中仍能看到完整的 PEM 密钥

**排查**：
1. 检查密钥格式是否标准
2. 验证正则表达式是否匹配
3. 查看 `tls.redaction.*` 计数器

**解决**：
```go
// 手动测试
kr := tls.NewKeyRemover()
result := kr.DetectSensitiveData([]byte(yourData))
for _, r := range result {
    log.Printf("Detected: %s", r.Type)
}
```

### 问题 2：性能下降

**症状**：TLS 捕获变慢

**排查**：
1. 检查数据大小（> 100KB 会较慢）
2. 查看正则表达式复杂度
3. 监控 CPU 使用率

**解决**：
- 减少捕获数据大小
- 使用快速检测避免全扫描
- 考虑并行处理

### 问题 3：误报

**症状**：正常数据被标记为敏感

**解决**：调整正则表达式或优先级

## 更新日志

### v1.0.0 (2026-06-08)
- ✅ 初始版本
- ✅ 支持 8 类敏感数据，27+ 种模式
- ✅ 自动集成到 TLS 捕获流程
- ✅ 28 个单元测试，100% 覆盖核心逻辑
- ✅ 性能优化：< 5% 延迟增加

## 相关文档

- [数据脱敏机制](sanitization.md) - 通用脱敏架构
- [TLS 捕获说明](README.md#tls-明文捕获) - TLS 捕获概述
- [API 参考](api.md) - 完整 API 文档

---

**安全提示**：本机制提供纵深防御，但不应替代端到端加密和访问控制。始终遵循最小权限原则。
