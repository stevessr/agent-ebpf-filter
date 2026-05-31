# 01 — 根目录、构建与运行层

本层用于回答：项目怎么构建、怎么运行、命令入口在哪里、运行时端口/权限/环境变量怎么工作、验证命令如何选择。

## 根目录关键文件

| 文件/目录 | 作用 | 注意事项 |
| --- | --- | --- |
| `Makefile` | 项目主构建入口 | 大多数开发/验证从这里开始 |
| `go.work` / `go.work.sum` | Go workspace | 覆盖 `backend/`、`wrapper/`、`tools/dev-env-tui/` 等模块 |
| `package.json` / `bun.lock` | 根级 JS workspace / 工具依赖 | 前端真实包在 `frontend/package.json` |
| `frontend/package.json` | Vue 前端脚本和依赖 | `build` = `vue-tsc -b && vite build` |
| `CLAUDE.md` | Claude Code 项目指令 | 修改代码前遵守其中约定 |
| `AGENTS.md` | 贡献者/Agent 约定 | 包含生成文件、auth、devcontainer 等 gotchas |
| `README.md` / `README_cn.md` | 产品说明 | 行为变化常需要同步 |
| `.env.dev` / `.env.dev.mk` | 本地开发环境变量 | 由 dev-env TUI 写入，不要提交 |
| `.port` / `backend/.port` | 后端端口 handoff | Vite dev proxy 会读取 `backend/.port` |
| `agent-wrapper` / `backend/agent-ebpf-filter` | 构建产物 | 不当作源码手改 |

## Make targets 分层

### 一次性准备

```bash
make predev
make deps
```

- `make predev` 并行安装 Go/Python/frontend/TUI 开发依赖并做检查。
- 它会把不可写的 GOPATH（如旧容器里的 `/go`）规整到 `$HOME/go`。
- 前端依赖使用 Bun；JS/TS protobuf 生成仍通过 Node.js 的 `protobufjs-cli` 行为。

### 本地开发

```bash
make dev
make dev-backend
make dev-frontend
make run-backend
make run-frontend
```

- `make dev` 启动 backend hot-reload 和 frontend dev server，使用 Zellij session。
- `scripts/dev-backend.sh` 负责后端热加载。
- `scripts/dev-frontend.sh` 负责前端开发服务。
- 后端会在 `8080..8089` 找第一个可用端口，并写入 `backend/.port`。
- Vite 代理读取 `backend/.port`。

### 构建

```bash
make backend
make frontend
make wrapper
make proto
make all
make build
```

- `make all` = proto + backend + frontend + wrapper。
- `make build` 在 proto 后并行构建 backend/frontend/wrapper。
- `backend-bare` 会在 `backend/ebpf` 里运行多个 `go generate`，生成 tracker/TLS/cgroup/LSM 绑定。
- `frontend-bare` 会 `cd frontend && bun install && bun run build`。
- `wrapper-bare` 会 `cd wrapper && go build -o ../agent-wrapper`。

### eBPF / OS enforcement

```bash
make ebpf-bootstrap
make ebpf-tls
make ebpf-cgroup
make ebpf-lsm
make os-enforcement-preflight
make os-enforcement-check
make os-enforcement-smoke
make os-enforcement-smoke-start
```

- 改 `backend/ebpf/agent_tracker.c`：需要 `cd backend/ebpf && go generate`，然后 `cd backend && go build ./...`。
- 改 `backend/ebpf/cgroup_sandbox.c`：优先 `make ebpf-cgroup`。
- 改 `backend/ebpf/lsm_enforcer.c`：优先 `make ebpf-lsm`。
- OS enforcement 相关 smoke 往往需要特权环境，必要时使用 `OS_SMOKE_PRIVILEGE_CMD='sudo -E'`。

### Dev container

```bash
make dev-image
make dev-image-repository
make dev-image-tag
make docker
make exec
```

- `make dev-image` 打印当前分支对应 GHCR devcontainer image。
- `make docker` 只拉取 GHCR image，不做本地 fallback build。
- `make exec` 创建/进入特权 devcontainer：
  - `--privileged`
  - host PID/network
  - 挂载 `/sys/fs/bpf`、`/sys/kernel/debug`、`/lib/modules`
  - `frontend/node_modules` 和 `adapters/python/.venv` 使用 container-local volume
  - host Git config 只读挂载，不挂载凭据/SSH key/credential store

### 安装与卸载

```bash
make install
make uninstall
```

安装行为：

- 构建 production backend/frontend/wrapper。
- 安装到 `/opt/agent-ebpf-filter`。
- public binaries 到 `/usr/local/bin`。
- 写入 `/etc/agent-ebpf-filter/agent-ebpf-filter.env`。
- 设置 `GIN_MODE=release`、`AGENT_WRAPPER_PATH=/usr/local/bin/agent-wrapper`。
- 通过 `AGENT_REAL_HOME` 让 runtime config 保持在真实用户 `~/.config/agent-ebpf-filter`。
- systemd 优先；无 systemd 时写 rc.local managed block。
- 服务需要保持 privileged/root，因为后端加载 eBPF、cgroup、BPF LSM，也可能绑定 80/443。

## 运行时端口模型

- 后端监听 `8080..8089` 的第一个可用端口。
- 写入 `backend/.port`。
- 前端 Vite dev proxy 读取该文件。
- adapters 也可通过该文件作为本地 fallback。
- native hooks callback URL 也会基于当前端口推导，除非 `AGENT_HOOK_ENDPOINT` 覆盖。
- domain forward proxy 是另一套入口：启用后可绑定 public HTTP/HTTPS 端口（默认 80/443）。

## 权限模型

后端需要特权来管理 eBPF：

1. 启动 backend。
2. 检查当前进程能力。
3. 如不足，通过 `sudo` 或 `pkexec` 自提权。
4. bootstrap/open/pin BPF maps 与 links。
5. 子 shell / 命令尽量降权回调用用户。

重要 pin 路径：

- 主 tracker：`/sys/fs/bpf/agent-ebpf/maps`、`/sys/fs/bpf/agent-ebpf/links`
- cgroup sandbox：`/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps`、`/sys/fs/bpf/agent-ebpf/cgroup_sandbox/links`
- LSM enforcer：`/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps`、`/sys/fs/bpf/agent-ebpf/lsm_enforcer/links`

## 环境变量分组

`Makefile` 的 `DEV_ENV_EXPORTS` 暴露大量变量，常见分组如下：

### Auth / runtime

- `DISABLE_AUTH`
- `GIN_MODE`
- `AGENT_API_KEY`
- `AGENT_ACCESS_TOKEN`
- `AGENT_BACKEND_PORT`
- `AGENT_REAL_HOME`

### Wrapper / shell / hooks

- `AGENT_WRAPPER_PATH`
- `AGENT_HOOK_ENDPOINT`
- `AGENT_SHELL_DIR`
- `AGENT_EBPF_DEV_SESSION`

### Runtime gates

- `AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED`
- `AGENT_RUNTIME_SHELL_SESSIONS_ENABLED`
- `AGENT_RUNTIME_SYSTEM_RUN_ENABLED`
- `AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED`
- `AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED`
- `AGENT_RUNTIME_TLS_CAPTURE_ENABLED`
- `AGENT_RUNTIME_OTLP_ENABLED`
- `AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED`

### ML / LLM

- `AGENT_ML_ENABLED`
- `AGENT_ML_MODEL_TYPE`
- `AGENT_ML_MODEL_PATH`
- `AGENT_ML_AUTO_TRAIN`
- `AGENT_LLM_ENABLED`
- `AGENT_LLM_BASE_URL`
- `AGENT_LLM_API_KEY`
- `AGENT_LLM_MODEL`
- `OPENAI_BASE_URL`
- `OPENAI_API_KEY`
- `OPENAI_MODEL`

### Sandbox / cluster / benchmark

- `AGENT_CGROUP_SANDBOX_PATH`
- `AGENT_EBPF_BOOTSTRAP`
- `AGENT_EBPF_NO_SANDBOX`
- `AGENT_CLUSTER_*`
- `RUNTIME_REPLAY_*`
- `ML_SWEEP_*`

## 验证命令选择

| 改动类型 | 最小验证 | 更完整验证 |
| --- | --- | --- |
| Go backend 普通逻辑 | `cd backend && go test ./...` | `make backend` |
| wrapper | `cd wrapper && go test ./...` | `make wrapper` |
| frontend | `cd frontend && bun run build` | `make frontend` |
| proto | `make proto` | `make all` |
| eBPF tracker | `cd backend/ebpf && go generate` + `cd backend && go build ./...` | `make backend` |
| cgroup/LSM | `make ebpf-cgroup` / `make ebpf-lsm` | OS enforcement smoke |
| dev-env TUI | `cd tools/dev-env-tui && go test ./...` | `make dev-env-doctor` |
| runtime replay | `make runtime-benchmark` | 查看 `reports/runtime-replay-*` |

## RTK 注意事项

本环境有 RTK hook，普通 shell 命令可能自动经 `rtk` 代理。直接使用 RTK meta 命令时显式调用：

```bash
rtk gain
rtk gain --history
rtk discover
rtk proxy <cmd>
```

如果 `rtk gain` 报错，可能装成了同名 Rust Type Kit。