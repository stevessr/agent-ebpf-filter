# VitePress 插件配置

本文档站已配置以下插件，支持 Mermaid 图表和 LaTeX 数学公式渲染。

## 已安装插件

### 1. vitepress-plugin-mermaid

**版本**: `^2.0.17`

**用途**: 渲染 Mermaid 图表（流程图、时序图、架构图）

**配置位置**: `docs/.vitepress/config.ts`

```typescript
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(defineConfig({
  // ...
  mermaid: {
    // Mermaid configuration options
  }
}))
```

**使用方法**:

````markdown
```mermaid
graph LR
    A[开始] --> B[处理]
    B --> C[结束]
```
````

### 2. markdown-it-mathjax3

**版本**: `^5.2.0`

**用途**: 渲染 LaTeX 数学公式（行内和块级）

**配置位置**: `docs/.vitepress/config.ts`

```typescript
import mathjax3 from 'markdown-it-mathjax3'

export default withMermaid(defineConfig({
  // ...
  markdown: {
    config: (md) => {
      md.use(mathjax3)
    }
  }
}))
```

**使用方法**:

**行内公式**:
```markdown
这是一个行内公式 $E = mc^2$。
```

**块级公式**:
```markdown
$$
\frac{-b \pm \sqrt{b^2 - 4ac}}{2a}
$$
```

## 示例页面

### Mermaid 图表示例

本站多个页面使用了 Mermaid 图表：

- [总体架构](/architecture/overview) - 系统架构图、分层视图
- [数据流](/architecture/data-flow) - 时序图、组件流程图
- [后端启动链路](/backend/runtime-startup) - 启动流程图
- [安全模型](/security/model) - 五层安全模型、时序图、状态机
- [Wrapper 命令策略](/integrations/wrapper) - 策略决策流程图
- [前端工作台](/frontend/workbench) - 技术栈依赖图、数据流

查看完整列表：[图表与示例索引](/guide/diagrams-and-examples)

### LaTeX 数学公式示例

查看：[性能分析与数学模型](/reference/performance-models)

包含：
- 零拷贝优化性能公式
- 事件丢失率模型
- ML 风险评分算法
- 贝叶斯信誉更新
- 网络流聚合模型
- 缓存命中率公式
- 内存占用估算

## 构建与开发

### 开发模式

```bash
bun run docs:dev
```

访问 `http://localhost:5173`，支持热重载。

### 生产构建

```bash
bun run docs:build
```

输出目录：`docs/.vitepress/dist/`

### 预览生产构建

```bash
bun run docs:preview
```

## 插件特性

### Mermaid 支持的图表类型

本站使用的类型：
- `graph TB/LR` - 有向图（架构图、依赖图）
- `flowchart TD` - 流程图（决策树、状态机）
- `sequenceDiagram` - 时序图（交互流程）

未使用但支持的类型：
- `classDiagram` - UML 类图
- `stateDiagram` - 状态图
- `erDiagram` - 实体关系图
- `gantt` - 甘特图
- `pie` - 饼图

### LaTeX 支持的语法

**常用符号**:
- 希腊字母: $\alpha, \beta, \gamma, \lambda, \mu, \sigma, \theta, \rho$
- 运算符: $\sum, \prod, \int, \frac{a}{b}, \sqrt{x}, x^2, x_i$
- 关系: $\leq, \geq, \approx, \in, \subset$
- 逻辑: $\land, \lor, \neg, \forall, \exists$

**矩阵**:
```latex
$$
\begin{bmatrix}
a & b \\
c & d
\end{bmatrix}
$$
```

**分段函数**:
```latex
$$
f(x) = \begin{cases}
x^2 & \text{if } x \geq 0 \\
-x & \text{if } x < 0
\end{cases}
$$
```

**对齐方程**:
```latex
$$
\begin{aligned}
a &= b + c \\
  &= d + e
\end{aligned}
$$
```

## 故障排查

### Mermaid 图表不渲染

1. 检查 `withMermaid` 是否正确包裹 `defineConfig`
2. 检查 Mermaid 语法是否正确（使用 [Mermaid Live Editor](https://mermaid.live/) 验证）
3. 确认代码块使用 ````mermaid` 标记

### LaTeX 公式不渲染

1. 确认 `markdown-it-mathjax3` 已正确导入和配置
2. 检查公式语法（使用 `$...$` 或 `$$...$$`）
3. 避免在公式中使用未转义的特殊字符

### 构建失败

1. 清理缓存：`rm -rf docs/.vitepress/.temp docs/.vitepress/cache`
2. 重新安装依赖：`rm -rf node_modules bun.lock && bun install`
3. 检查 TypeScript 类型错误

## 维护

### 更新插件

```bash
bun update vitepress-plugin-mermaid markdown-it-mathjax3
```

### 检查依赖版本

```bash
bun pm ls | grep -E "vitepress|mermaid|mathjax"
```

## 参考

- [VitePress 官方文档](https://vitepress.dev/)
- [vitepress-plugin-mermaid](https://github.com/emersonbottero/vitepress-plugin-mermaid)
- [markdown-it-mathjax3](https://github.com/tani/markdown-it-mathjax3)
- [Mermaid 文档](https://mermaid.js.org/)
- [MathJax 文档](https://docs.mathjax.org/)
