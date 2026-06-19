# 图表与示例索引

本站在关键章节中使用了 Mermaid 图表和代码示例，方便理解架构、数据流和集成方式。

## 架构图表

### 总体架构

**位置**: [架构总览](/architecture/overview)

包含：
- 系统级总架构图（用户空间 + 内核空间 + 前端）
- L0-L5 分层视图
- 组件交互与依赖关系

### 数据流图

**位置**: [数据流](/architecture/data-flow)

包含：
- eBPF 事件流时序图
- Wrapper 策略流时序图
- Native Hook 流时序图
- PID Registration 流时序图
- 前端配置流时序图
- 导出流组件图

### 后端启动

**位置**: [后端启动链路](/backend/runtime-startup)

包含：
- Main() 启动流程图
- Bootstrap 与特权提升时序图
- eBPF 初始化依赖图
- Runtime settings JSON 示例
- 后台任务启动伪代码

## 安全模型图表

**位置**: [安全模型](/security/model)

包含：
- 五层安全模型总图
- 权限层时序图（CAP check + sudo）
- 认证层时序图（token validation）
- Runtime gate 决策树
- 内核控制层状态机（wrapper/cgroup/LSM）
- 数据保护层流水线

## 集成图表

### Wrapper

**位置**: [Wrapper 命令策略](/integrations/wrapper)

包含：
- Wrapper 执行时序图（含 ML risk scoring）
- 决策类型流程图（ALLOW/BLOCK/ALERT/REWRITE）
- Metadata 提取代码示例
- Wrapper rule 配置 JSON 示例
- 安全边界依赖图

## 前端图表

**位置**: [前端工作台总览](/frontend/workbench)

包含：
- 技术栈依赖图
- 目录分层架构图
- 工作台页面分类图
- 设计原则数据流图
- 典型数据流时序图（Dashboard 实时事件）

## 代码示例位置

### Go 代码示例

| 位置 | 示例内容 |
| --- | --- |
| [后端启动链路](/backend/runtime-startup) | 端口选择、runtime settings 加载 |
| [数据流](/architecture/data-flow) | ringbuf decode 零拷贝实现 |
| [Wrapper](/integrations/wrapper) | metadata 提取、argv digest 计算 |

### JSON 配置示例

| 位置 | 示例内容 |
| --- | --- |
| [后端启动链路](/backend/runtime-startup) | runtime.json 完整配置 |
| [Wrapper](/integrations/wrapper) | wrapper rules 配置 |

### Bash 命令示例

| 位置 | 示例内容 |
| --- | --- |
| [快速开始](/guide/quick-start) | make predev/dev/all |
| [构建与运行](/operations/build-and-run) | eBPF 构建命令 |

## Mermaid 图表类型

本站使用的 Mermaid 图表类型：

- **graph TB/LR**: 架构图、依赖图、分类图
- **sequenceDiagram**: 时序图、交互流程
- **flowchart TD**: 流程图、决策树、状态机

所有图表支持：
- 节点样式（fill 颜色标识关键/危险/成功节点）
- 交互式渲染（VitePress 自动启用）
- 响应式布局

## 使用建议

- **新开发者**：优先查看架构总览和数据流图
- **后端开发**：关注启动链路、事件管线和 eBPF 图表
- **前端开发**：关注前端工作台和数据流时序图
- **安全审查**：关注安全模型五层图和内核控制层
- **集成开发**：关注 Wrapper、Hooks 和 PID Registration 时序图

## 后续可扩展点

未来可继续补充：

- Execution Graph 构建算法流程图
- Network Flow 聚合逻辑图
- ML 训练与推理管线图
- Plugin 编译与加载流程图
- Cluster 心跳与转发拓扑图
- OTLP span 派生规则图
