# 路径映射功能使用指南

## 概述

**路径映射（Path Mapping）** 提供双向路径转换能力：
- **发送时（Outgoing）**：将真实路径映射为脱敏路径
- **接收时（Incoming）**：将脱敏路径逆映射回真实路径

这允许用户自定义路径脱敏规则，同时保持数据的可追溯性。

---

## 核心功能

### 1. 双向映射

```go
pm := NewPathMapper(PathMapperConfig{Enabled: true})

// 添加规则
pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",      // 真实路径
    Replacement: "/home/user/",       // 脱敏路径
    Priority:    100,
    Type:        PathRulePrefix,
})

// 发送时：真实 → 脱敏
sanitized := pm.MapOutgoing("/home/alice/secret.txt")
// → "/home/user/secret.txt"

// 接收时：脱敏 → 真实
real := pm.MapIncoming("/home/user/secret.txt")
// → "/home/alice/secret.txt"
```

### 2. 支持的规则类型

| 类型 | 说明 | 示例 |
|------|------|------|
| **Exact** | 精确匹配 | `/home/alice/secret.txt` |
| **Prefix** | 前缀匹配 | `/home/alice/` |
| **Suffix** | 后缀匹配 | `.secret` |
| **Wildcard** | 通配符（*, **） | `/home/*/documents/*.pdf` |
| **Regex** | 正则表达式 | `^/home/[a-z]+/.*\.txt$` |

---

## 使用示例

### 示例 1：前缀替换

**场景**：隐藏用户主目录

```go
pm := NewPathMapper(PathMapperConfig{Enabled: true})

pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",
    Replacement: "/home/user/",
    Priority:    100,
    Type:        PathRulePrefix,
})

// 发送
pm.MapOutgoing("/home/alice/documents/report.pdf")
// → "/home/user/documents/report.pdf"

pm.MapOutgoing("/home/alice/projects/app/main.go")
// → "/home/user/projects/app/main.go"

// 接收
pm.MapIncoming("/home/user/documents/report.pdf")
// → "/home/alice/documents/report.pdf"
```

### 示例 2：精确匹配

**场景**：隐藏特定敏感文件

```go
pm.AddRule(PathMappingRule{
    Pattern:     "/etc/secrets/api_key.txt",
    Replacement: "/etc/config/settings.txt",
    Priority:    100,
    Type:        PathRuleExact,
})

// 发送
pm.MapOutgoing("/etc/secrets/api_key.txt")
// → "/etc/config/settings.txt"

// 接收
pm.MapIncoming("/etc/config/settings.txt")
// → "/etc/secrets/api_key.txt"
```

### 示例 3：通配符

**场景**：隐藏所有用户的文档目录

```go
pm.AddRule(PathMappingRule{
    Pattern:     "/home/*/documents/*.pdf",
    Replacement: "/shared/docs/file.pdf",
    Priority:    100,
    Type:        PathRuleWildcard,
})

// 匹配
pm.MapOutgoing("/home/alice/documents/report.pdf")
// → "/shared/docs/file.pdf"

pm.MapOutgoing("/home/bob/documents/invoice.pdf")
// → "/shared/docs/file.pdf"

// 不匹配（* 不跨越 /）
pm.MapOutgoing("/home/alice/work/documents/report.pdf")
// → "/home/alice/work/documents/report.pdf" (保持原样)
```

**通配符规则**：
- `*` - 匹配任意字符，**不包括 /** 
- `**` - 匹配任意字符，**包括 /**

```go
// * vs **
Pattern: "/home/*/file.txt"
    匹配: /home/alice/file.txt
    不匹配: /home/alice/dir/file.txt

Pattern: "/home/**/file.txt"
    匹配: /home/alice/file.txt
    匹配: /home/alice/dir/file.txt
    匹配: /home/alice/dir/subdir/file.txt
```

### 示例 4：后缀替换

**场景**：隐藏文件扩展名

```go
pm.AddRule(PathMappingRule{
    Pattern:     ".secret",
    Replacement: ".txt",
    Priority:    100,
    Type:        PathRuleSuffix,
})

// 发送
pm.MapOutgoing("/home/user/passwords.secret")
// → "/home/user/passwords.txt"

// 接收
pm.MapIncoming("/home/user/passwords.txt")
// → "/home/user/passwords.secret"
```

### 示例 5：正则表达式

**场景**：复杂模式匹配

```go
pm.AddRule(PathMappingRule{
    Pattern:     `^/home/([a-z]+)/private/(.*)$`,
    Replacement: `/home/user/public/$2`,
    Priority:    100,
    Type:        PathRuleRegex,
})

// 发送
pm.MapOutgoing("/home/alice/private/data/file.txt")
// → "/home/user/public/data/file.txt"
```

---

## 优先级

多个规则可能匹配同一路径时，**优先级高的规则先匹配**。

```go
pm := NewPathMapper(PathMapperConfig{Enabled: true})

// 低优先级：通用规则
pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",
    Replacement: "/home/user/",
    Priority:    50,
    Type:        PathRulePrefix,
})

// 高优先级：特定规则
pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/secret/",
    Replacement: "/home/hidden/",
    Priority:    100,
    Type:        PathRulePrefix,
})

// 高优先级规则优先匹配
pm.MapOutgoing("/home/alice/secret/key.txt")
// → "/home/hidden/key.txt"

// 低优先级规则匹配其他路径
pm.MapOutgoing("/home/alice/public/file.txt")
// → "/home/user/public/file.txt"
```

**建议优先级分配**：
- **100+**：特定敏感路径（精确匹配）
- **80-99**：用户主目录、项目目录
- **50-79**：通用规则
- **< 50**：兜底规则

---

## 批量处理

```go
pm := NewPathMapper(PathMapperConfig{Enabled: true})

pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",
    Replacement: "/home/user/",
    Priority:    100,
    Type:        PathRulePrefix,
})

// 批量发送
paths := []string{
    "/home/alice/file1.txt",
    "/home/alice/file2.txt",
    "/home/bob/file3.txt",
}

sanitized := pm.MapOutgoingBatch(paths)
// → ["/home/user/file1.txt", "/home/user/file2.txt", "/home/bob/file3.txt"]

// 批量接收
real := pm.MapIncomingBatch(sanitized)
// → ["/home/alice/file1.txt", "/home/alice/file2.txt", "/home/bob/file3.txt"]
```

---

## 大小写敏感

```go
// 大小写敏感（默认）
pm := NewPathMapper(PathMapperConfig{
    Enabled:       true,
    CaseSensitive: true,
})

pm.AddRule(PathMappingRule{
    Pattern:     "/Home/Alice/",
    Replacement: "/home/user/",
    Priority:    100,
    Type:        PathRulePrefix,
})

pm.MapOutgoing("/Home/Alice/file.txt")  // ✅ 匹配
pm.MapOutgoing("/home/alice/file.txt")  // ❌ 不匹配

// 大小写不敏感
pm := NewPathMapper(PathMapperConfig{
    Enabled:       true,
    CaseSensitive: false,
})

pm.MapOutgoing("/Home/Alice/file.txt")  // ✅ 匹配
pm.MapOutgoing("/home/alice/file.txt")  // ✅ 匹配
pm.MapOutgoing("/HOME/ALICE/file.txt")  // ✅ 匹配
```

---

## 规则管理

### 添加规则

```go
err := pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",
    Replacement: "/home/user/",
    Priority:    100,
    Type:        PathRulePrefix,
})
if err != nil {
    log.Fatal("Failed to add rule:", err)
}
```

### 移除规则

```go
removed := pm.RemoveRule("/home/alice/")
if removed {
    log.Info("Rule removed")
}
```

### 获取所有规则

```go
rules := pm.GetRules()
for _, rule := range rules {
    fmt.Printf("Pattern: %s → %s (Priority: %d)\n",
        rule.Pattern, rule.Replacement, rule.Priority)
}
```

### 清空所有规则

```go
pm.ClearRules()
```

### 启用/禁用

```go
pm.SetEnabled(false)  // 禁用映射
pm.SetEnabled(true)   // 启用映射

if pm.IsEnabled() {
    log.Info("Path mapping is enabled")
}
```

---

## 持久化

### 导出规则

```go
// 导出到文件
rules := pm.ExportRules()

data, _ := json.MarshalIndent(rules, "", "  ")
os.WriteFile("path_mapping_rules.json", data, 0644)
```

### 导入规则

```go
// 从文件导入
data, _ := os.ReadFile("path_mapping_rules.json")

var rules []PathMappingRule
json.Unmarshal(data, &rules)

pm.ImportRules(rules)
```

---

## 统计信息

```go
tracker := NewPathMappingStatsTracker()

// 记录操作
tracker.RecordOutgoing(true)   // 映射成功
tracker.RecordOutgoing(false)  // 未映射
tracker.RecordIncoming(true)   // 逆映射成功

// 获取统计
stats := tracker.GetStats()

fmt.Printf("Total Outgoing: %d (Mapped: %d, Unmapped: %d)\n",
    stats.TotalOutgoing, stats.OutgoingMapped, stats.OutgoingUnmapped)

fmt.Printf("Total Incoming: %d (Mapped: %d, Unmapped: %d)\n",
    stats.TotalIncoming, stats.IncomingMapped, stats.IncomingUnmapped)

// 重置统计
tracker.Reset()
```

---

## 默认规则

```go
// 使用默认规则
pm := NewPathMapper(PathMapperConfig{
    Enabled:      true,
    DefaultRules: DefaultPathMappingRules(),
})

// 默认规则包括：
// 1. /home/*/ → /home/user/
// 2. /Users/*/ → /Users/user/ (macOS)
// 3. $HOME → ~/
```

---

## 性能优化

### 规则顺序

- 高优先级规则先匹配
- 精确匹配最快
- 正则表达式最慢

**建议**：
1. 常用规则设置高优先级
2. 精确匹配优先于通配符
3. 避免过多正则表达式规则

### 批量处理

```go
// ❌ 慢：逐个处理
for _, path := range paths {
    sanitized := pm.MapOutgoing(path)
}

// ✅ 快：批量处理
sanitized := pm.MapOutgoingBatch(paths)
```

### 缓存

精确匹配规则使用内部缓存（`reverseMap`），逆映射性能最优。

---

## 注意事项

### 1. 映射冲突

避免多个规则映射到相同的替换路径：

```go
// ❌ 冲突：两个不同路径映射到同一个
pm.AddRule(PathMappingRule{
    Pattern:     "/home/alice/secret.txt",
    Replacement: "/home/user/file.txt",
    Type:        PathRuleExact,
})
pm.AddRule(PathMappingRule{
    Pattern:     "/home/bob/data.txt",
    Replacement: "/home/user/file.txt",  // 冲突！
    Type:        PathRuleExact,
})

// 逆映射会混淆
pm.MapIncoming("/home/user/file.txt")
// → 只能返回第一个匹配
```

### 2. 通配符限制

`*` 不跨越 `/`，使用 `**` 跨越目录：

```go
Pattern: "/home/*/file.txt"
    ✅ /home/alice/file.txt
    ❌ /home/alice/dir/file.txt

Pattern: "/home/**/file.txt"
    ✅ /home/alice/file.txt
    ✅ /home/alice/dir/file.txt
```

### 3. 正则表达式

复杂正则可能影响性能，谨慎使用。

### 4. 逆映射准确性

- **Exact、Prefix、Suffix**：逆映射 100% 准确
- **Wildcard、Regex**：逆映射是"最佳努力"，可能不完全准确

---

## 集成示例

### 与脱敏引擎集成

```go
// 创建路径映射器
pathMapper := NewPathMapper(PathMapperConfig{
    Enabled:      true,
    DefaultRules: DefaultPathMappingRules(),
})

// 添加自定义规则
pathMapper.AddRule(PathMappingRule{
    Pattern:     "/home/alice/",
    Replacement: "/home/user/",
    Priority:    100,
    Type:        PathRulePrefix,
})

// 在事件发送前映射路径
event.Path = pathMapper.MapOutgoing(event.Path)
event.Cwd = pathMapper.MapOutgoing(event.Cwd)

// 发送事件...

// 在日志分析时逆映射
realPath := pathMapper.MapIncoming(logEntry.Path)
```

### API 端点

```http
POST /config/path-mapping/rules
{
  "pattern": "/home/alice/",
  "replacement": "/home/user/",
  "priority": 100,
  "type": "prefix"
}

GET /config/path-mapping/rules
→ 返回所有规则

DELETE /config/path-mapping/rules
{
  "pattern": "/home/alice/"
}

POST /config/path-mapping/map-outgoing
{
  "path": "/home/alice/secret.txt"
}
→ { "mapped": "/home/user/secret.txt" }

POST /config/path-mapping/map-incoming
{
  "path": "/home/user/secret.txt"
}
→ { "real": "/home/alice/secret.txt" }
```

---

## 最佳实践

### 1. 规则设计

✅ **推荐**：
```go
// 清晰的前缀规则
Pattern: "/home/alice/"
Replacement: "/home/user/"

// 明确的通配符
Pattern: "/home/*/documents/*.pdf"
```

❌ **不推荐**：
```go
// 过于宽泛
Pattern: "/*"
Replacement: "/data/"

// 复杂的正则（影响性能）
Pattern: `^/([a-z]+)/([0-9]+)/([a-zA-Z0-9_-]+)/.*$`
```

### 2. 优先级分配

```go
// 100+ 特定敏感路径
pm.AddRule(PathMappingRule{
    Pattern: "/etc/secrets/api_key.txt",
    Priority: 110,
    Type: PathRuleExact,
})

// 80-99 用户目录
pm.AddRule(PathMappingRule{
    Pattern: "/home/alice/",
    Priority: 90,
    Type: PathRulePrefix,
})

// 50-79 通用规则
pm.AddRule(PathMappingRule{
    Pattern: "/tmp/",
    Priority: 60,
    Type: PathRulePrefix,
})
```

### 3. 测试

```go
func TestPathMapping(t *testing.T) {
    pm := NewPathMapper(PathMapperConfig{Enabled: true})
    
    // 添加规则
    pm.AddRule(...)
    
    // 测试双向映射
    sanitized := pm.MapOutgoing(realPath)
    recovered := pm.MapIncoming(sanitized)
    
    if recovered != realPath {
        t.Errorf("Round-trip failed: %s != %s", recovered, realPath)
    }
}
```

---

## FAQ

**Q: 路径映射会影响性能吗？**

A: 影响很小（~0.5-2 μs/操作）。精确匹配最快，正则表达式稍慢。

**Q: 可以动态添加/删除规则吗？**

A: 是的，所有操作都是线程安全的，可以随时修改规则。

**Q: 逆映射一定准确吗？**

A: Exact/Prefix/Suffix 类型 100% 准确。Wildcard/Regex 是"最佳努力"，建议用于单向脱敏。

**Q: 支持多对一映射吗？**

A: 技术上支持，但逆映射会混淆。建议避免。

**Q: 规则数量有限制吗？**

A: 无硬性限制，但规则过多会影响性能。建议 < 100 条。

---

## 总结

路径映射提供灵活的双向路径转换能力：

✅ **5 种规则类型**：Exact、Prefix、Suffix、Wildcard、Regex  
✅ **双向映射**：发送时映射、接收时逆映射  
✅ **优先级控制**：支持规则优先级排序  
✅ **高性能**：~0.5-2 μs/操作  
✅ **线程安全**：所有操作都加锁保护  
✅ **持久化**：支持规则导出/导入  

**使用场景**：
- 隐藏用户主目录
- 脱敏敏感文件路径
- 跨环境路径转换
- 日志分析时路径还原

---

**文档版本**：v1.0  
**更新日期**：2026-06-08
