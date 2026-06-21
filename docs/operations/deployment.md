# 部署与安装

---

## 本机系统服务安装

```bash
make install
```

### 安装行为

`make install` 执行以下步骤：

1. **Production build**：编译 backend、frontend、wrapper
2. **安装服务文件**：复制到 `/opt/agent-ebpf-filter`
3. **安装公开二进制**：`agent-ebpf-filter` 和 `agent-wrapper` → `/usr/local/bin`
4. **写入环境文件**：`/etc/agent-ebpf-filter/agent-ebpf-filter.env`
5. **注册系统服务**：systemd 优先，无 systemd 时 rc.local fallback

### 环境文件

安装器自动配置：

| 变量 | 值 | 说明 |
| --- | --- | --- |
| `GIN_MODE` | `release` | 启用 release mode auth |
| `AGENT_WRAPPER_PATH` | `/usr/local/bin/agent-wrapper` | wrapper 路径 |
| `AGENT_REAL_HOME` | 安装用户的 `$HOME` | 保持 runtime config 在用户目录 |

Runtime 配置持久化在：`~/.config/agent-ebpf-filter/runtime.json`

### systemd vs rc.local

| 安装方式 | 条件 | 服务管理 |
| --- | --- | --- |
| systemd | 当前系统有运行中的 systemd manager | `systemctl start/stop/restart agent-ebpf-filter` |
| rc.local | 无 systemd | `/usr/local/sbin/agent-ebpf-filter-service start/stop` |

### 安装参数

```bash
make install INSTALL_METHOD=systemd      # 强制 systemd
make install INSTALL_METHOD=rc.local     # 强制 rc.local
make install INSTALL_START=0             # 安装但不立即启动
make install INSTALL_ENABLE=0            # 安装但不设置开机启动
```

### 卸载

```bash
make uninstall
```

::: tip 权限说明
安装的服务以 root 运行，因为需要：eBPF 程序加载、cgroup/LSM attach、可选的 80/443 端口绑定。
:::

---

## Kubernetes 部署

部署清单位于 `deploy/kubernetes/`。

### 典型场景

节点级 DaemonSet，每个节点运行一个 Agent eBPF Filter 实例：

```bash
kubectl apply -f deploy/kubernetes/agent-ebpf-filter.yaml
```

### 权限要求

因为需要 eBPF、bpffs、cgroup 和可能的 BPF LSM，Pod 需要：

- `privileged: true` 或精细的 `capabilities`（`CAP_BPF`、`CAP_SYS_ADMIN`）
- 挂载 `/sys/fs/bpf`
- 挂载 `/sys/kernel/debug`
- 挂载 `/lib/modules`
- Host PID namespace（可选，用于进程关联）

### External API 集成

Kubernetes 环境中推荐使用 `/api/v1/*` 版本化接口：

```bash
# 服务发现
kubectl port-forward svc/agent-ebpf-filter 8080:8080
curl http://localhost:8080/api/v1/health
```

详见 [External API](../integrations/external-api.md) 和 [Kubernetes 部署文档](kubernetes.md)。

---

## Domain Forward（域名转发）

可选的 Host/SNI-based HTTP/HTTPS 反向转发，**默认关闭**。

### 启用方式

通过 Runtime Config（`/config/runtime`）配置 `domainForwardProxy`：

```json
{
  "domainForwardProxy": {
    "enabled": true,
    "httpPort": 80,
    "httpsPort": 443,
    "routes": [
      {
        "host": "example.com",
        "upstream": "https://example.com",
        "certFile": "/etc/agent-certs/example.pem",
        "keyFile": "/etc/agent-certs/example.key"
      }
    ]
  }
}
```

### 工作方式

| 协议 | 行为 |
| --- | --- |
| HTTP | 按 `Host` header 路由 |
| HTTPS | 先用默认/路由级证书终止 TLS，再按解密后的 `Host` 路由 |

### 注意事项

- 监听 80/443 需要 root 或 `CAP_NET_BIND_SERVICE`
- HTTPS 需要至少一个默认证书/私钥或路由级证书/私钥
- `host` 支持精确域名和 `*.example.com` 通配
- 转发器使用直接 outbound dial，不继承 `HTTP_PROXY`/`HTTPS_PROXY`
- **Data plane 流量不受 API token 保护**，config 和 status 路由受保护
- 路由变更会重启 forwarding listeners，可能中断活跃连接

---

## 安全注意事项

::: warning
- 系统服务以 root 运行，请确保环境受信
- Release mode 下所有敏感 API 需要 runtime access token
- 高风险能力（PTY / system run / policy mutation / TLS capture）需要 runtime gate 显式启用
- 对外暴露时应放在受信反向代理之后
:::

---

## 相关导航

- [构建与运行](build-and-run.md)
- [开发容器](devcontainer.md)
- [Kubernetes](kubernetes.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [External API](../integrations/external-api.md)
