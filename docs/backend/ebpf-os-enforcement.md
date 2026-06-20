# eBPF 与 OS Enforcement

本项目的内核能力分为三类：主 syscall tracker、cgroup 网络阻断、BPF LSM 文件/执行阻断。TLS uprobe capture 是可选高风险诊断能力。

## 主 tracker

源码：

- `backend/ebpf/agent_tracker.c`
- `backend/ebpf/agent_tracker_common.h`
- `backend/ebpf/agent_tracker_syscalls.h`
- `backend/ebpf/agent_tracker_tail.h`
- `backend/ebpf/gen.go`

覆盖 syscall：

- `execve`
- `openat`
- `connect`
- `mkdirat`
- `unlinkat`
- `ioctl`
- `bind`
- `sendto`
- `recvfrom`

## pinned maps

主 tracker：

```text
/sys/fs/bpf/agent-ebpf/maps
/sys/fs/bpf/agent-ebpf/links
```

核心 maps：

- `agent_pids`
- `tracked_comms`
- `tracked_paths`
- `events`

## cgroup sandbox

源码：

- `backend/ebpf/cgroup_sandbox.c`
- `backend/ebpf/gen_cgroup.go`
- `backend/app/*cgroup*`

语义：

- attach 到 cgroup v2；
- 支持 connect4 / connect6；
- 支持 sendmsg4 / sendmsg6；
- cgroup blocklist 使用 cgroup v2 inode id；
- destination blocking 使用 exact IPv4 / IPv6 / TCP/UDP port maps；
- IPv4-mapped IPv6 会归一化到 IPv4 key。

::: warning 不要误写
不要把 cgroup destination blocking 描述成 CIDR / range / 防火墙规则树。它是 exact key map。
:::

## BPF LSM enforcer

源码：

- `backend/ebpf/lsm_enforcer.c`
- `backend/ebpf/gen_lsm.go`
- `backend/app/*lsm*`

覆盖：

- `bprm_check_security`：exec；
- `file_open`；
- `file_permission`；
- `mmap_file`；
- `file_mprotect`；
- `inode_setattr`；
- `inode_create`；
- `inode_link`；
- `inode_symlink`；
- `inode_unlink`；
- `inode_mkdir`；
- `inode_rmdir`；
- `inode_mknod`；
- `inode_rename`。

匹配语义：

- exec：exact path 或 executable basename；
- file / directory：basename-based；
- policy maps restrictive permissions；
- mutation 通过 authenticated backend API。

## TLS uprobe capture

源码：

- `backend/ebpf/agent_tls_capture.c`
- `backend/ebpf/gen_tls.go`
- `backend/app/tls_*`
- `backend/codex/capture/handlers/handlers.go`

安全边界：

- 默认关闭；
- 需要 `FeatureTLSCapture` 编译进来；
- 需要 runtime `TlsCaptureEnabled`；
- 需要 auth；
- 需要 redaction；
- 普通事件只携带 metadata / digest。

## 修改 eBPF 的验证

```bash
cd backend/ebpf && go generate
cd ../.. && cd backend && go build ./...
```

cgroup / LSM：

```bash
make ebpf-cgroup
make ebpf-lsm
```

## 相关文档与同步点

| 主题 | 应同步文档 |
| --- | --- |
| syscall 覆盖、ringbuf payload、生成绑定 | [协议与事件模型](/architecture/protocol-events)、[事件管线](/backend/event-pipeline)、[生成文件边界](/reference/generated-files) |
| cgroup / LSM 策略语义 | [策略语义](/security/policy-semantics)、[安全模型](/security/model)、[Runtime Gates 与 Auth](/security/runtime-gates-auth) |
| Security Policies UI 或前端状态展示 | [前端工作台](/frontend/workbench)、[路由与功能页](/frontend/routes-and-pages)、[组件与 Composables](/frontend/components-composables) |
| OS smoke / preflight / benchmark | [验证、测试与 Benchmark](/operations/verification-benchmark)、[部署与安装](/operations/deployment) |
| kernel-ml / CUDA helper / proc/sysfs UAPI | [ML、Plugins 与扩展能力](/backend/ml-plugins)、[内核 ML 实现](/backend/kernel-ml-implementation)、[kernel-ml/README](../../kernel-ml/README.md) |

::: tip 表述边界
本页描述“内核事实采集”和“内核确定性阻断”。ML / LLM / wrapper 可以产生风险判断或建议策略，但真正同步写入 cgroup / LSM map 仍受 [Runtime Gates 与 Auth](/security/runtime-gates-auth) 约束。
:::
