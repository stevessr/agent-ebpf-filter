# TLS 拦截快速入门指南

## 5 分钟上手 TLS 明文捕获

### 前置要求

- Linux 内核 5.8+ (支持 uprobe)
- 后端已构建：`make backend`
- curl 或其他 HTTPS 客户端

### 步骤 1：启动后端

```bash
cd backend
./agent-ebpf-filter
```

**预期输出**：
```
[INFO] eBPF maps loaded from /sys/fs/bpf/agent-ebpf/
[INFO] TLS capture enabled: true
[TLS] Attached 12 uprobes to /usr/lib/x86_64-linux-gnu/libssl.so.3
[TLS] Started Go process discovery loop (interval: 1m)
[INFO] Server listening on :8080
```

### 步骤 2：验证 TLS 库状态

```bash
curl -s http://localhost:8080/api/tls-capture/libraries | jq .
```

**预期输出**：
```json
{
  "libraries": [
    {
      "name": "openssl",
      "path": "/usr/lib/x86_64-linux-gnu/libssl.so.3",
      "attached": true,
      "available": true,
      "error": ""
    }
  ]
}
```

### 步骤 3：执行测试请求

```bash
curl -s https://httpbin.org/get > /dev/null
```

### 步骤 4：查看捕获的明文

```bash
curl -s http://localhost:8080/api/tls-capture/recent?limit=5 | jq '.events[] | {
  process: .comm,
  method: .method,
  url: .url,
  host: .host,
  direction: .direction
}'
```

**预期输出**：
```json
{
  "process": "curl",
  "method": "GET",
  "url": "/get",
  "host": "httpbin.org",
  "direction": "send"
}
{
  "process": "curl",
  "method": "",
  "url": "",
  "host": "httpbin.org",
  "direction": "recv"
}
```

### 步骤 5：查看完整明文数据

```bash
curl -s http://localhost:8080/api/tls-capture/recent?limit=1 | \
  jq -r '.events[0].plaintext'
```

**预期输出**：
```http
GET /get HTTP/1.1
Host: httpbin.org
User-Agent: curl/8.5.0
Accept: */*

```

---

## 进阶使用

### 过滤特定进程

```bash
curl -s "http://localhost:8080/api/tls-capture/recent?filter=comm:curl" | jq .
```

### 过滤特定域名

```bash
curl -s "http://localhost:8080/api/tls-capture/recent?filter=host:github.com" | jq .
```

### 实时监控 WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/tls-capture');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(`[${data.comm}] ${data.method} ${data.host}${data.url}`);
};
```

### 手动附加 Go 程序

```bash
# 查找 Go 程序 PID
ps aux | grep myapp

# 附加 uprobe
curl -X POST http://localhost:8080/api/tls-capture/go-binary \
  -H "Content-Type: application/json" \
  -d '{"path": "/usr/local/bin/myapp", "pid": 12345}'
```

### 手动附加自定义 OpenSSL

```bash
curl -X POST http://localhost:8080/api/tls-capture/library \
  -H "Content-Type: application/json" \
  -d '{"path": "/opt/custom/libssl.so.1.1", "library": "openssl"}'
```

---

## 交互式演示

运行完整演示脚本（包含 5 个场景）：

```bash
./scripts/demo-tls-intercept.sh
```

**菜单选项**：
1. 检查 TLS 捕获状态
2. 启动 TLS 捕获 + 附加默认库
3. 查看已附加的 TLS 库
4. 演示场景 1: 基础 HTTPS 拦截
5. 演示场景 2: 过滤特定进程
6. 演示场景 3: 查看完整明文
7. 演示场景 4: Go 程序 TLS 拦截
8. 演示场景 5: Node.js TLS 拦截
9. 查看最近事件
10. 实时监控模式

### 命令行快速模式

```bash
# 运行所有演示
./scripts/demo-tls-intercept.sh full

# 只运行基础演示
./scripts/demo-tls-intercept.sh demo1

# 实时监控
./scripts/demo-tls-intercept.sh monitor

# 查看最近 20 个事件
./scripts/demo-tls-intercept.sh recent 20

# 过滤特定进程
./scripts/demo-tls-intercept.sh process curl
```

---

## 前端 UI

访问 `http://localhost:8080` 并导航到：

**Network → TLS Capture** 标签页

功能：
- 📊 实时事件流（WebSocket 推送）
- 🔍 按进程/域名/方法过滤
- 📝 查看完整 HTTP 请求/响应明文
- 📈 库状态监控
- ⚙️ 启动/停止捕获控制

---

## 故障排查

### 问题：没有捕获到事件

**检查 1**: 确认 TLS 捕获已启用

```bash
curl http://localhost:8080/api/tls-capture/status | jq .started
# 应该返回 true
```

**检查 2**: 确认库已附加

```bash
curl http://localhost:8080/api/tls-capture/libraries | jq '.libraries[] | select(.attached == true)'
```

**检查 3**: 查看后端日志

```bash
# 应该看到类似输出：
# [TLS] Attached 12 uprobes to libssl.so.3
```

### 问题：Go 程序未自动发现

**原因**: 自动发现循环每分钟运行一次

**解决**: 手动触发附加

```bash
# 获取进程路径
readlink /proc/<PID>/exe

# 手动附加
curl -X POST http://localhost:8080/api/tls-capture/go-binary \
  -d '{"path": "/path/to/binary", "pid": <PID>}'
```

### 问题：权限错误

**错误**: `failed to attach uprobe: operation not permitted`

**原因**: 需要 `CAP_BPF` 或 root 权限

**解决**: 使用 sudo 或添加能力

```bash
sudo ./agent-ebpf-filter
# 或
sudo setcap cap_bpf,cap_sys_admin=ep ./agent-ebpf-filter
```

### 问题：找不到 libssl.so

**错误**: `library not found`

**原因**: OpenSSL 未安装或路径非标准

**解决**: 查找并手动附加

```bash
# 查找 libssl.so
find /usr -name "libssl.so*" 2>/dev/null

# 手动附加
curl -X POST http://localhost:8080/api/tls-capture/library \
  -d '{"path": "/path/to/libssl.so.3", "library": "openssl"}'
```

---

## 性能影响

- **CPU 开销**: <1% (典型负载)
- **内存开销**: ~2 MB (缓冲区)
- **延迟影响**: <100 μs/请求
- **最大捕获**: 17 KB/请求 (自动截断)

---

## 安全注意事项

⚠️ **TLS 明文捕获是高风险功能**

1. **仅用于开发/调试环境**
2. **捕获的明文包含敏感信息**（密码、token、cookies）
3. **建议启用数据脱敏**: Config → Redaction
4. **不要在生产环境启用**
5. **API 端点受认证保护**（需要 X-API-KEY）

启用脱敏：

```bash
curl -X PUT http://localhost:8080/api/config/runtime \
  -H "Content-Type: application/json" \
  -d '{"redactionLevel": "strict"}'
```

---

## 下一步

- 📖 阅读[完整实现报告](TLS_INTERCEPT_COMPLETE.md)了解架构细节
- 🎯 查看[项目 README](../../README.md)了解其他功能
- 🔧 探索前端 UI 的可视化界面
- 🚀 尝试集成到 CI/CD 流程进行 HTTPS 流量审计

---

## 相关文档

- [完整 TLS 拦截报告](TLS_INTERCEPT_COMPLETE.md) - 架构、API、技术细节
- [数据脱敏指南](../../backend/redaction/README.md) - 敏感信息保护
- [eBPF 程序源码](../../backend/ebpf/agent_tls_capture.c) - uprobe 实现
- [主项目文档](../../README.md) - 完整功能介绍
