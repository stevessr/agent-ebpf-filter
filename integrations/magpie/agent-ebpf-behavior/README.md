# Agent eBPF Filter · Magpie 行为管理中间件

> **目标软件：** [yetone/magpie](https://github.com/yetone/magpie)，不是 Apache Magpie。
>
> **这是一个提供给 Magpie 使用的插件，不负责安装 Magpie 本体。**

通过 Magpie 官方 Gateway Middleware API（`onRequest`、`onEvent`、`onResponse`）管理经 Magpie 网关发送的 Agent 请求，补充 Agent eBPF Filter 原有的系统调用、文件、进程、网络监测与拦截能力。

## 架构 / 责任边界

```text
Claude Code / Codex / Gemini / ... (连到 Magpie 网关)
                   |
                   v
          yetone/magpie Gateway
                   |
           Agent behavior middleware
             |- onRequest: 请求策略预检，可 403 拦截
             |- onEvent: 流式工具调用元数据记录
             '- onResponse: 非流式工具调用元数据记录
                   |
                   v
         Provider / AI 模型（若未拦截）

Agent 进程 -- syscall/exec/file/network --> Agent eBPF Filter 内核/后端
                                                   |
                                      特权策略与审计，由 eBPF / wrapper 执行
```

**限制要明确：** Magpie 中间件在 Go 内嵌的 moejs 沙箱中执行，不能使用 `fetch`、Node.js、计时器、文件系统或网络 API。因此它不能直接推送事件至本项目的后端，也不能访问/变更 eBPF map。Magpie 插件日志仍由 Magpie 自己持有；本项目内核端的进程、文件和网络观测独立运行。插件绝不会宣称看到了 Magpie 以外的 Agent 请求。

它可以**拦截发往模型的请求**，包括工具能力的声明；但 **`onEvent` / `onResponse` 只能观察模型返回的工具调用，不能将此当作工具执行时的强制沙箱**。真实的命令、路径、网络禁止操作须在 Agent eBPF Filter 的 BPF LSM/cgroup/wrapper 策略中配置。

另外，上游运行时在 JavaScript hook 抛出异常或超时时会保留原请求继续执行（fail-open）。高安全场景不能只依赖此插件。

## 在已有 Magpie 中安装此插件

克隆或拉取本仓库，然后在仓库根目录运行：

```sh
# 官方 CLI 支持本地目录作为插件来源。此命令只给现有 Magpie 安装插件。
magpie plugin add ./integrations/magpie/agent-ebpf-behavior

# 查看插件是否加载
magpie plugin

# 为本地目录安装的插件设置策略（请从项目根目录运行）
magpie plugin options "$(pwd)/integrations/magpie/agent-ebpf-behavior" \
  '{"mode":"enforce","blocked_tools":["Bash","shell*"],"audit":true}'

# 恢复只观察模式
magpie plugin options "$(pwd)/integrations/magpie/agent-ebpf-behavior" \
  '{"mode":"audit","blocked_tools":["Bash","shell*"],"audit":true}'

# 如不再需要
magpie plugin rm "$(pwd)/integrations/magpie/agent-ebpf-behavior"
```

也可以在 Magpie 的 **Settings → Plugins → Add** 中选择本地插件目录，之后在该插件的 **Options** 编辑 JSON 策略。**本插件暂未发布到 npm；不要执行 `magpie plugin add @agent-ebpf-filter/magpie-behavior` 并假定 npm 已存在。**

提示：本地安装指向真实目录；不要移动或删除插件目录，否则 Magpie 将无法加载它。修改 JS 文件后，上游约每秒检查文件更新时间，必要时重新加载插件。

## 策略项（Magpie 的 `ctx.options`）

| 键名 | 类型 | 说明 |
|---|---|---|
| `enabled` | boolean | `false` 全部关闭，默认 `true` |
| `mode` | string | `audit`（默认，仅观察）或 `enforce`（预检拒绝） |
| `blocked_agents` | string[] | 不允许的 Agent 身份；按 Magpie `ctx.agent` 匹配 |
| `allowed_agents` | string[] | 非空时仅允许列出的 Agent；无身份时也拒绝 |
| `blocked_models` | string[] | 拒绝指定模型 |
| `allowed_models` | string[] | 非空时仅允许列出的模型 |
| `blocked_protocols` | string[] | 禁止 `anthropic`、`chat`、`responses`、`gemini` 等协议 |
| `blocked_tools` | string[] | 请求声明了列出的工具时拒绝整个请求 |
| `allowed_tools` | string[] | 非空时拒绝未列入的工具声明，包括无法识别的工具 |
| `blocked_tool_categories` | string[] | 根据工具**名称**推断类别并拒绝；支持 `process`、`file_read`、`file_write`、`network`、`other` |
| `allowed_tool_categories` | string[] | 仅允许指定的工具名称类别，未识别的归入 `other` |
| `blocked_user_terms` | string[] | 禁止用户最近一轮文本中包含指定字面子串（区分不了语义） |
| `max_output_tokens` | integer | enforce 模式下仅压低请求里**已存在**的输出 token 上限，不新增缺失字段 |
| `audit` | boolean | 禁用/启用元数据审计日志，默认启用 |
| `audit_requests` | boolean | 记录未触发拦截的请求，默认关闭 |
| `audit_tool_calls` | boolean | 记录模型拟发起的工具调用名，默认开启 |

规则字符串不区分大小写。支持**精确匹配**与**尾部星号**，例如 `gpt-*`、`shell*`；**不运行用户提供的正则表达式**。单字段最多接受 128 条规则，最多检查 256 条工具声明；只要配置了工具黑名单或白名单，无法识别或超出范围的工具声明均会被视为未知并拦截。检查包括 OpenAI/Anthropic 的强制工具选择（`tool_choice`）、Gemini 的 `toolConfig.functionCallingConfig.allowedFunctionNames` 和内置工具。`enforce` 下格式错误或过大的规则将返回 403 `invalid_policy`，不静默忽略。

用户文本规则仅扫描最新一轮用户文本的前 64 Ki **字符**，跳过工具结果及系统提示；在 `enforce` 模式下，文本超出检查上限且已配置 `blocked_user_terms` 时，会返回 403 `uninspectable_user_text`，防止仅检查前缀造成绕过。文件、图片、音频、工具输出不在此扫描范围内。

> 配置中的空数组表示对相应类别不设限制；`mode: "audit"` 不会修改请求或拒绝请求。要切换到拦截模式，必须在 Magpie 里显式设成 `"enforce"`。

### 样例：限制高风险 Agent 工具

```json
{
  "enabled": true,
  "mode": "enforce",
  "blocked_tools": ["Bash", "shell*", "exec*", "codeExecution"],
  "blocked_models": ["unknown-provider/*"],
  "audit": true,
  "audit_requests": false,
  "audit_tool_calls": true
}
```

本插件只控制向模型**声明工具能力的请求**，不会修改工具实现，也不会验证 Agent 随后实际执行的 Shell 命令。建议同时使用 Agent eBPF Filter 的包装器规则和内核级防护。

### 样例：按行为类别管理工具

```json
{
  "mode": "enforce",
  "blocked_tool_categories": ["process", "file_write"],
  "audit_tool_calls": true
}
```

该选项会阻止带有 Bash、shell_command、codeExecution、Write、Edit、apply_patch 等**已知名称**的工具能力声明；`file_read`、`network`、`other` 为其他分类。分类仅根据工具名称进行启发式匹配：自定义工具名或 MCP 工具可能落到 `other`，并不能推断工具的实际副作用。需要严格白名单时使用 `allowed_tool_categories`，例如只允许 `file_read`；必须由 eBPF/LSM/agent-wrapper 拦截真实的进程、文件和网络行为。

### 样例：允许名单

```json
{
  "mode": "enforce",
  "allowed_agents": ["codex", "claude*"],
  "allowed_models": ["deepseek/*", "openai/*"],
  "allowed_tools": ["Read", "Search", "ListFiles"]
}
```

若工具名在不同 Agent/协议下不同，需逐一加入；否则网关将返回 403。请注意 Magpie 的 `ctx.agent` 可能为空；白名单应在真实 Agent 环境中验证身份值。

## 审计隐私

日志形如：

```text
[agent-ebpf-filter] {"event":"deny","code":"unapproved_tool","agent":"codex","protocol":"chat","tool":"unknown"}
[agent-ebpf-filter] {"event":"tool_call_observed","code":"model_proposed_tool","agent":"codex","protocol":"chat","tool":"Bash"}
```

不记录提示词、响应内容、工具参数、环境变量或密钥。用户定义的工具名和 Agent 名称必须通过安全格式检查才能被记入日志，其他值写成 `unknown`。Magpie 中间件日志按上游实现限制为每分钟 60 行，可能丢弃突发审计记录；不要以其作为完整且可靠的司法/安全审计日志。

## 开发和测试

```sh
node --test integrations/magpie/agent-ebpf-behavior/behavior.test.mjs
```

上述 Node 单元测试无需启动特权后端或 Magpie，也不下载第三方包。CI 还会**检出固定提交 `71f796069f9078295b54e26d1f790bf38e087c6c` 的 yetone/magpie**，将本插件加载到其真正的 moejs 中间件运行时，验证 403 策略拦截、正常请求透传和 SSE 原样透传。该测试不需要 root、内核 eBPF 或真实云端 API 凭据；真实生产 Magpie 环境中的端到端服务部署验证仍单独进行。

参考：[yetone/magpie Gateway middleware reference](https://github.com/yetone/magpie/blob/main/docs/reference.md#gateway-middleware)。
