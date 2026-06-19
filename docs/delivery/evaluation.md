# 评测报告

评测报告应区分功能验证、性能 benchmark、安全边界验证和文档站构建验证。

## 功能验证

| 项 | 命令 / 方法 | 结果 |
| --- | --- | --- |
| backend tests | `cd backend && go test ./...` | 待填写 |
| frontend build | `cd frontend && bun run build` | 待填写 |
| wrapper tests | `cd wrapper && go test ./...` | 待填写 |
| docs build | `bun run docs:build` | 待填写 |
| proto generation | `make proto` | 待填写 |
| eBPF build | `make backend` | 待填写 |

## OS enforcement 验证

记录：

- kernel version；
- bpffs；
- cgroup v2；
- BPF LSM availability；
- privilege command；
- smoke result。

## Runtime replay benchmark

```bash
make runtime-benchmark
```

报告应包含：

- 输入场景；
- event count；
- p50 / p95 / p99 latency；
- memory；
- CPU；
- 是否启用 ML / TLS / OTLP / persistence。

## 文档站验证

```bash
bun run docs:build
```

检查：

- VitePress 构建通过；
- nav / sidebar 无断链；
- 页面标题和 outline 正常；
- 代码块高亮正常。
