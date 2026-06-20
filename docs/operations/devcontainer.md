# 开发容器

开发容器提供特权 Linux 环境，适合 eBPF 开发和演示。

## 主要命令

```bash
make dev-image
make docker
make exec
```

## 容器特性

- privileged；
- host PID/network；
- 挂载 `/sys/fs/bpf`；
- 挂载 `/sys/kernel/debug`；
- 挂载 `/lib/modules`；
- frontend node_modules 使用 container-local volume；
- Python venv 使用 container-local volume；
- 只读挂载 Git config；
- 不挂载凭据、SSH key、credential store。

## GHCR image

`make docker` 是 pull-only，不做本地 fallback build。若镜像不存在，应等待或运行 GitHub Actions devcontainer image workflow。

## 注意

- 不要把 host-only dependency tree 复用进 container；
- VS Code Dev Containers 使用 image field；
- Podman 场景注意 user namespace 与 `--pid=host` 组合限制。

---

## 相关导航

- [构建与运行](build-and-run.md)
- [部署与安装](deployment.md)
- [验证、测试与 Benchmark](verification-benchmark.md)
- [devcontainer README](../../.devcontainer/README.md)
- [文档地图](../reference/documentation-map.md)
