# 04 — 协议、eBPF 与生成层

本层用于处理 proto、eBPF C 程序、Go/JS/Python 生成物、事件字段跨层同步和“不要手改”的边界。

## Protobuf 源文件

路径：`proto/`

| 文件 | 作用 |
| --- | --- |
| `tracker.proto` | 聚合文件，只 import 其他功能域 proto，保持下游兼容 |
| `tracker_common.proto` | 通用类型 |
| `tracker_events.proto` | 事件类型、事件 payload、event enum |
| `tracker_registration.proto` | Agent register/unregister 协议 |
| `tracker_system.proto` | system stats / runtime system 协议 |
| `tracker_config.proto` | config/runtime/security/ML 等配置协议 |
| `tracker_shell.proto` | shell session 协议 |

改 proto 后运行：

```bash
make proto
```

## Protobuf 生成物

不要手改：

- `backend/pb/tracker_common.pb.go`
- `backend/pb/tracker_config.pb.go`
- `backend/pb/tracker_events.pb.go`
- `backend/pb/tracker_registration.pb.go`
- `backend/pb/tracker_shell.pb.go`
- `backend/pb/tracker_system.pb.go`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- `adapters/python/tracker_*_pb2.py`
- `adapters/js/tracker_pb.js`

如果生成物和源码不一致，以 `proto/*.proto` 为源头重新生成。

## 新增事件字段的同步链

新增/修改事件字段时，按这个顺序：

1. 修改 `proto/tracker_events.proto`。
2. 运行 `make proto`。
3. 修改 eBPF event decode 或 backend event construction。
4. 检查 `backend/network_events.go` 和相关 mapping。
5. 检查 `backend/event_envelope.go` / `event_context*.go` 是否需要带入 context。
6. 检查 WebSocket / archive / persistence 是否自动覆盖。
7. 修改前端 display/filter/table/modal。
8. 修改 AgentSight / ExecutionGraph / OTLP 关联逻辑（如字段会影响观测语义）。
9. 更新文档和 tests。

## 新增配置字段的同步链

1. 修改 `proto/tracker_config.proto`。
2. 运行 `make proto`。
3. 修改 backend runtime/config state。
4. 修改 env override 读取（如需要）。
5. 修改 config handler export/import。
6. 修改 frontend `types/config.ts`。
7. 修改对应 `composables/config/useConfig*.ts`。
8. 修改 Config tab UI。
9. 更新 README/docs/AGENTS。

## eBPF 源文件

路径：`backend/ebpf/`

| 文件 | 作用 |
| --- | --- |
| `agent_tracker.c` | 主 syscall tracing 程序 |
| `agent_tracker_common.h` | 共享结构/常量 |
| `agent_tracker_syscalls.h` | syscall 相关 helper / definitions |
| `agent_tracker_tail.h` | tail call / 分段逻辑 |
| `cgroup_sandbox.c` | cgroup connect / UDP sendmsg 等 OS-level network blocking |
| `lsm_enforcer.c` | BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename enforcement |
| `agent_tls_capture.c` | TLS capture uprobe 相关程序 |
| `gen.go` | tracker go:generate |
| `gen_tls.go` | TLS go:generate |
| `gen_cgroup.go` | cgroup go:generate |
| `gen_lsm.go` | LSM go:generate |
| `vmlinux.h` | BTF 生成 header |

## eBPF 生成物

不要手改：

- `backend/ebpf/agenttracker_bpfel.go`
- `backend/ebpf/agenttracker_bpfeb.go`
- `backend/ebpf/agenttracker_bpfel.o`
- `backend/ebpf/agenttracker_bpfeb.o`
- `backend/ebpf/agenttlscapture_x86_bpfel.go`
- `backend/ebpf/agenttlscapture_x86_bpfel.o`
- `backend/ebpf/agentcgroupsandbox_bpfel.go`
- `backend/ebpf/agentcgroupsandbox_bpfeb.go`
- `backend/ebpf/agentcgroupsandbox_bpfel.o`
- `backend/ebpf/agentcgroupsandbox_bpfeb.o`
- `backend/ebpf/agentlsmenforcer_bpfel.go`
- `backend/ebpf/agentlsmenforcer_bpfeb.go`
- `backend/ebpf/agentlsmenforcer_bpfel.o`
- `backend/ebpf/agentlsmenforcer_bpfeb.o`

## eBPF 修改工作流

### 修改主 tracker

```bash
cd backend/ebpf && go generate
cd ../.. && cd backend && go build ./...
```

或：

```bash
make backend
```

检查同步点：

- C struct layout 是否与 Go decode 匹配。
- event type 是否与 proto/frontend filters 一致。
- map key/value 大小是否与 Go bootstrap/control 匹配。
- ringbuf event 大小是否可接受。

### 修改 cgroup sandbox

```bash
make ebpf-cgroup
cd backend && go build ./...
```

检查同步点：

- `backend/cgroup_sandbox_*.go`
- map pin 路径和权限。
- API status/block/unblock。
- docs/security-model、AGENTS gotchas。

### 修改 LSM enforcer

```bash
make ebpf-lsm
cd backend && go build ./...
```

检查同步点：

- `backend/lsm_enforcer_*.go`
- basename/path matching 语义。
- policy map mutation API。
- docs/threat-model、security-model。

### 修改 TLS eBPF

```bash
make ebpf-tls
cd backend && go build ./...
```

检查同步点：

- `backend/tls_probe_manager.go`
- `backend/tls_capture_controller.go`
- fragment assembler / parser。
- runtime gate 默认关闭。
- 脱敏和 digest 行为。

## Kernel matching 关键事实

- `agent_pids`：PID match，注册进程种子 + fork/clone lineage + userspace parent fallback。
- `tracked_comms`：16-byte command-name exact match。
- `tracked_paths`：256-byte path exact match，不递归。
- cgroup blocklist：基于 cgroup v2 inode id。
- destination blocking：exact IPv4 / IPv6 / TCP/UDP port maps。
- LSM file-name policy：basename-based。
- LSM exec policy：exact path 或 executable basename。

## Generated 文件判断规则

当你看到这些特征，默认是生成物，不直接编辑：

- 文件名包含 `_bpfel.go` / `_bpfeb.go`。
- 文件名是 `.o` BPF object。
- 路径在 `backend/pb/`。
- 路径在 `frontend/src/pb/`。
- Python 文件名以 `tracker_` 开头并以 `_pb2.py` 结尾。
- JS protobuf bundle：`adapters/js/tracker_pb.js`。

如果用户明确要求编辑生成物，也应先说明风险，并优先建议修改源头后重新生成。

## 事件一致性检查表

改事件相关代码后问自己：

- proto enum/message 是否更新？
- Go generated pb 是否更新？
- frontend generated pb 是否更新？
- adapters generated pb 是否更新？
- eBPF event type 常量是否一致？
- backend type→string / string→type mapping 是否一致？
- Dashboard filters 是否显示？
- Network/ExecutionGraph/AgentSight 是否识别？
- JSONL persistence 是否兼容旧数据？
- docs/README 是否仍准确？
