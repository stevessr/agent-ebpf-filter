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
curl -s http://localhost:8080/tls-capture/libraries | jq .
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
curl -s http://localhost:8080/tls-capture/recent?limit=5 | jq '.events[] | {
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
curl -s http://localhost:8080/tls-capture/recent?limit=1 | \
  jq -r '.events[0].body // .events[0].raw_hex_dump'
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
curl -s "http://localhost:8080/tls-capture/recent?filter=comm:curl" | jq .
```

### 过滤特定域名

```bash
curl -s "http://localhost:8080/tls-capture/recent?filter=host:github.com" | jq .
```

### 实时监控 WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/tls-capture');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(`[${data.comm}] ${data.method} ${data.host}${data.url}`);
};
```

### 观察 WebSocket 广播健康度

`/tls-capture/status` 中的 `broadcast` 对象反映实时消费者、待写队列和累计写入故障：

```bash
curl -s http://localhost:8080/tls-capture/status | jq .broadcast
```

```json
{
  "activeClients": 1,
  "queuedEvents": 0,
  "queueCapacity": 64,
  "queueFullDropsTotal": 0,
  "writeFailuresTotal": 0,
  "writeDeadlineFailuresTotal": 0
}
```

- `queueCapacity` 是每个 WebSocket 客户端的容量，`queuedEvents` 是所有活跃客户端的队列总和。
- `queueFullDropsTotal` 持续增长通常表示某个消费者过慢或停滞；后端会断开队列已满的客户端，避免阻塞其他消费者。
- `writeFailuresTotal` 或 `writeDeadlineFailuresTotal` 持续增长时，检查浏览器、反向代理或网络是否频繁断开连接。
- 三个 `*Total` 计数器从当前后端进程启动后累加；重启后重置。

### 手动附加 Go 程序

```bash
# 查找 Go 程序 PID
ps aux | grep myapp

# 附加 uprobe
curl -X POST http://localhost:8080/tls-capture/go-binary \
  -H "Content-Type: application/json" \
  -d '{"path": "/usr/local/bin/myapp", "pid": 12345}'
```

### 手动附加自定义 OpenSSL

```bash
curl -X POST http://localhost:8080/tls-capture/library \
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
- 实时事件流（WebSocket 推送）
- 按进程/域名/方法过滤
- SSL 过滤器表达式搜索（支持 `len>100&data_type=http_request` 语法）
- 数据类型自动分类（HTTP Request/Response, JSON, SSE, gRPC, Binary, Text）
- TLS 握手检测与标记
- UID/TID 进程归属展示
- 查看完整 HTTP 请求/响应明文
- 库状态监控
- WebSocket 广播客户端、队列与失败计数监控
- 启动/停止捕获控制

---

## 故障排查

### 问题：没有捕获到事件

**检查 1**: 确认 TLS 捕获已启用

```bash
curl http://localhost:8080/tls-capture/status | jq .started
# 应该返回 true
```

**检查 2**: 确认库已附加

```bash
curl http://localhost:8080/tls-capture/libraries | jq '.libraries[] | select(.attached == true)'
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
curl -X POST http://localhost:8080/tls-capture/go-binary \
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
curl -X POST http://localhost:8080/tls-capture/library \
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


## SSL 过滤器表达式

前端工具栏和 `app/tls/ssl_filter.go` 支持 AgentSight 兼容的 SSL 过滤表达式语法。

### 语法

```
expression := condition | expression & expression | expression | expression
condition  := field operator value
```

### 支持字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `is_handshake` | bool | TLS 握手记录 |
| `truncated` | bool | 数据被截断 |
| `len` | number | 捕获长度 |
| `pid` | number | 进程 ID |
| `tid` | number | 线程 ID |
| `uid` | number | 用户 ID |
| `timestamp_ns` | number | 时间戳 (纳秒) |
| `latency_ms` | number | 延迟 (毫秒) |
| `data_type` | string | 自动检测类型 (http_request/http_response/json/sse/grpc/binary/text) |
| `direction` | string | 方向 (send/recv) |
| `lib` | string | TLS 库 (openssl/go/gnutls/nss) |
| `function` | string | 函数名 (READ/RECV, WRITE/SEND) |
| `comm` | string | 进程名 |
| `method` | string | HTTP 方法 |
| `url` | string | URL 路径 |
| `host` | string | 主机名 |

### 运算符

| 运算符 | 别名 | 说明 |
|--------|------|------|
| `=` | exact | 精确匹配 |
| `!=` | not_equal | 不等于 |
| `>` | gt | 大于 |
| `<` | lt | 小于 |
| `>=` | gte | 大于等于 |
| `<=` | lte | 小于等于 |
| `~` | contains | 包含子串 |

### 示例

```
# 大于 100 字节的 HTTP 请求
len>100&data_type=http_request

# chunked 编码的读取数据
data~chunked|function=READ/RECV

# TLS 握手记录
is_handshake=true

# 来自特定进程的发送数据
comm=curl&direction=send

# JSON 格式的响应数据
data_type=json&direction=recv
```

### 数据类型自动检测

`DetectSSLDataType()` 函数对 TLS 明文进行自动分类:

| 检测类型 | 匹配规则 |
|---------|---------|
| `http_request` | 以 GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS/CONNECT/TRACE 开头 |
| `http_response` | 以 HTTP/ 开头 |
| `sse` | 以 data:/event:/id:/retry: 开头 |
| `json` | 以 { } 或 [ ] 包裹的有效 JSON |
| `grpc` | gRPC 帧头 (第5字节 0x80 标志) |
| `binary` | 包含空字节或不可打印字符 >25% |
| `text` | 其他可打印文本 |

## 下一步

- 阅读[总体架构](../architecture/overview.md)了解系统设计
- 查看[路由 API 参考](routes-api.md)了解完整 API 索引

## 相关文档

- [后端 API 路由参考](routes-api.md)
- [eBPF 程序源码](../../backend/ebpf/agent_tls_capture.c)
- [TLS 明文解析源码](../../backend/app/tls/httpparsertls.go)
- [SSL 过滤器源码](../../backend/app/tls/ssl_filter.go)
