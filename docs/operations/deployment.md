# 部署与安装

## 本机安装

```bash
make install
```

安装行为：

- production backend / frontend / wrapper build；
- 安装到 `/opt/agent-ebpf-filter`；
- public binaries 到 `/usr/local/bin`；
- 写入 `/etc/agent-ebpf-filter/agent-ebpf-filter.env`；
- 设置 `GIN_MODE=release`；
- 设置 `AGENT_WRAPPER_PATH=/usr/local/bin/agent-wrapper`；
- 使用 `AGENT_REAL_HOME` 保持 runtime config 在真实用户 home；
- systemd 优先，无 systemd 时 rc.local fallback。

卸载：

```bash
make uninstall
```

## Kubernetes

部署清单位于：

```text
deploy/kubernetes/
```

典型场景是节点级 DaemonSet。因为需要 eBPF、bpffs、cgroup 和可能的 BPF LSM，部署权限应被明确审查。

## Domain forward

可选 Host/SNI-based forward proxy 默认关闭。启用后可绑定 80/443：

- HTTP 按 Host 转发；
- HTTPS 需要默认或 route-level cert/key；
- data plane 不由 API token 保护；
- config 和 status 受 auth 保护。

---

## 相关导航

- [构建与运行](build-and-run.md)
- [开发容器](devcontainer.md)
- [Kubernetes](../kubernetes.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [External API](../external-api.md)
