# TLS 明文捕获 — 设计文档

日期: 2026-05-10

## 目标

通过 eBPF uprobes 挂载到主流加密库（OpenSSL、BoringSSL、Go crypto/tls、GnuTLS、NSS），
在加解密之前/之后捕获 TLS 通信的明文数据，作为通用 HTTPS 审计日志的一部分。
完整明文通过分片拼装机制保留，不截断。

## 非目标

- 不做中间人代理（MITM）
- 不修改目标进程的内存或控制流
- 不注入证书或劫持连接
- 不负责数据安全归档（加密存储由已有日志系统负责）

## 架构

```
  ┌─────────────────────────────────────────────────────────────────┐
  │                        Linux host                               │
  │  ┌─────────────────────┐                                        │
  │  │ uprobe handlers      │   tls_events ringbuf                  │
  │  │ SSL_write / SSL_read │ ──────────────────────────────┐       │
  │  │ tls.Write / tls.Read │                                │       │
  │  │ gnutls_record_send   │                                ▼       │
  │  │ PR_Write / PR_Read   │               ┌─────────────────────┐  │
  │  └─────────────────────┘               │ Go TLS 拼装引擎      │  │
  │                                        │ - 分片缓冲区          │  │
  │  ┌─────────────────────┐               │ - HTTP/JSON 解析     │  │
  │  │ tracepoint 处理器    │  events       │ - WebSocket 推送     │  │
  │  │ (syscall 追踪)       │  ringbuf      └────────┬────────────┘  │
  │  └─────────────────────┘                         │               │
  │                                                  ▼               │
  │                                        ┌─────────────────────┐  │
  │                                        │ Vue 前端             │  │
  │                                        │ TLSCapture.vue 新增 │  │
  │                                        └─────────────────────┘  │
  └─────────────────────────────────────────────────────────────────┘
```

## 组件详解

### 1. eBPF 侧 — `backend/ebpf/agent_tls_capture.c`

#### 1.1 数据结构

```c
#define TLS_FRAG_SIZE 960
#define TLS_MAX_FRAGS 64

// 库类型枚举
#define TLS_LIB_OPENSSL   0
#define TLS_LIB_GO        1
#define TLS_LIB_GNUTLS    2
#define TLS_LIB_NSS       3

#define TLS_DIR_RECV 0
#define TLS_DIR_SEND 1

struct tls_fragment {
    u64 timestamp_ns;
    u32 pid;
    u32 tgid;
    u32 data_len;
    u32 total_len;
    u16 frag_index;
    u16 frag_count;
    u8  lib_type;
    u8  direction;
    char comm[16];
    char data[TLS_FRAG_SIZE];
};
```

独立 ringbuf:

```c
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} tls_events SEC(".maps");
```

#### 1.2 Hook 函数

**通用分片发送逻辑**（所有库共用）:

```c
static __always_inline int emit_tls_fragment(void *ctx, const void *buf,
                                              u32 total_len, u8 lib, u8 dir) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u64 now_ns = bpf_ktime_get_ns();

    u16 frag_count = (total_len + TLS_FRAG_SIZE - 1) / TLS_FRAG_SIZE;
    if (frag_count > TLS_MAX_FRAGS) return 0;

    for (u16 i = 0; i < frag_count; i++) {
        struct tls_fragment *f = bpf_ringbuf_reserve(&tls_events, sizeof(*f), 0);
        if (!f) break;

        f->timestamp_ns = now_ns;
        f->pid = (u32)pid_tgid;
        f->tgid = (u32)(pid_tgid >> 32);
        f->frag_index = i;
        f->frag_count = frag_count;
        f->total_len = total_len;
        f->lib_type = lib;
        f->direction = dir;

        u32 offset = i * TLS_FRAG_SIZE;
        u32 chunk = total_len - offset;
        if (chunk > TLS_FRAG_SIZE) chunk = TLS_FRAG_SIZE;
        f->data_len = chunk;

        bpf_get_current_comm(&f->comm, sizeof(f->comm));

        if (bpf_probe_read_user(f->data, chunk, buf + offset) < 0) {
            bpf_ringbuf_discard(f, 0);
            break;
        }

        bpf_ringbuf_submit(f, 0);
    }
    return 0;
}
```

**OpenSSL/BoringSSL**:

```c
// SSL_write(SSL *ssl, const void *buf, int num)
SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    u32 len = (u32)PT_REGS_PARM3(ctx);
    if (len == 0 || len > 16384) return 0;
    return emit_tls_fragment(ctx, buf, len, TLS_LIB_OPENSSL, TLS_DIR_SEND);
}

// SSL_read 用 uretprobe，在函数返回后读取 buf 内容
SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx) {
    s32 ret = (s32)PT_REGS_RC(ctx);
    if (ret <= 0) return 0;
    // 需要 enter 时保存 buf 指针，通过 pid_tgid map 传递
    // 详见 1.3 Go 侧的 retprobe 处理
    return 0;
}
```

**注意**：`SSL_read` 和其他读取函数的 buffer 在 uretprobe 时数据已经到达，
但需要通过 enter-probe 保存 buffer 地址，exit-probe 读取数据。
这和 syscall enter/exit 模式类似。er 数据保存方式:

- sys_enter 时将 buffer 地址存入 per-CPU map
- sys_exit/uretprobe 时从 per-CPU map 取出地址，读取数据

**Go crypto/tls**:

Go 使用自定义 ABI，函数签名为 `func (*Conn) Write(b []byte) (int, error)`。
需要通过 ELF 符号解析获取函数虚拟地址。Go slice 结构为 `{ptr, len, cap}`。

```c
SEC("uprobe/crypto_tls_Conn_Write")
int uprobe_go_tls_write(struct pt_regs *ctx) {
    // Go ABI: AX=*Conn, BX=data.ptr, CX=data.len
    // 在 Linux amd64 上通过 PT_REGS_PARM 系列不一定直接对得上,
    // 需要根据 Go 1.17+ register-based ABI 调整
    const void *buf = (const void *)PT_REGS_PARM3(ctx);  // data ptr
    u32 len = (u32)PT_REGS_PARM4(ctx);                    // data len
    if (len == 0 || len > 16384) return 0;
    return emit_tls_fragment(ctx, buf, len, TLS_LIB_GO, TLS_DIR_SEND);
}
```

**GnuTLS**:

```c
// gnutls_record_send(gnutls_session_t, const void *data, size_t, unsigned)
SEC("uprobe/gnutls_record_send")
int uprobe_gnutls_send(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    u32 len = (u32)PT_REGS_PARM3(ctx);
    if (len == 0 || len > 16384) return 0;
    return emit_tls_fragment(ctx, buf, len, TLS_LIB_GNUTLS, TLS_DIR_SEND);
}
```

**NSS**:

```c
// PR_Write(PRFileDesc *fd, const void *buf, PRInt32 amount)
SEC("uprobe/PR_Write")
int uprobe_nss_write(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    u32 len = (u32)PT_REGS_PARM3(ctx);
    if (len == 0 || len > 16384) return 0;
    return emit_tls_fragment(ctx, buf, len, TLS_LIB_NSS, TLS_DIR_SEND);
}
```

#### 1.3 uretprobe（读取方向）处理

对于读取函数（SSL_read, tls.Read, gnutls_record_recv, PR_Read），
需要在 uprobe 阶段保存传入的 buffer 指针，在 uretprobe 阶段读取数据。

使用 per-CPU array map 传递 buffer 地址:

```c
struct retprobe_ctx {
    void *buf;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct retprobe_ctx);
} retprobe_buf SEC(".maps");

// enter probe: 保存 buf 指针
SEC("uprobe/SSL_read")
int uprobe_ssl_read(struct pt_regs *ctx) {
    u32 zero = 0;
    struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &zero);
    if (rc) rc->buf = (void *)PT_REGS_PARM2(ctx);
    return 0;
}

// exit probe: 读取返回的明文
SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx) {
    s32 ret = (s32)PT_REGS_RC(ctx);
    if (ret <= 0) return 0;
    u32 zero = 0;
    struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &zero);
    if (!rc || !rc->buf) return 0;
    return emit_tls_fragment(ctx, rc->buf, (u32)ret, TLS_LIB_OPENSSL, TLS_DIR_RECV);
}
```

### 2. Go 后端

#### 2.1 文件组织

- **新文件** `backend/tls_capture.go` — uprobe 管理、分片拼装、HTTP 解析、API 端点
- **修改** `backend/main.go` — 初始化 `TLSProbeManager`, 注册 `/ws/tls-capture` 路由
- **修改** `backend/ebpf/gen.go` — 添加 `agent_tls_capture.c` 的 go generate 规则
- **修改** `backend/ebpf_runtime.go` — 支持 uprobe attach/detach

#### 2.2 TLSProbeManager

```go
type TLSProbeManager struct {
    objs       bpf.AgentTlsCaptureObjects
    links      []link.Link
    assembler  *FragmentAssembler
    goSymbols  map[string]uint64  // binary path → "crypto/tls.(*Conn).Write" offset
    mu         sync.Mutex
    targetPIDs map[uint32]bool    // agent_pids 的内容缓存
}

func NewTLSProbeManager(pidMap *ebpf.Map) (*TLSProbeManager, error)
func (m *TLSProbeManager) AttachStaticLibs() error       // OpenSSL, GnuTLS, NSS
func (m *TLSProbeManager) DiscoverGoProcesses()           // /proc 扫描，ELF 解析
func (m *TLSProbeManager) AttachGoUprobes(binPath string, pid int) error
func (m *TLSProbeManager) ReadLoop(wsClients *sync.Map)   // ringbuf 读取循环
func (m *TLSProbeManager) Close()
```

**静态库路径搜索策略**：

```go
var staticLibPatterns = []string{
    // OpenSSL
    "/usr/lib/libssl.so.3", "/usr/lib/libssl.so.1.1",
    "/usr/lib/x86_64-linux-gnu/libssl.so.3",
    "/usr/lib64/libssl.so.3",
    // BoringSSL (通常编译进应用程序)
    // 用户可配置自定义路径
    // GnuTLS
    "/usr/lib/libgnutls.so.30", "/usr/lib/libgnutls.so",
    // NSS
    "/usr/lib/libssl3.so", "/usr/lib/libnspr4.so",
    // LibreSSL
    "/usr/lib/libssl.so.51", "/usr/lib/libtls.so",
}
```

**Go 二进制符号解析**：

```go
func parseGoSymbols(binPath string) (map[string]uint64, error) {
    f, _ := elf.Open(binPath)
    syms, _ := f.Symbols()
    offsets := make(map[string]uint64)
    for _, s := range syms {
        if s.Name == "crypto/tls.(*Conn).Write" ||
           s.Name == "crypto/tls.(*Conn).Read" {
            offsets[s.Name] = s.Value
        }
    }
    return offsets, nil
}
```

**Go 进程发现**：每 60s 扫描 `/proc/*/exe`，找到 tracked PID 中运行 Go 二进制的新进程，
解析 ELF 符号后 attach uprobe。

#### 2.3 FragmentAssembler 分片拼装

```go
type FragmentAssembler struct {
    mu       sync.Mutex
    pending  map[fragKey]*fragmentBuffer
    timeout  time.Duration  // 5s
}

type fragKey struct {
    Tgid        uint32
    TimestampNS uint64
    Direction   uint8 // 0=recv, 1=send
}

type fragmentBuffer struct {
    fragments   map[uint16][]byte
    totalLen    uint32
    fragCount   uint16
    received    uint16
    createdAt   time.Time
    libType     uint8
    comm        string
    pid         uint32
}
```

拼装逻辑：收到分片 → 按 `frag_index` 存储 → 标记 `received` → 当 `received == fragCount` 时触发拼装 → 调用 HTTP 解析器 → 清理 buffer。

超时清理器每 10s 运行一次，丢弃超过 5s 未收齐的 buffer。

#### 2.4 HTTP 协议解析

拼装完成后的明文 buffer 进行 HTTP 解析：

```go
type TLSPlaintextEvent struct {
    Timestamp   time.Time
    Pid         uint32
    Comm        string
    Direction   string // "send" | "recv"
    LibType     string // "OpenSSL" | "Go" | "GnuTLS" | "NSS"

    // HTTP parsed
    Method      string // GET/POST/...
    URL         string
    Host        string
    StatusCode  int
    Headers     map[string]string
    Body        string // 截断至 16KB
    ContentType string

    // Raw
    RawHexDump  string // 可选 hex dump
}
```

- 出站（send）：解析 HTTP request line → method/URL/headers/body
- 入站（recv）：解析 HTTP status line → status code/headers/body
- JSON body 自动美化缩进
- 非 HTTP 协议（如 WebSocket、gRPC）记录为 raw bytes hex dump

#### 2.5 WebSocket API

**`GET /ws/tls-capture`**：

```json
{
  "type": "tls_plaintext",
  "timestamp": "2026-05-10T15:30:00Z",
  "pid": 12345,
  "comm": "claude",
  "direction": "send",
  "lib": "Go",
  "host": "api.anthropic.com",
  "method": "POST",
  "url": "https://api.anthropic.com/v1/messages",
  "status": 0,
  "headers": {"content-type": "application/json", "x-api-key": "***REDACTED***"},
  "body": "{\"model\":\"claude-opus-4-7\",\"messages\":[...]}",
  "body_size": 2048,
  "raw_available": false
}
```

敏感 header 自动脱敏（Authorization, x-api-key, Cookie, Set-Cookie）。

#### 2.6 管理 API

- `GET /api/tls-capture/recent?limit=100` — 最近 N 条明文记录
- `GET /api/tls-capture/libraries` — 当前已 attach 的库列表及状态
- `POST /api/tls-capture/go-binary` — 手动注册 Go 二进制路径

### 3. 前端 — `frontend/src/views/TLSCapture.vue`

#### 3.1 布局

```
┌─────────────────────────────────────────────────────────┐
│  TLS 明文日志                   [搜索...]  [过滤器 ▼]    │
├─────────────────────────────────────────────────────────┤
│  ◀ 自动滚动                                连接状态 ●    │
│                                                         │
│  ┌─ 15:30:01.234  claude  ▶ Send  Go   ──────────────┐  │
│  │ POST /v1/messages → api.anthropic.com:443         │  │
│  │ Body: {"model":"claude-opus-4-7",...}  2.0 KB     │  │
│  │ [展开]  [复制 curl]  [复制 body]                   │  │
│  └──────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─ 15:30:03.567  claude  ◀ Recv  Go   ──────────────┐  │
│  │ 200 OK  ← api.anthropic.com:443                    │  │
│  │ Body: {"id":"msg_xxx",...}  5.2 KB                 │  │
│  │ [展开]  [复制 body]                                │  │
│  └──────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─ 15:30:05.890  curl    ▶ Send  OpenSSL  ──────────┐  │
│  │ POST /v1/chat/completions → api.openai.com:443     │  │
│  │ Body: {"model":"gpt-4",...}  1.5 KB                │  │
│  │ [展开]  [复制 curl]  [复制 body]                    │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

#### 3.2 功能

- 实时 WebSocket 连接，自动滚动
- 过滤器：进程名（comm）、库类型（checkbox）、方向（出站/入站）、域名模糊匹配
- 搜索框：在 body/URL/headers 中全文搜索，高亮关键词
- 展开：点击展开显示完整 body（JSON 格式化+语法高亮）
- 复制 curl：从 request event 构造 `curl` 等价命令
- 复制 body：一键复制 body 内容
- 每条 event 显示字节数、耗时标签
- 侧边栏统计：今日总捕获数、按进程分布饼图

#### 3.3 路由

```ts
{ path: '/tls-capture', name: 'tls-capture', component: () => import('@/views/TLSCapture.vue') }
```

侧边栏新增 "TLS 捕获" 菜单项。

### 4. 构建变更

**`backend/ebpf/gen.go`** ：

```go
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -type tls_fragment -type retprobe_ctx AgentTlsCapture agent_tls_capture.c -- -I/usr/include/bpf -I.
```

**Makefile** 新增目标：

```makefile
.PHONY: ebpf-tls
ebpf-tls:
    cd backend/ebpf && go generate gen_tls.go
```

### 5. 边界情况与错误处理

| 场景 | 处理 |
|---|---|
| 目标库未安装 | 跳过，不报错，API 端点返回空状态 |
| Go 二进制去符号 (stripped) | 跳过，记录 warning 日志 |
| 分片超时未收齐 | 10s 清理器丢弃，记录 dropped_fragment counter |
| ringbuf 满 | eBPF 侧直接 discard（已有 collector_stats） |
| HTTP 解析失败（非 HTTP 协议） | 存为 raw hex dump，body 字段显示 hex |
| 进程退出时有未完成分片 | pid 退出时清理 pending buffer |
| 超大 body (>1MB) | 截断至 16KB，标记 truncated |
| uprobe attach 失败（权限/冲突） | 跳过该库，继续尝试其他库 |

### 6. eBPF 验证器考量

- `emit_tls_fragment` 中的 for 循环需要 `#pragma unroll` 或限制迭代次数
- `bpf_probe_read_user` 在 uprobe 上下文中可用（用户空间指针）
- ringbuf reserve 在循环中可能受限于 verifier 的指令上限
- `SSL_read` 的 uretprobe 需要额外的 per-CPU map 传递 ctx
- 总体 stack 使用控制在 512 字节限制内

### 7. 安全考量

- 敏感 HTTP header 在 Go 侧脱敏（Authorization, x-api-key, Cookie）
- TLS 明文日志端点受现有认证保护（X-API-KEY）
- Body 截断至 16KB 防止内存膨胀
- 可选：用户可在 Config 页面关闭 TLS 捕获功能
- 不记录到 SQLite/文件，仅内存环形缓冲区（16MB max）+ JSONL 可选持久化

### 8. 实现顺序

1. eBPF 侧 `agent_tls_capture.c` + `gen.go` 编译验证
2. Go 侧 `TLSProbeManager` + ringbuf 读取 + 分片拼装
3. HTTP 协议解析器 + WebSocket API
4. Go 二进制符号解析 + 动态发现
5. 前端 `TLSCapture.vue` + 路由
6. 集成测试（模拟 HTTPS 请求验证端到端流程）
