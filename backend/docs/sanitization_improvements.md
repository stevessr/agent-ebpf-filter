# 数据脱敏机制改进计划

## 市场调研总结

### 业界最佳实践 (2026)

根据最新的行业研究，现代数据脱敏方案包含以下关键技术：

#### 1. 核心技术分类

**匿名化 (Anonymization)**：
- 永久移除或改变PII，使个人无法被识别
- 匿名化后的数据不再被视为PII（GDPR）
- **不可逆**

**假名化 (Pseudonymization)**：
- 用人工标识符替换识别字段
- 保持单独的映射表允许重新识别
- 仍被视为PII（GDPR）
- **可逆**

#### 2. 主流技术

| 技术 | 描述 | 用途 | 可逆性 |
|------|------|------|--------|
| **Data Masking** | 替换为真实但虚构的值 | 保持业务功能和数据结构 | 否 |
| **Pseudonymization** | 可逆的token替换 | 需要追溯的场景 | 是 |
| **Generalization** | 降低精度（生日→年份） | 保留统计意义 | 否 |
| **Synthetic Data** | 生成结构真实的虚构记录 | 测试/训练数据 | 否 |
| **Tokenization** | 用non-sensitive token替换 | 支付/凭证 | 是 |
| **Perturbation** | 轻微改变值保持模式 | 统计分析 | 否 |
| **Aggregation** | 合并数据移除个体标识 | 报告/分析 | 否 |
| **Suppression** | 直接移除PII字段 | 不需要的数据 | 否 |

#### 3. 2026年重点趋势

**AI系统的PII处理**：
- 覆盖AI系统的所有输出面：prompts、completions、logs、traces、audit trails
- Legend开源库：在agentic loop的4个边界拦截PII
- Microsoft Presidio + Docling：GenAI管道的自动化PII混淆

**In-Place vs In-Flight**：
- **In-Place Masking**：在存储/数据库中静态脱敏
- **In-Flight Masking**：数据在系统间移动时动态脱敏
- **最佳实践**：两种方法结合使用

**数据中心安全**：
- 传统边界安全不足
- 通过masking和redaction实现数据中心安全是必要的

### 现有实现评估

#### ✅ 已实现的功能

1. **静态替换（Suppression + Data Masking）**
   - PEM密钥、SSH密钥、JWT → 占位符
   - 符合业界的"数据移除"最佳实践

2. **分级脱敏（4个级别）**
   - None/Basic/Standard/Strict
   - 类似于业界的"风险分级"方法

3. **两层防护**
   - SSL Hook（加密材料） + 通用脱敏（字段）
   - 符合"纵深防御"原则

4. **自动集成**
   - 默认启用、透明处理
   - 符合"secure by default"原则

#### ⚠️ 缺失的功能

根据业界最佳实践，我们缺少以下关键功能：

1. **假名化（Pseudonymization）** ❌
   - 可逆的token替换
   - 保持映射表允许追溯
   - **用例**：需要审计追踪但保护隐私

2. **泛化（Generalization）** ❌
   - 降低精度（具体日期→年份）
   - 保留统计意义
   - **用例**：数据分析、报告

3. **格式保留脱敏（Format-Preserving）** ❌
   - 保持数据类型和格式
   - 保持数据长度
   - **用例**：测试环境、开发

4. **一致性脱敏（Consistent Masking）** ⚠️
   - 同一个值在多处出现时使用相同的脱敏结果
   - **用例**：保持数据关联性

5. **上下文感知脱敏（Context-Aware）** ❌
   - 根据数据上下文选择脱敏策略
   - **用例**：智能识别和处理

6. **合规性映射** ⚠️
   - 明确标注符合哪些合规要求
   - GDPR、CCPA、HIPAA、PCI DSS
   - **用例**：合规审计

## 改进建议

### 优先级1：假名化（Pseudonymization）

**为什么重要**：
- GDPR明确区分匿名化和假名化
- 支持审计追踪的同时保护隐私
- 市场上大多数企业解决方案都包含此功能

**实现方案**：

```go
// 假名化引擎
type PseudonymEngine struct {
    mappingStore map[string]string  // 原始值 → 假名
    cipher       cipher.Block        // 加密用于生成假名
    hmacKey      []byte              // HMAC密钥
}

// 生成一致的假名
func (pe *PseudonymEngine) Pseudonymize(value string, category FieldCategory) string {
    // 1. 检查映射表
    if pseudonym, exists := pe.mappingStore[value]; exists {
        return pseudonym
    }
    
    // 2. 生成新假名
    pseudonym := pe.generatePseudonym(value, category)
    
    // 3. 保存映射
    pe.mappingStore[value] = pseudonym
    
    return pseudonym
}

// 反向查找（需要权限）
func (pe *PseudonymEngine) Depseudonymize(pseudonym string) (string, error) {
    // 从映射表反向查找
}
```

**集成方式**：
- 在Standard/Strict级别提供"假名化"选项
- 用户可选择：完全移除 vs 假名化
- 假名化的数据可以用于审计追踪

### 优先级2：泛化（Generalization）

**为什么重要**：
- 保留数据分析价值
- GDPR认可的技术
- 适合统计和报告场景

**实现方案**：

```go
// 泛化器
type Generalizer struct {
    rules map[FieldCategory]GeneralizationRule
}

// IP地址泛化
func (g *Generalizer) GeneralizeIP(ip string) string {
    // 192.168.1.100 → 192.168.1.0/24
    // 保留网段，隐藏主机
}

// 时间戳泛化
func (g *Generalizer) GeneralizeTimestamp(ts time.Time, precision string) time.Time {
    // 具体秒 → 小时 或 天
}

// 路径泛化
func (g *Generalizer) GeneralizePath(path string) string {
    // /home/user/project/src/main.go → /home/*/project/*/main.go
}
```

**配置示例**：
```json
{
  "generalization": {
    "ip_addresses": "subnet",     // 保留子网
    "timestamps": "hour",          // 精确到小时
    "file_paths": "pattern"        // 保留模式
  }
}
```

### 优先级3：格式保留脱敏

**为什么重要**：
- 测试环境需要真实格式的数据
- 保持数据验证逻辑正常工作

**实现方案**：

```go
// 格式保留脱敏
func FormatPreservingMask(value string, dataType DataType) string {
    switch dataType {
    case TypeEmail:
        // user@example.com → fake_abc123@example.com
        // 保持email格式
        
    case TypePhone:
        // +1-234-567-8900 → +1-555-000-1234
        // 保持电话号码格式
        
    case TypeCreditCard:
        // 4532-1234-5678-9010 → 4532-****-****-1234
        // 显示前4后4，中间mask
        
    case TypeSSN:
        // 123-45-6789 → ***-**-6789
        // 保持SSN格式
    }
}
```

### 优先级4：一致性增强

**当前问题**：
- 同一个API密钥在不同位置可能显示为不同的占位符

**改进方案**：

```go
// 一致性脱敏
type ConsistentRedactor struct {
    cache map[string]string  // 原始值 → 一致的脱敏结果
}

func (cr *ConsistentRedactor) RedactConsistently(value string) string {
    if masked, exists := cr.cache[value]; exists {
        return masked  // 返回之前使用的脱敏结果
    }
    
    // 生成新的脱敏结果（确定性的）
    masked := hash(value) + "_MASKED"
    cr.cache[value] = masked
    return masked
}
```

### 优先级5：合规性标注

**实现方案**：

```go
// 合规性映射
type ComplianceMapping struct {
    GDPR  bool  // 符合GDPR
    CCPA  bool  // 符合CCPA
    HIPAA bool  // 符合HIPAA
    PCIDSS bool // 符合PCI DSS
}

// 为每个脱敏级别标注合规性
var complianceMatrix = map[RedactionLevel]ComplianceMapping{
    RedactionLevelNone: {false, false, false, false},
    RedactionLevelBasic: {false, false, false, true},  // PCI DSS
    RedactionLevelStandard: {true, true, false, true}, // GDPR, CCPA, PCI DSS
    RedactionLevelStrict: {true, true, true, true},    // 全部
}
```

### 优先级6：NLP驱动的PII检测

**业界趋势**：
- 使用NER (Named Entity Recognition)
- 上下文感知的PII检测

**实现方案**：

```go
// NLP驱动的检测器
type NLPDetector struct {
    nerModel *ner.Model
}

func (nd *NLPDetector) DetectPII(text string) []PIIEntity {
    entities := nd.nerModel.Extract(text)
    
    var piiEntities []PIIEntity
    for _, entity := range entities {
        if isPII(entity.Type) {
            piiEntities = append(piiEntities, PIIEntity{
                Type:  entity.Type,  // PERSON, EMAIL, PHONE, etc.
                Value: entity.Value,
                Start: entity.Start,
                End:   entity.End,
            })
        }
    }
    return piiEntities
}
```

## 实施路线图

### 阶段1：核心增强（1-2周）

1. ✅ 实现假名化引擎
2. ✅ 实现泛化功能
3. ✅ 增强一致性脱敏
4. ✅ 添加格式保留脱敏

### 阶段2：集成和配置（1周）

1. 集成到现有脱敏流程
2. 添加配置选项
3. 更新前端UI支持新功能
4. 更新Proto定义

### 阶段3：合规和文档（1周）

1. 添加合规性标注
2. 完善文档
3. 添加使用示例
4. 创建合规性报告模板

### 阶段4：高级功能（未来）

1. NLP驱动的PII检测
2. 机器学习模型集成
3. 自定义脱敏策略
4. 脱敏审计和报告

## 参考资料

### 开源工具和框架

1. **Legend (legend-pii)** - Python库，用于AI系统的PII假名化
   - [GitHub](https://github.com/legend-pii)
   - 特点：agentic loop的4个边界拦截

2. **Microsoft Presidio** - PII检测和匿名化
   - [GitHub](https://github.com/microsoft/presidio)
   - 特点：NLP驱动、可扩展

3. **ARX Data Anonymization Tool** - 企业级匿名化
   - [Website](https://arx.deidentifier.org/)
   - 特点：k-anonymity、l-diversity、t-closeness

### 行业标准

1. **NIST Privacy Framework** - 隐私风险管理
2. **ISO/IEC 27701** - 隐私信息管理
3. **GDPR Article 32** - 数据保护措施

## 下一步行动

建议按以下顺序实施：

1. **立即开始**：假名化引擎（最高优先级）
2. **本周完成**：泛化功能和一致性增强
3. **下周开始**：格式保留脱敏
4. **持续改进**：合规性标注和文档

---

## Sources

市场调研基于以下资料：

- [PII Pseudonymization for Agentic Systems](https://deconvoluteai.com/blog/pii-pseudonymization-agentic-systems)
- [Data Masking Best Practices 2026](https://www.synthesized.io/post/data-masking)
- [AI Output PII Redaction Implementation Guide 2026](https://www.digitalapplied.com/blog/ai-output-pii-redaction-implementation-guide-2026)
- [Pseudonymization vs. Anonymization Guide](https://xata.io/blog/pseudonymization-vs-anonymization-which-approach-fits-your-data-strategy)
- [Data Redaction Techniques & Best Practices 2026](https://www.strac.io/blog/data-redaction)
- [What Is Data Masking? 2026 Guide](https://www.redactable.com/blog/what-is-data-masking)
