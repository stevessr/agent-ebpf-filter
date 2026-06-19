# 验证、测试与 Benchmark

## 最小验证表

| 改动 | 命令 |
| --- | --- |
| Markdown / docs | `bun run docs:build` |
| backend Go | `cd backend && go test ./...` |
| wrapper | `cd wrapper && go test ./...` |
| frontend | `cd frontend && bun run build` |
| proto | `make proto` + backend/frontend build |
| main eBPF | `cd backend/ebpf && go generate` + `cd backend && go build ./...` |
| cgroup | `make ebpf-cgroup` |
| LSM | `make ebpf-lsm` |
| TLS eBPF | `make ebpf-tls` |
| runtime replay | `make runtime-benchmark` |

## OS enforcement smoke

```bash
make os-enforcement-preflight
make os-enforcement-check
make os-enforcement-smoke
make os-enforcement-smoke-start
```

需要特权时：

```bash
OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start
```

## Benchmark 文档

- `docs/benchmark.md`
- `docs/ml-benchmark-report.md`
- `benchmarks/`
- `reports/runtime-replay-*`

## 报告规范

引用性能数据时必须记录：

- 日期；
- 机器 / kernel / CPU / RAM；
- 命令；
- build tags；
- runtime settings；
- 样本规模；
- 成功 / 失败 / 跳过原因。
