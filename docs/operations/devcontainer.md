# 开发容器

开发容器提供特权 Linux 环境，适合 eBPF 开发和演示。

---

## 快速开始

```bash
make dev-image    # 打印当前分支的 GHCR 镜像引用
make docker       # 拉取 GHCR 镜像（pull-only，不做本地构建）
make exec         # 启动或附加到特权容器 shell
```

---

## 容器特性

| 配置项 | 说明 |
| --- | --- |
| `privileged` | 启用特权模式（eBPF attach 需要） |
| Host PID | `--pid=host`（进程关联） |
| Host Network | `--network=host`（网络观测） |
| `/sys/fs/bpf` | 挂载 bpffs |
| `/sys/kernel/debug` | 挂载 debugfs |
| `/lib/modules` | 挂载内核模块 |
| `frontend/node_modules` | container-local volume |
| `adapters/python/.venv` | container-local volume |
| Git config | 只读挂载 `~/.gitconfig` 和 `~/.config/git` |
| 凭据 | **不挂载** SSH key、credential store |

---

## GHCR Image 工作流

### Image 命名

镜像引用格式：

```
ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>-<branch-hash>
```

- `branch-hash` = branch 名称 SHA-256 的前 12 位 hex
- 发布 `linux/amd64` 和 `linux/arm64` 多架构 manifest

### 构建流程

1. GitHub Actions devcontainer image workflow 构建镜像
2. 镜像中预运行 `make predev`，依赖缓存在 `/opt/agent-ebpf-predev`
3. Post-create hook 从缓存种子并运行 `make predev-check`（仅验证，不联网安装）

### `make docker` 是 pull-only

`make docker` **不做本地 fallback build**。如果镜像不存在：

- 等待 GitHub Actions workflow 完成
- 或手动触发 workflow

### Detached HEAD

```bash
# 指定分支名
DEV_BRANCH=main make docker

# 或指定完整镜像
DEV_IMAGE=ghcr.io/owner/repo/devcontainer:custom make exec
```

---

## VS Code Dev Containers

`.devcontainer/devcontainer.json` 使用 `image` 字段直接引用 GHCR 镜像。

### 配置要点

- **使用 image 字段**，不使用 Dockerfile 构建
- `.devcontainer/Dockerfile` 仅作为 GitHub Actions 构建输入
- `updateRemoteUserUID` **禁用**，避免生成 `updateUID.Dockerfile`
- Container UID/GID: `1001:1001`

### 分支/Fork 开发

使用 Make targets 获取正确的镜像引用：

```bash
# 获取镜像仓库
make dev-image-repository

# 获取镜像 tag
make dev-image-tag

# 在 VS Code 中使用
DEV_IMAGE_REPOSITORY=$(make dev-image-repository) \
DEV_IMAGE_TAG=$(make dev-image-tag) \
code .
```

---

## Podman 兼容性

| 注意事项 | 说明 |
| --- | --- |
| User namespace | 需要对齐镜像 UID/GID `1001:1001` |
| `--init` + `--pid=host` | Podman 拒绝此组合，不要同时使用 |
| rootless Podman | 可能无法满足 eBPF 特权需求 |

---

## 依赖隔离

`make exec` 和 VS Code Dev Containers 挂载 container-local volumes：

- `frontend/node_modules`
- `adapters/python/.venv`

这样做的原因：

1. 依赖安装从镜像缓存保持可写
2. 不与 host 创建的依赖树冲突
3. 容器内外的 node_modules / venv 互相独立

### Post-create 行为

默认行为：从 `/opt/agent-ebpf-predev` 种子 + `make predev-check`

如需在 post-create 阶段联网安装（不推荐）：

```bash
DEVCONTAINER_POSTCREATE_INSTALL=1
```

---

## 注意事项

- 不要把 host-only dependency tree 复用进 container
- Git config 只读挂载，不挂载凭据和 SSH key
- 如果 post-create 报告缺少依赖，重建或拉取最新镜像

---

## 相关导航

- [构建与运行](build-and-run.md)
- [部署与安装](deployment.md)
- [验证、测试与 Benchmark](verification-benchmark.md)
- [文档地图](../reference/documentation-map.md)
