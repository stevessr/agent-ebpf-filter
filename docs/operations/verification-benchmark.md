# 验证、测试与 Benchmark

## 最小验证表

| 改动 | 命令 |
| --- | --- |
| Markdown 链接 / 文档互链 | `python3 scripts/check-doc-links.py` |
| VitePress 页面 / nav / mermaid | `bun run docs:build` |
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

## 文档验证

文档站同时包含 VitePress 页面、仓库组件 README、历史专题文档和外部参考快照。建议按两层验证：

```bash
# 轻量、仓库感知：检查本仓库 Markdown 链接和源码路径
python3 scripts/check-doc-links.py

# 可选：输出弱入链 / 弱出链页面，辅助补文档关系
python3 scripts/check-doc-links.py --report

# 渲染级：检查 VitePress frontmatter、sidebar、mermaid、代码块等
bun run docs:build
```

`scripts/check-doc-links.py` 默认排除 `docs/ref/**`，因为其中包含外部快照和上游原始链接；需要审计参考快照时再显式使用 `--include-ref`。

## 报告规范

引用性能数据时必须记录：

- 日期；
- 机器 / kernel / CPU / RAM；
- 命令；
- build tags；
- runtime settings；
- 样本规模；
- 成功 / 失败 / 跳过原因。

---

## 相关导航

- [构建与运行](build-and-run.md)
- [部署与安装](deployment.md)
- [Benchmark](runtime-replay-benchmark.md)
- [评测报告](../delivery/evaluation.md)
- [文档关系审计](../reference/documentation-audit.md)

