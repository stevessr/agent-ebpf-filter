# 数据脱敏机制使用指南

## ### 1. 选择脱敏级别

访问前端 **Config → Redaction** 页面，选择：

- **None**：无脱敏（仅开发环境）
- **Basic**：脱敏明显的密码/token
- **Standard**：脱敏常见敏感信息（推荐）✅
- **Strict**：最大化脱敏

### 2. 配置生效

配置保存后自动应用到：
- ✅ WebSocket 实时事件流
- ✅ JSONL 持久化日志
- ✅ MCP 接口响应
- ✅ OTLP 遥测导出
- ✅ 前端 UI 展示

### 3. 查看效果

在 Dashboard、Network、TLSCapture 页面：
- 顶部显示当前脱敏级别徽章
- 敏感字段自动显示脱敏后的值
- 鼠标悬停查看脱敏提示

## ### Standard 级别（默认）

| 类别 | 原始数据 | 脱敏后 |
|------|---------|--------|
| **用户目录** | `/home/steve/.ssh/id_rsa` | `~/.ssh/id_rsa` |
| **配置目录** | `/home/steve/.config/app/key` | `<CONFIG>/app/key` |
| **命令参数** | `mysql -p MyPass123` | `mysql -p [REDACTED]` |
| **API 密钥** | `Authorization: Bearer sk-xxx` | `Authorization: [REDACTED]` |
| **内网 IP** | `192.168.1.100` | `<PRIVATE_IP>` |
| **内部域名** | `app.internal.corp` | `<INTERNAL_DOMAIN>` |

### Strict 级别

| 类别 | 原始数据 | 脱敏后 |
|------|---------|--------|
| **完整路径** | `/home/steve/proj/src/main.go` | `<HOME>/<PATH>/main.go` |
| **长参数** | `--data /long/path/to/file` | `--data <ARG>` |
| **所有 IP** | `8.8.8.8` | `<IP>` |
| **所有域名** | `api.example.com` | `<DOMAIN>` |

## ### 1：保护凭证信息

**需求**：防止密码、token 泄漏到日志

**配置**：
- 脱敏级别：Basic 或更高
- 自动脱敏：password、token、api_key、bearer、authorization

**验证**：
```bash
# 执行包含密码的命令
mysql -u root -p MyPassword123

# 查看事件日志
tail ~/.config/agent-ebpf-filter/events.jsonl
# 应该看到 -p [REDACTED]
```

### 2：保护内网拓扑

**需求**：日志导出时不暴露内网 IP 和域名

**配置**：
- 脱敏级别：Standard 或 Strict
- 自动脱敏内网 IP 和内部域名

**验证**：
在 Network 页面查看连接，内网地址应显示为 `<PRIVATE_IP>` 或 `<INTERNAL_DOMAIN>`

### 3：合规审计

**需求**：最大化保护用户隐私，符合数据保护法规

**配置**：
- 脱敏级别：Strict
- 启用所有出口的脱敏
- 添加自定义规则匹配企业特定模式

**验证**：
导出配置和日志，人工审查确认无敏感信息泄漏

## ### API 密钥格式

```json
{
  "category": "custom_regex",
  "pattern": "myapp_[a-f0-9]{32}",
  "action": "replace",
  "priority": 1,
  "replacement": "<MYAPP_KEY>"
}
```

### ```json
{
  "category": "custom_regex",
  "pattern": "[a-zA-Z0-9._%+-]+@mycompany\\.com",
  "action": "replace",
  "priority": 2,
  "replacement": "<CORP_EMAIL>"
}
```

### ```json
{
  "category": "paths",
  "pattern": "/opt/company/secrets/.*",
  "action": "replace",
  "priority": 1,
  "replacement": "<SECRETS>"
}
```

## ### runtime.json 配置

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
      "otlp_enabled": false,
      "ui_enabled": true
    },
    "custom_rules": []
  }
}
```

### ```bash
# 设置脱敏级别
export AGENT_REDACTION_LEVEL=strict

# 启用/禁用脱敏
export AGENT_REDACTION_ENABLED=true
```

## ### 1：修改配置不生效

**解决**：
1. 刷新前端页面
2. 断开并重新连接 WebSocket
3. 重启后端服务：`systemctl restart agent-ebpf-filter`

### 2：自定义规则不匹配

**解决**：
1. 在 [regex101.com](https://regex101.com) 测试正则表达式
2. 检查规则优先级（高优先级先执行）
3. 确认规则已启用（`"enabled": true`）
4. 查看后端日志：`journalctl -u agent-ebpf-filter -f`

### 3：过度脱敏影响调试

**解决**：
1. 临时切换到 Basic 级别
2. 或针对特定出口禁用脱敏（如仅禁用 UI 显示）
3. 添加白名单规则排除特定字段

## | 级别 | CPU 开销 | 延迟增加 | 吞吐影响 |
|------|---------|---------|---------|
| None | 0% | 0ms | 无 |
| Basic | < 2% | < 0.5ms | < 5% |
| Standard | < 5% | < 1ms | < 10% |
| Strict | < 10% | < 2ms | < 15% |

**优化建议**：
- Standard 是性能和安全的最佳平衡
- 避免过多复杂的自定义正则规则
- 缓存会显著提升重复事件的处理速度

## ### 推荐做法

1. **默认启用**：生产环境使用 Standard 或 Strict
2. **定期审查**：每月检查脱敏规则和统计
3. **测试验证**：部署前在测试环境验证脱敏效果
4. **限制访问**：限制对 None 级别的访问权限
5. **审计日志**：启用脱敏配置变更的审计日志

### 避免做法

1. **生产用 None**：除非完全信任环境，否则不要禁用脱敏
2. **降级为 Basic**：Standard 是推荐的最低安全级别
3. **绕过脱敏**：不要直接访问原始数据源绕过脱敏层
4. **过度自定义**：过多规则会影响性能和可维护性
5. **忽略更新**：随着威胁变化，定期更新脱敏规则

## - 完整文档：[数据脱敏机制](sanitization.md)
- 架构设计：[总体架构](../architecture/overview.md)
- API 参考：[路由与 API](../backend/routes-api.md)
- 开发维护指南：[维护检查清单](../reference/maintenance-checklists.md)

---

**注意**：脱敏是不可逆的，请根据业务需求选择合适的级别。
