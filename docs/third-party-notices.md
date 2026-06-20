# 第三方依赖、来源与许可证说明

> 项目：Agent eBPF Filter  
> 用途：满足操作系统设计赛关于“复制的代码和文档来源、使用目的、授权信息、开源协议合规”的说明要求。  
> 源代码协议：仓库根目录 `LICENSE` 当前为 GPL-3.0。  
> 文档 / 答辩材料建议协议：CC-BY-SA 4.0。  
> 状态：草案。最终提交前需要队伍人工核验每个依赖的实际许可证、版本和来源。

---

## 1. 总体声明

Agent eBPF Filter 当前以自研代码为主体，主要第三方内容通过 Go module、前端 package manager、系统工具链、Linux 内核接口文档快照和 protobuf 生成链路引入。

本项目应区分三类内容：

1. **依赖引用**：通过 `go.mod`、`package.json`、系统包、开发工具链引入的第三方库；通常不把第三方源码复制进本项目主体源码中，但最终提交包如果包含 vendor / node_modules / 缓存目录，应额外处理许可证文件。
2. **生成文件**：由项目自身 `proto/*.proto` 或 eBPF C 源文件生成的 Go / JS / Python / BPF object；不应手工修改，许可证跟随项目源码与生成工具要求。
3. **本地文档快照 / 参考资料**：如 `frontend/public/linux-docs/6.18/` 中的 Linux syscall / eBPF helper 本地快照；应明确来源、快照日期、用途和许可证。

如后续直接复制了第三方源码、文档、图片、论文图、往届项目代码片段，必须在复制位置保留来源和许可证声明，并在本文档“直接复制 / 借鉴登记表”中登记。

---

## 2. 项目自身协议

| 内容 | 协议 | 位置 | 说明 |
| --- | --- | --- | --- |
| 项目源代码 | GPL-3.0 | `LICENSE` | 满足赛事要求中 GPL / Apache / BSD / 木兰协议至少一种的条件 |
| 技术文档、答辩材料、PPT、视频说明 | 建议 CC-BY-SA 4.0 | `docs/`、PPT、视频说明 | 按赛事要求，最终材料需显式标注 |
| AI 使用披露记录 | 建议 CC-BY-SA 4.0 | `docs/ai-usage/` | 作为开发相关文档的一部分 |

建议在 README、设计文档和 PPT 封底加入：

```text
源代码按 GPL-3.0 许可发布；技术文档与答辩材料按 CC-BY-SA 4.0 许可发布。
```

---

## 3. Go 后端依赖

来源文件：`backend/go.mod`。

### 3.1 直接依赖清单

| 依赖 | 当前版本 | 主要用途 | 许可证核验状态 |
| --- | --- | --- | --- |
| `github.com/NVIDIA/go-nvml` | `v0.13.0-1` | NVIDIA NVML / GPU 指标能力 | TBD，提交前核验 |
| `github.com/cilium/ebpf` | `v0.21.0` | eBPF 程序加载、map/link 管理 | TBD，提交前核验 |
| `github.com/creack/pty/v2` | `v2.0.1` | PTY shell session | TBD，提交前核验 |
| `github.com/gin-gonic/gin` | `v1.12.0` | HTTP API / 路由 | TBD，提交前核验 |
| `github.com/gorilla/websocket` | `v1.5.3` | WebSocket 实时事件推送 | TBD，提交前核验 |
| `github.com/modelcontextprotocol/go-sdk` | `v1.6.1` | MCP endpoint / tools | TBD，提交前核验 |
| `github.com/pelletier/go-toml/v2` | `v2.2.4` | TOML 配置解析 | TBD，提交前核验 |
| `github.com/shirou/gopsutil/v3` | `v3.24.5` | CPU / memory / process / system metrics | TBD，提交前核验 |
| `github.com/ulikunitz/xz` | `v0.5.15` | 压缩数据处理 | TBD，提交前核验 |
| `github.com/vladimirvivien/go4vl` | `v0.5.0` | Linux video / camera 相关接口 | TBD，提交前核验 |
| `go.opentelemetry.io/otel` | `v1.43.0` | OpenTelemetry trace API | TBD，提交前核验 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | `v1.43.0` | OTLP HTTP trace exporter | TBD，提交前核验 |
| `go.opentelemetry.io/otel/sdk` | `v1.43.0` | OpenTelemetry SDK | TBD，提交前核验 |
| `go.opentelemetry.io/otel/trace` | `v1.43.0` | Trace API | TBD，提交前核验 |
| `golang.org/x/net` | `v0.52.0` | 网络扩展库 | TBD，提交前核验 |
| `golang.org/x/sys` | `v0.42.0` | Linux syscall / 系统接口 | TBD，提交前核验 |
| `google.golang.org/protobuf` | `v1.36.11` | Protobuf runtime | TBD，提交前核验 |

### 3.2 间接依赖

`backend/go.mod` 还包含若干 indirect dependencies，例如：

- `github.com/bytedance/sonic`
- `github.com/gin-contrib/sse`
- `github.com/google/uuid`
- `github.com/grpc-ecosystem/grpc-gateway/v2`
- `github.com/quic-go/quic-go`
- `go.opentelemetry.io/proto/otlp`
- `google.golang.org/grpc`
- `google.golang.org/genproto/*`

最终提交前建议运行许可证扫描工具，生成完整依赖许可证表。例如：

```bash
cd backend
# 示例：如本地安装了 go-licenses，可运行
# go-licenses report ./... > ../reports/go-licenses-backend.txt
```

如果没有自动工具，也应至少人工核验直接依赖和主要间接依赖许可证。

---

## 4. Wrapper 依赖

来源文件：`wrapper/go.mod`。

| 依赖 | 当前版本 / 来源 | 主要用途 | 许可证核验状态 |
| --- | --- | --- | --- |
| `agent-ebpf-filter` | `replace ../backend` | 复用后端 protobuf / 类型 | 项目自身 GPL-3.0 |
| `google.golang.org/protobuf` | `v1.36.11` | wrapper 与后端 UDS protobuf 协议 | TBD，提交前核验 |

Wrapper 是项目自研命令拦截层，源码位于 `wrapper/main.go`。

---

## 5. 前端依赖

来源文件：`frontend/package.json`。

### 5.1 Runtime dependencies

| 依赖 | 当前版本 | 主要用途 | 许可证核验状态 |
| --- | --- | --- | --- |
| `@ant-design/icons-vue` | `^7.0.1` | UI 图标 | TBD，提交前核验 |
| `@wterm/dom` | `0.1.9` | Web terminal DOM 支持 | TBD，提交前核验 |
| `acorn` | `^8.16.0` | JS parser / plugin 相关解析 | TBD，提交前核验 |
| `ant-design-vue` | `^4.2.6` | UI 组件库 | TBD，提交前核验 |
| `apexcharts` | `^5.10.6` | 图表 | TBD，提交前核验 |
| `axios` | `^1.15.1` | HTTP client | TBD，提交前核验 |
| `d3` | `^7.9.0` | 图 / 网络 / 可视化 | TBD，提交前核验 |
| `markdown-it` | `14.1.1` | Markdown 渲染 | TBD，提交前核验 |
| `monaco-editor` | `^0.55.1` | 代码编辑器 | TBD，提交前核验 |
| `protobufjs` | `^8.0.1` | 前端 protobuf runtime | TBD，提交前核验 |
| `shiki` | `^4.0.2` | 代码高亮 | TBD，提交前核验 |
| `smol-toml` | `^1.6.1` | TOML 解析 | TBD，提交前核验 |
| `toml` | `^4.1.1` | TOML 解析 | TBD，提交前核验 |
| `vue` | `^3.5.32` | 前端框架 | TBD，提交前核验 |
| `vue-router` | `4` | 路由 | TBD，提交前核验 |
| `vue3-apexcharts` | `^1.11.1` | Vue 图表封装 | TBD，提交前核验 |

### 5.2 Dev dependencies

| 依赖 | 当前版本 | 主要用途 | 许可证核验状态 |
| --- | --- | --- | --- |
| `@types/acorn` | `^6.0.4` | TypeScript 类型 | TBD |
| `@types/d3` | `^7.4.3` | TypeScript 类型 | TBD |
| `@types/node` | `^24.12.2` | TypeScript 类型 | TBD |
| `@vitejs/plugin-vue` | `^6.0.6` | Vite Vue 插件 | TBD |
| `@vue/tsconfig` | `^0.9.1` | Vue TS config | TBD |
| `protobufjs-cli` | `^2.0.1` | JS protobuf 生成工具 | TBD |
| `typescript` | `5.9.3` | TypeScript 编译 | TBD |
| `vite` | `^8.0.9` | 前端构建工具 | TBD |
| `vue-tsc` | `^3.2.7` | Vue TypeScript typecheck | TBD |

最终提交建议不要提交 `node_modules/`。如果比赛平台要求完整离线包并包含 `node_modules/`，必须保留每个包自带 LICENSE，并另附依赖许可证清单。

---

## 6. Kernel ML 与系统工具链

`kernel-ml/` 是项目自研的 DKMS 内核模块目录，核心文件包括：

- `ml_inference.h`
- `ml_inference.c`
- `kernel_ml_main.c`
- `cuda_infer_helper.cu`
- `model_loader.py`
- `Makefile`
- `dkms.conf`
- `test_module.sh`
- `README.md`

涉及外部工具 / 环境：

| 工具 / 环境 | 用途 | 许可证 / 来源核验状态 |
| --- | --- | --- |
| Linux kernel headers / Kbuild | 构建 DKMS 模块 | 依系统环境，TBD |
| DKMS | 动态内核模块管理 | 依系统环境，TBD |
| clang / ld.lld / gcc | 编译内核模块 / eBPF | 依系统环境，TBD |
| CUDA Toolkit / libcuda / libcudart | 可选 userspace CUDA helper | 依 NVIDIA 条款，TBD |
| Python / sklearn | 模型训练与导出示例 | 若进入最终代码路径需核验，TBD |

注意：CUDA runtime 不能直接链接进 Linux 内核模块；本项目采用“DKMS 内核模块 + userspace CUDA helper”的分层方式，CUDA helper 在用户态运行。

---

## 7. Protobuf 与生成文件

协议源文件位于：

```text
proto/
  tracker.proto
  tracker_common.proto
  tracker_events.proto
  tracker_registration.proto
  tracker_system.proto
  tracker_config.proto
  tracker_shell.proto
```

由 `make proto` 生成：

- `backend/pb/*.pb.go`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- `adapters/python/tracker_*_pb2.py`
- `adapters/js/tracker_pb.js`

说明：

1. `proto/*.proto` 为本项目协议源定义。
2. 生成文件由 protobuf 工具链生成，不手工修改。
3. 生成文件的使用应同时遵守本项目许可证和 protobuf 工具链 / runtime 许可证。
4. 修改协议时应从 `proto/` 源文件改起并重新生成。

---

## 8. eBPF 与 Linux 相关来源

### 8.1 eBPF 程序

项目 eBPF 源文件位于 `backend/ebpf/`，包括：

- `agent_tracker.c`
- `agent_tracker_common.h`
- `agent_tracker_syscalls.h`
- `agent_tracker_tail.h`
- `cgroup_sandbox.c`
- `lsm_enforcer.c`
- `agent_tls_capture.c`
- `gen*.go`

这些文件为项目主体实现。若其中任何代码直接复制自 Linux kernel samples、libbpf examples、博客、往届作品或第三方项目，应在具体文件位置追加来源注释，并在本文档登记。当前草案尚未逐行审计复制来源，状态为 **TBD：待人工核验**。

### 8.2 Linux 文档本地快照

路径：`frontend/public/linux-docs/6.18/`。

该目录 README 当前说明：

- Release：Linux 6.18 LTS；
- Snapshot date：2026-04-28；
- Syscall snapshots：61；
- eBPF helper snapshots：17；
- 用途：Config 页面 popup preview 的本地缓存。

合规要求：

1. 最终提交前应补充原始来源 URL 列表或总来源说明；
2. 应核验 Linux kernel documentation / man-pages / helper docs 的许可证；
3. 若属于复制文档，应在快照文件或目录 README 中明确来源、快照日期和用途；
4. 在答辩文档和 PPT 中说明该目录是用于离线预览的文档快照，不是项目原创系统代码。

---

## 9. 直接复制 / 借鉴登记表

> 当前表格是提交前人工核验模板。若确认没有直接复制第三方源码，可保留一行“未发现直接复制第三方源码，第三方能力通过依赖引入”。若存在复制 / 改写 / 借鉴，应逐项登记。

| 项目位置 | 来源 | 来源许可证 | 使用目的 | 修改说明 | 是否保留原版权声明 | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| `frontend/public/linux-docs/6.18/` | Linux syscall / eBPF helper 文档快照，具体 URL 待补 | TBD | 前端 Config 页面离线预览 | Markdown / HTML 转换快照 | 待核验 | 待补充 |
| `backend/ebpf/*` | 待人工核验是否参考 Linux / libbpf 示例 | TBD | eBPF 程序实现 | TBD | TBD | 待核验 |
| `kernel-ml/*` | 待人工核验是否参考第三方示例 | TBD | DKMS / CUDA helper / 模型加载 | TBD | TBD | 待核验 |
| 其他 | TBD | TBD | TBD | TBD | TBD | 待核验 |

---

## 10. 往届作品 / 开源作品参考说明

赛事允许学习往届功能赛作品或优秀开源作品，但要求在第一次提交版本中标注引用或基于的作品，并在设计文档和 PPT 中说明参考版本与增量贡献。

当前草案尚未确认本项目是否基于某个往届作品或特定开源基础版本。最终提交前必须由队伍确认：

- [ ] 是否基于往届作品；
- [ ] 是否基于某个开源项目 fork；
- [ ] 是否复制 / 改写了第三方模块；
- [ ] 是否在第一次正式提交中清楚标注基础版本；
- [ ] 是否在设计文档和 PPT 中列出增量贡献。

如果没有基础版本，可声明：

```text
本项目未基于某个往届作品或单一开源项目 fork 开发；主要第三方能力通过包管理器依赖引入，核心 eBPF 观测、Go 后端、Vue 前端、wrapper、AgentSight 集成和 kernel-ml 模块由队伍实现。具体依赖与许可证见 third-party notices。
```

该声明必须建立在人工核验事实基础上。

---

## 11. AI 工具相关材料

AI 工具使用披露位置：`docs/ai-usage/README.md`。

应在最终设计文档和 PPT 中说明：

| 工具 | 使用场景 | 成果 | 人工复核 |
| --- | --- | --- | --- |
| Claude Code | 代码阅读、文档草案、结构梳理、测试建议 | 文档草案、规划文档、结构说明、交互记录模板 | 队伍人工核验、修改、运行测试后采纳 |
| 其他 AI 工具 | TBD | TBD | TBD |

AI 输出不应被描述为“未经人工确认的事实”。凡涉及赛事规则、许可证、性能数据、引用来源的内容，最终都应由队伍人工核验。

---

## 12. 最终提交前检查清单

- [ ] 根目录 `LICENSE` 为 GPL-3.0，且 README / PPT 已说明。
- [ ] 文档和答辩材料标注 CC-BY-SA 4.0。
- [ ] Go 直接依赖许可证已核验。
- [ ] Go 间接依赖许可证已核验或有自动扫描报告。
- [ ] 前端 runtime / dev dependencies 许可证已核验。
- [ ] `node_modules/` 不进入最终源码提交，或保留其 LICENSE 并生成清单。
- [ ] `frontend/public/linux-docs/6.18/` 已补充来源和许可证说明。
- [ ] eBPF / kernel-ml 代码已人工核验是否复制第三方示例。
- [ ] 往届作品 / 开源基础版本情况已说明。
- [ ] AI 使用披露记录已补充。
- [ ] 所有外部图片、图标、截图、论文图在 PPT 中逐页标注来源。
- [ ] 如存在第三方复制代码，在复制位置保留来源注释和许可证声明。

---

## 相关导航

- [第三方与 AI 使用披露](delivery/compliance.md)
- [外部资源与最佳实践](reference/external-resources.md)
- [AI 使用记录](ai-usage/README.md)
- [OS competition defense 草案](os-competition-defense.md)
- [文档关系审计](reference/documentation-audit.md)
