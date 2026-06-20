# 演示脚本

本页是 VitePress 版本的答辩演示脚本骨架。详细脚本可继续维护 `docs/demo-script.md`。

## 演示 1：启动系统

```bash
make predev
make dev
```

展示：

- 后端端口写入；
- 前端 Dashboard；
- system health；
- runtime config。

## 演示 2：事件采集

操作：注册一个 Agent PID 或添加 tracked command，然后执行命令。

展示：

- Dashboard 事件；
- strace-style summary；
- Event modal；
- Execution Graph 节点。

## 演示 3：网络流

操作：触发本地网络请求。

展示：

- Network page；
- flow table；
- dst port / protocol / interface；
- DNS / SNI / Host enrichment（如有）。

## 演示 4：Wrapper 策略

操作：配置 wrapper rule，然后通过 `agent-wrapper` 执行命令。

展示：

- ALLOW / BLOCK / ALERT / REWRITE；
- wrapper intercept event；
- policy decision。

## 演示 5：OS Enforcement

前提：明确授权 + 特权环境。

展示：

- cgroup exact IP/port blocking；
- BPF LSM basename / exec blocking；
- status counters；
- Security Policies UI。

## 演示 6：录制与回放

展示：

- event recording start / stop；
- replay；
- export。

## 失败兜底

- eBPF 不可用：展示 recorded replay；
- BPF LSM 不可用：展示 wrapper / cgroup 或截图；
- TLS capture 不启用：强调默认关闭并展示 metadata / digest；
- 端口冲突：查看 `backend/.port`；
- 权限失败：切换 devcontainer 或 sudo/pkexec 环境。

---

## 相关导航

- [比赛答辩主线](competition-defense.md)
- [评测报告](evaluation.md)
- [第三方与 AI 使用披露](compliance.md)
- [验证、测试与 Benchmark](../operations/verification-benchmark.md)
- [OS competition defense 草案](../os-competition-defense.md)
