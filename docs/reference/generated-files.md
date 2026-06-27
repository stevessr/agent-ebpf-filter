# 生成文件边界

以下文件不要手工编辑。应修改源文件后重新生成。

## Protobuf generated

- `backend/pb/*.pb.go`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- `adapters/python/tracker_*_pb2.py`
- `adapters/js/tracker_pb.js`

源头：

```text
proto/*.proto
```

生成：

```bash
make proto
```

## eBPF generated

- `backend/ebpf/*_bpfel.go`
- `backend/ebpf/*_bpfeb.go`
- `backend/ebpf/*.o`

源头：

- `backend/ebpf/agent_tracker.c`
- `backend/ebpf/cgroup_sandbox.c`
- `backend/ebpf/lsm_enforcer.c`
- `backend/ebpf/agent_tls_capture.c`
- `backend/ebpf/gen*.go`

生成：

```bash
cd backend/ebpf && go generate
make ebpf-cgroup
make ebpf-lsm
make ebpf-tls
```

## Build outputs

- `backend/agent-ebpf-filter`
- `agent-wrapper`
- `frontend/dist/`
- `docs/.vitepress/dist/`

## 如果看到：

- `_bpfel.go`
- `_bpfeb.go`
- `.o`
- `pb.go`
- `tracker_pb.js`
- `tracker_pb.d.ts`
- `tracker_*_pb2.py`

优先判断为生成物，不直接编辑。

---

## - [维护检查清单](maintenance-checklists.md)
- [代码入口索引](code-entrypoints.md)
- [协议与事件模型](../architecture/protocol-events.md)
- [构建与运行](../operations/build-and-run.md)
- [文档关系审计](documentation-audit.md)
