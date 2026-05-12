# TLS 明文捕获 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 agent-ebpf-filter 增加基于 eBPF uprobes 的 TLS 明文捕获、后端拼装解析、实时 WebSocket/API 与 Vue 可视化页面。

**Architecture:** 新增独立 `agent_tls_capture.c` 和 bpf2go 对象，不侵入现有 syscall tracepoint 事件通道。Go 后端用 `TLSProbeManager` 管理 uprobe attach、ringbuf 读取、分片拼装、HTTP 解析和内存环形归档，再通过 `/ws/tls-capture` 与 `/tls-capture/*` API 暴露给前端。Vue 前端新增 `TLSCapture.vue`，复用现有 `buildWebSocketUrl()`、axios 鉴权和 Ant Design Vue 组件。

**Tech Stack:** C eBPF + cilium/ebpf bpf2go/link/ringbuf、Go 1.26、Gin、gorilla/websocket、Vue 3 `<script setup lang="ts">`、Ant Design Vue、Bun/Vite。

---

## File Structure

- Create `backend/ebpf/agent_tls_capture.c` — TLS uprobe 程序、fragment 结构、ringbuf map、retprobe buffer map。
- Create `backend/ebpf/gen_tls.go` — `go generate` 入口，生成 `AgentTlsCapture` bpf2go 对象。
- Modify `backend/ebpf/gen.go` — 保留现有 syscall 生成规则，不混入 TLS 对象。
- Modify `Makefile` — `backend`、`backend-bare`、`ebpf-bootstrap`、`clean` 纳入 TLS bpf2go 生成和清理。
- Create `backend/tls_capture_types.go` — TLS fragment Go 镜像结构、公开 JSON event、状态结构和常量。
- Create `backend/tls_fragment_assembler.go` — 分片拼装、超时清理、内存限制。
- Create `backend/tls_http_parser.go` — HTTP request/response 解析、JSON body 格式化、敏感 header 脱敏、raw hex fallback。
- Create `backend/tls_capture_store.go` — 最近事件归档、统计计数、库 attach 状态。
- Create `backend/tls_probe_manager.go` — bpf2go 对象加载、library/Go uprobe attach、ringbuf read loop、关闭逻辑。
- Create `backend/tls_capture_handlers.go` — `/ws/tls-capture`、`/tls-capture/recent`、`/tls-capture/libraries`、`/tls-capture/go-binary` handlers。
- Modify `backend/main.go` — 初始化 TLS manager，注册路由，在进程退出时关闭。
- Create backend tests:
  - `backend/tls_fragment_assembler_test.go`
  - `backend/tls_http_parser_test.go`
  - `backend/tls_capture_store_test.go`
  - `backend/tls_probe_manager_test.go`
  - `backend/tls_capture_handlers_test.go`
- Create `frontend/src/views/TLSCapture.vue` — TLS 明文日志 UI。
- Modify `frontend/src/router/index.ts` — 增加 `/tls-capture` 路由。
- Modify `frontend/src/App.vue` — 增加顶部菜单项和选中状态。
- Modify docs after behavior lands:
  - `README.md`
  - `docs/architecture.md`
  - `backend/README.md`
  - `frontend/README.md`

---

### Task 1: Establish clean baseline

**Files:**
- No source edits.

- [ ] **Step 1: Run backend tests before editing**

Run:
```bash
make test
```
Expected: command exits 0. If it fails, capture the failing package/test names and ask whether to investigate before implementing TLS capture.

- [ ] **Step 2: Run frontend build before editing**

Run:
```bash
cd frontend && bun run build
```
Expected: command exits 0 and Vite emits `dist/` output. If it fails, capture the TypeScript/Vite diagnostics and ask whether to fix baseline first.

- [ ] **Step 3: Run eBPF generation before editing**

Run:
```bash
cd backend/ebpf && go generate
```
Expected: command exits 0 and existing `agenttracker_bpf*.go` files regenerate without compile errors.

---

### Task 2: Add TLS eBPF source and generation target

**Files:**
- Create: `backend/ebpf/agent_tls_capture.c`
- Create: `backend/ebpf/gen_tls.go`
- Modify: `Makefile:5-16`, `Makefile:77-95`, `Makefile:134-142`
- Test: `backend/ebpf` generation command

- [ ] **Step 1: Create TLS bpf2go generator file**

Write `backend/ebpf/gen_tls.go`:
```go
package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -target amd64 -type tls_fragment -type retprobe_ctx AgentTlsCapture agent_tls_capture.c -- -I.
```

- [ ] **Step 2: Create minimal verifier-friendly TLS eBPF program**

Write `backend/ebpf/agent_tls_capture.c`:
```c
//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define TLS_FRAG_SIZE 960
#define TLS_MAX_FRAGS 18
#define TLS_MAX_CAPTURE_SIZE (TLS_FRAG_SIZE * TLS_MAX_FRAGS)

#define TLS_LIB_OPENSSL 0
#define TLS_LIB_GO 1
#define TLS_LIB_GNUTLS 2
#define TLS_LIB_NSS 3

#define TLS_DIR_RECV 0
#define TLS_DIR_SEND 1

struct tls_fragment {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tgid;
    __u32 data_len;
    __u32 total_len;
    __u16 frag_index;
    __u16 frag_count;
    __u8 lib_type;
    __u8 direction;
    char comm[16];
    char data[TLS_FRAG_SIZE];
};

struct retprobe_ctx {
    void *buf;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} tls_events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct retprobe_ctx);
} retprobe_buf SEC(".maps");

static __always_inline int emit_tls_fragment(const void *buf, __u32 total_len, __u8 lib, __u8 dir) {
    if (!buf || total_len == 0 || total_len > TLS_MAX_CAPTURE_SIZE) {
        return 0;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 now_ns = bpf_ktime_get_ns();
    __u16 frag_count = (total_len + TLS_FRAG_SIZE - 1) / TLS_FRAG_SIZE;
    if (frag_count == 0 || frag_count > TLS_MAX_FRAGS) {
        return 0;
    }

#pragma unroll
    for (__u16 i = 0; i < TLS_MAX_FRAGS; i++) {
        if (i >= frag_count) {
            break;
        }
        struct tls_fragment *f = bpf_ringbuf_reserve(&tls_events, sizeof(*f), 0);
        if (!f) {
            break;
        }

        __u32 offset = (__u32)i * TLS_FRAG_SIZE;
        __u32 chunk = total_len - offset;
        if (chunk > TLS_FRAG_SIZE) {
            chunk = TLS_FRAG_SIZE;
        }

        f->timestamp_ns = now_ns;
        f->pid = (__u32)pid_tgid;
        f->tgid = (__u32)(pid_tgid >> 32);
        f->data_len = chunk;
        f->total_len = total_len;
        f->frag_index = i;
        f->frag_count = frag_count;
        f->lib_type = lib;
        f->direction = dir;
        bpf_get_current_comm(&f->comm, sizeof(f->comm));

        if (bpf_probe_read_user(f->data, chunk, (const char *)buf + offset) < 0) {
            bpf_ringbuf_discard(f, 0);
            break;
        }
        bpf_ringbuf_submit(f, 0);
    }
    return 0;
}

static __always_inline int save_retprobe_buf(void *buf) {
    __u32 zero = 0;
    struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &zero);
    if (rc) {
        rc->buf = buf;
    }
    return 0;
}

static __always_inline void *load_retprobe_buf(void) {
    __u32 zero = 0;
    struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &zero);
    if (!rc) {
        return 0;
    }
    void *buf = rc->buf;
    rc->buf = 0;
    return buf;
}

SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    __u32 len = (__u32)PT_REGS_PARM3(ctx);
    return emit_tls_fragment(buf, len, TLS_LIB_OPENSSL, TLS_DIR_SEND);
}

SEC("uprobe/SSL_read")
int uprobe_ssl_read(struct pt_regs *ctx) {
    return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx) {
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    if (ret <= 0) {
        return 0;
    }
    return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_OPENSSL, TLS_DIR_RECV);
}

SEC("uprobe/gnutls_record_send")
int uprobe_gnutls_record_send(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    __u32 len = (__u32)PT_REGS_PARM3(ctx);
    return emit_tls_fragment(buf, len, TLS_LIB_GNUTLS, TLS_DIR_SEND);
}

SEC("uprobe/gnutls_record_recv")
int uprobe_gnutls_record_recv(struct pt_regs *ctx) {
    return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/gnutls_record_recv")
int uretprobe_gnutls_record_recv(struct pt_regs *ctx) {
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    if (ret <= 0) {
        return 0;
    }
    return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_GNUTLS, TLS_DIR_RECV);
}

SEC("uprobe/PR_Write")
int uprobe_pr_write(struct pt_regs *ctx) {
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    __u32 len = (__u32)PT_REGS_PARM3(ctx);
    return emit_tls_fragment(buf, len, TLS_LIB_NSS, TLS_DIR_SEND);
}

SEC("uprobe/PR_Read")
int uprobe_pr_read(struct pt_regs *ctx) {
    return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/PR_Read")
int uretprobe_pr_read(struct pt_regs *ctx) {
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    if (ret <= 0) {
        return 0;
    }
    return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_NSS, TLS_DIR_RECV);
}

SEC("uprobe/crypto_tls_Conn_Write")
int uprobe_crypto_tls_conn_write(struct pt_regs *ctx) {
#if defined(__TARGET_ARCH_x86)
    const void *buf = (const void *)PT_REGS_PARM2(ctx);
    __u32 len = (__u32)PT_REGS_PARM3(ctx);
#else
    const void *buf = 0;
    __u32 len = 0;
#endif
    return emit_tls_fragment(buf, len, TLS_LIB_GO, TLS_DIR_SEND);
}

SEC("uprobe/crypto_tls_Conn_Read")
int uprobe_crypto_tls_conn_read(struct pt_regs *ctx) {
#if defined(__TARGET_ARCH_x86)
    return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
#else
    return 0;
#endif
}

SEC("uretprobe/crypto_tls_Conn_Read")
int uretprobe_crypto_tls_conn_read(struct pt_regs *ctx) {
    __s32 ret = (__s32)PT_REGS_RC(ctx);
    if (ret <= 0) {
        return 0;
    }
    return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_GO, TLS_DIR_RECV);
}
```

- [ ] **Step 3: Update Makefile generation and cleanup**

Modify `Makefile` so eBPF generation runs both generators:
```makefile
.PHONY: all backend frontend wrapper clean proto proto-check help predev predev-go predev-python predev-frontend dev run deps ebpf-bootstrap ebpf-tls cuda ml-sweep ml-presentation runtime-benchmark test build

backend-bare:
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go
	cd backend && go build -o agent-ebpf-filter

backend: cuda proto ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go
	cd backend && go build -o agent-ebpf-filter

ebpf-bootstrap: ## Pre-build the backend binary (bootstrap happens automatically on first run)
	@(cd backend/ebpf && go generate && go generate gen_tls.go)
	@(cd backend && go build -o agent-ebpf-filter)

ebpf-tls: ## Generate TLS capture eBPF bindings
	@(cd backend/ebpf && go generate gen_tls.go)
```

Modify `clean` TLS entries:
```makefile
	rm -f backend/ebpf/agenttlscapture_bpfel.go backend/ebpf/agenttlscapture_bpfeb.go
	rm -f backend/ebpf/agenttlscapture_bpfel.o backend/ebpf/agenttlscapture_bpfeb.o
```

- [ ] **Step 4: Generate TLS eBPF bindings**

Run:
```bash
cd backend/ebpf && go generate gen_tls.go
```
Expected: exits 0 and creates `agenttlscapture_bpfel.go`, `agenttlscapture_bpfeb.go`, `agenttlscapture_bpfel.o`, `agenttlscapture_bpfeb.o`.

- [ ] **Step 5: Commit**

```bash
git add Makefile backend/ebpf/agent_tls_capture.c backend/ebpf/gen_tls.go backend/ebpf/agenttlscapture_bpfel.go backend/ebpf/agenttlscapture_bpfeb.go backend/ebpf/agenttlscapture_bpfel.o backend/ebpf/agenttlscapture_bpfeb.o
git commit -m "feat: add TLS capture eBPF program"
```

---

### Task 3: Add TLS capture types and fragment assembler

**Files:**
- Create: `backend/tls_capture_types.go`
- Create: `backend/tls_fragment_assembler.go`
- Test: `backend/tls_fragment_assembler_test.go`

- [ ] **Step 1: Write assembler tests first**

Create `backend/tls_fragment_assembler_test.go`:
```go
package main

import (
	"bytes"
	"testing"
	"time"
)

func TestFragmentAssemblerCompletesOutOfOrderFragments(t *testing.T) {
	assembler := NewFragmentAssembler(5 * time.Second)
	second := tlsFragmentSample(1234, 99, tlsDirectionSend, tlsLibOpenSSL, 1, 2, []byte("world"), 10)
	first := tlsFragmentSample(1234, 99, tlsDirectionSend, tlsLibOpenSSL, 0, 2, []byte("hello"), 10)

	if event, ok := assembler.Add(second); ok || event != nil {
		t.Fatalf("second fragment completed event = %#v, ok = %v", event, ok)
	}
	event, ok := assembler.Add(first)
	if !ok || event == nil {
		t.Fatalf("expected completed event")
	}
	if got := string(event.Payload); got != "helloworld" {
		t.Fatalf("payload = %q, want helloworld", got)
	}
	if event.TotalLen != 10 || event.LibType != tlsLibOpenSSL || event.Direction != tlsDirectionSend {
		t.Fatalf("unexpected metadata %#v", event)
	}
}

func TestFragmentAssemblerDropsExpiredBuffers(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Millisecond)
	fragment := tlsFragmentSample(4321, 88, tlsDirectionRecv, tlsLibGnuTLS, 0, 2, []byte("partial"), 14)
	if event, ok := assembler.Add(fragment); ok || event != nil {
		t.Fatalf("partial fragment completed")
	}
	time.Sleep(20 * time.Millisecond)
	if dropped := assembler.CleanupExpired(time.Now()); dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if pending := assembler.Pending(); pending != 0 {
		t.Fatalf("pending = %d, want 0", pending)
	}
}

func TestFragmentAssemblerRejectsInvalidFragment(t *testing.T) {
	assembler := NewFragmentAssembler(5 * time.Second)
	fragment := tlsFragmentSample(1, 1, tlsDirectionSend, tlsLibGo, 2, 2, []byte("bad"), 3)
	if event, ok := assembler.Add(fragment); ok || event != nil {
		t.Fatalf("invalid fragment completed")
	}
	if pending := assembler.Pending(); pending != 0 {
		t.Fatalf("pending = %d, want 0", pending)
	}
}

func tlsFragmentSample(tgid uint32, timestamp uint64, direction uint8, lib uint8, index uint16, count uint16, payload []byte, total uint32) tlsFragment {
	var data [tlsFragmentSize]byte
	copy(data[:], payload)
	var comm [16]byte
	copy(comm[:], []byte("curl"))
	return tlsFragment{
		TimestampNS: timestamp,
		PID:         tgid,
		TGID:        tgid,
		DataLen:     uint32(len(payload)),
		TotalLen:    total,
		FragIndex:   index,
		FragCount:   count,
		LibType:     lib,
		Direction:   direction,
		Comm:        comm,
		Data:        data,
	}
}

func TestCompletedFragmentCopiesPayload(t *testing.T) {
	assembler := NewFragmentAssembler(5 * time.Second)
	fragment := tlsFragmentSample(7, 77, tlsDirectionRecv, tlsLibNSS, 0, 1, []byte("abc"), 3)
	event, ok := assembler.Add(fragment)
	if !ok || event == nil {
		t.Fatalf("expected completed event")
	}
	fragment.Data[0] = 'z'
	if !bytes.Equal(event.Payload, []byte("abc")) {
		t.Fatalf("payload mutated to %q", event.Payload)
	}
}
```

- [ ] **Step 2: Run assembler tests to verify failure**

Run:
```bash
cd backend && go test -run 'TestFragmentAssembler|TestCompletedFragment' ./...
```
Expected: FAIL with undefined `NewFragmentAssembler`, `tlsFragment`, or TLS constants.

- [ ] **Step 3: Add shared TLS types**

Create `backend/tls_capture_types.go`:
```go
package main

import "time"

const (
	tlsFragmentSize = 960
	tlsMaxFragments = 18
)

const (
	tlsLibOpenSSL uint8 = iota
	tlsLibGo
	tlsLibGnuTLS
	tlsLibNSS
)

const (
	tlsDirectionRecv uint8 = 0
	tlsDirectionSend uint8 = 1
)

type tlsFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	DataLen     uint32
	TotalLen    uint32
	FragIndex   uint16
	FragCount   uint16
	LibType     uint8
	Direction   uint8
	Comm        [16]byte
	Data        [tlsFragmentSize]byte
}

type completedTLSFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	Comm        string
	LibType     uint8
	Direction   uint8
	TotalLen    uint32
	Payload     []byte
}

type TLSPlaintextEvent struct {
	Type        string            `json:"type"`
	Timestamp   time.Time         `json:"timestamp"`
	PID         uint32            `json:"pid"`
	TGID        uint32            `json:"tgid"`
	Comm        string            `json:"comm"`
	Direction   string            `json:"direction"`
	Lib         string            `json:"lib"`
	Method      string            `json:"method,omitempty"`
	URL         string            `json:"url,omitempty"`
	Host        string            `json:"host,omitempty"`
	StatusCode  int               `json:"status,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	BodySize    int               `json:"body_size"`
	ContentType string            `json:"content_type,omitempty"`
	RawHexDump  string            `json:"raw_hex_dump,omitempty"`
	RawAvailable bool             `json:"raw_available"`
	Truncated   bool              `json:"truncated"`
}

type TLSLibraryStatus struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

type TLSCaptureStats struct {
	RecentEvents     int `json:"recent_events"`
	DroppedFragments int `json:"dropped_fragments"`
	PendingFragments int `json:"pending_fragments"`
}
```

- [ ] **Step 4: Add fragment assembler implementation**

Create `backend/tls_fragment_assembler.go`:
```go
package main

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

type FragmentAssembler struct {
	mu      sync.Mutex
	pending map[fragKey]*fragmentBuffer
	timeout time.Duration
	dropped int
}

type fragKey struct {
	TGID        uint32
	TimestampNS uint64
	Direction   uint8
}

type fragmentBuffer struct {
	fragments map[uint16][]byte
	totalLen  uint32
	fragCount uint16
	received  uint16
	createdAt time.Time
	libType   uint8
	comm      string
	pid       uint32
	tgid      uint32
}

func NewFragmentAssembler(timeout time.Duration) *FragmentAssembler {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &FragmentAssembler{pending: make(map[fragKey]*fragmentBuffer), timeout: timeout}
}

func (a *FragmentAssembler) Add(fragment tlsFragment) (*completedTLSFragment, bool) {
	if fragment.FragCount == 0 || fragment.FragCount > tlsMaxFragments || fragment.FragIndex >= fragment.FragCount {
		return nil, false
	}
	if fragment.DataLen > tlsFragmentSize || fragment.TotalLen == 0 {
		return nil, false
	}

	payload := make([]byte, fragment.DataLen)
	copy(payload, fragment.Data[:fragment.DataLen])

	a.mu.Lock()
	defer a.mu.Unlock()

	key := fragKey{TGID: fragment.TGID, TimestampNS: fragment.TimestampNS, Direction: fragment.Direction}
	buf := a.pending[key]
	if buf == nil {
		buf = &fragmentBuffer{
			fragments: make(map[uint16][]byte, fragment.FragCount),
			totalLen:  fragment.TotalLen,
			fragCount: fragment.FragCount,
			createdAt: time.Now(),
			libType:   fragment.LibType,
			comm:      sanitizeTLSComm(fragment.Comm),
			pid:       fragment.PID,
			tgid:      fragment.TGID,
		}
		a.pending[key] = buf
	}
	if buf.fragCount != fragment.FragCount || buf.totalLen != fragment.TotalLen {
		delete(a.pending, key)
		a.dropped++
		return nil, false
	}
	if _, exists := buf.fragments[fragment.FragIndex]; !exists {
		buf.fragments[fragment.FragIndex] = payload
		buf.received++
	}
	if buf.received != buf.fragCount {
		return nil, false
	}

	var out bytes.Buffer
	for i := uint16(0); i < buf.fragCount; i++ {
		part, ok := buf.fragments[i]
		if !ok {
			return nil, false
		}
		out.Write(part)
	}
	delete(a.pending, key)
	return &completedTLSFragment{
		TimestampNS: fragment.TimestampNS,
		PID:         buf.pid,
		TGID:        buf.tgid,
		Comm:        buf.comm,
		LibType:     buf.libType,
		Direction:   fragment.Direction,
		TotalLen:    buf.totalLen,
		Payload:     out.Bytes(),
	}, true
}

func (a *FragmentAssembler) CleanupExpired(now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	dropped := 0
	for key, buf := range a.pending {
		if now.Sub(buf.createdAt) > a.timeout {
			delete(a.pending, key)
			dropped++
		}
	}
	a.dropped += dropped
	return dropped
}

func (a *FragmentAssembler) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

func (a *FragmentAssembler) Dropped() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}

func sanitizeTLSComm(comm [16]byte) string {
	end := bytes.IndexByte(comm[:], 0)
	if end < 0 {
		end = len(comm)
	}
	return strings.TrimSpace(string(comm[:end]))
}
```

- [ ] **Step 5: Run assembler tests to verify pass**

Run:
```bash
cd backend && go test -run 'TestFragmentAssembler|TestCompletedFragment' ./...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/tls_capture_types.go backend/tls_fragment_assembler.go backend/tls_fragment_assembler_test.go
git commit -m "feat: assemble TLS plaintext fragments"
```

---

### Task 4: Add HTTP plaintext parser and redaction

**Files:**
- Create: `backend/tls_http_parser.go`
- Test: `backend/tls_http_parser_test.go`

- [ ] **Step 1: Write parser tests first**

Create `backend/tls_http_parser_test.go`:
```go
package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseTLSPlaintextHTTPRequestRedactsSensitiveHeaders(t *testing.T) {
	payload := []byte("POST /v1/messages HTTP/1.1\r\nHost: api.anthropic.com\r\nAuthorization: Bearer secret\r\nX-API-Key: key\r\nContent-Type: application/json\r\n\r\n{\"b\":2,\"a\":1}")
	event := parseTLSPlaintext(completedTLSFragment{TimestampNS: 1000, PID: 42, TGID: 42, Comm: "claude", LibType: tlsLibGo, Direction: tlsDirectionSend, Payload: payload})
	if event.Type != "tls_plaintext" || event.Direction != "send" || event.Lib != "Go" {
		t.Fatalf("unexpected metadata %#v", event)
	}
	if event.Method != "POST" || event.URL != "/v1/messages" || event.Host != "api.anthropic.com" {
		t.Fatalf("unexpected request fields %#v", event)
	}
	if event.Headers["authorization"] != "***REDACTED***" || event.Headers["x-api-key"] != "***REDACTED***" {
		t.Fatalf("headers were not redacted: %#v", event.Headers)
	}
	if !strings.Contains(event.Body, "\n") || !strings.Contains(event.Body, "\"a\": 1") {
		t.Fatalf("body was not pretty JSON: %q", event.Body)
	}
}

func TestParseTLSPlaintextHTTPResponse(t *testing.T) {
	payload := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nSet-Cookie: sid=secret\r\n\r\n{\"ok\":true}")
	event := parseTLSPlaintext(completedTLSFragment{TimestampNS: uint64(time.Second), PID: 7, TGID: 7, Comm: "curl", LibType: tlsLibOpenSSL, Direction: tlsDirectionRecv, Payload: payload})
	if event.StatusCode != 200 || event.Direction != "recv" || event.Lib != "OpenSSL" {
		t.Fatalf("unexpected response %#v", event)
	}
	if event.Headers["set-cookie"] != "***REDACTED***" {
		t.Fatalf("set-cookie was not redacted: %#v", event.Headers)
	}
	if event.ContentType != "application/json" || !strings.Contains(event.Body, "\"ok\": true") {
		t.Fatalf("unexpected body/content type %#v", event)
	}
}

func TestParseTLSPlaintextNonHTTPUsesHexDump(t *testing.T) {
	event := parseTLSPlaintext(completedTLSFragment{TimestampNS: 1, PID: 1, TGID: 1, Comm: "bin", LibType: tlsLibNSS, Direction: tlsDirectionSend, Payload: []byte{0, 1, 2, 255}})
	if event.RawAvailable || event.RawHexDump != "00 01 02 ff" || event.Body != "" {
		t.Fatalf("unexpected raw fallback %#v", event)
	}
}

func TestParseTLSPlaintextTruncatesLargeBody(t *testing.T) {
	payload := []byte("POST /upload HTTP/1.1\r\nHost: example.test\r\n\r\n" + strings.Repeat("a", tlsMaxBodySize+1))
	event := parseTLSPlaintext(completedTLSFragment{TimestampNS: 1, PID: 1, TGID: 1, Comm: "curl", LibType: tlsLibGnuTLS, Direction: tlsDirectionSend, Payload: payload})
	if !event.Truncated || len(event.Body) != tlsMaxBodySize {
		t.Fatalf("body len = %d truncated = %v", len(event.Body), event.Truncated)
	}
}
```

- [ ] **Step 2: Run parser tests to verify failure**

Run:
```bash
cd backend && go test -run 'TestParseTLSPlaintext' ./...
```
Expected: FAIL with undefined `parseTLSPlaintext` and `tlsMaxBodySize`.

- [ ] **Step 3: Add parser implementation**

Create `backend/tls_http_parser.go`:
```go
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tlsMaxBodySize = 16 * 1024

var sensitiveTLSHeaders = map[string]bool{
	"authorization": true,
	"x-api-key":     true,
	"cookie":        true,
	"set-cookie":    true,
}

func parseTLSPlaintext(fragment completedTLSFragment) TLSPlaintextEvent {
	event := TLSPlaintextEvent{
		Type:      "tls_plaintext",
		Timestamp: time.Unix(0, int64(fragment.TimestampNS)).UTC(),
		PID:       fragment.PID,
		TGID:      fragment.TGID,
		Comm:      fragment.Comm,
		Direction: tlsDirectionLabel(fragment.Direction),
		Lib:       tlsLibLabel(fragment.LibType),
		BodySize:  len(fragment.Payload),
	}
	if parseHTTPRequest(fragment.Payload, &event) || parseHTTPResponse(fragment.Payload, &event) {
		event.RawAvailable = false
		return event
	}
	event.RawHexDump = spacedHex(fragment.Payload)
	event.RawAvailable = false
	return event
}

func parseHTTPRequest(payload []byte, event *TLSPlaintextEvent) bool {
	reader := bufio.NewReader(bytes.NewReader(payload))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return false
	}
	body := readRemainingBody(reader)
	event.Method = req.Method
	event.URL = req.URL.String()
	event.Host = req.Host
	event.Headers = redactHTTPHeaders(req.Header)
	event.ContentType = req.Header.Get("Content-Type")
	event.Body, event.Truncated = formatTLSBody(body)
	return true
}

func parseHTTPResponse(payload []byte, event *TLSPlaintextEvent) bool {
	reader := bufio.NewReader(bytes.NewReader(payload))
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return false
	}
	body := readRemainingBody(reader)
	event.StatusCode = resp.StatusCode
	event.Headers = redactHTTPHeaders(resp.Header)
	event.ContentType = resp.Header.Get("Content-Type")
	event.Body, event.Truncated = formatTLSBody(body)
	return true
}

func readRemainingBody(reader *bufio.Reader) []byte {
	body, _ := reader.ReadBytes(0)
	if len(body) > 0 && body[len(body)-1] == 0 {
		body = body[:len(body)-1]
	}
	return body
}

func redactHTTPHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if sensitiveTLSHeaders[lower] {
			out[lower] = "***REDACTED***"
			continue
		}
		out[lower] = strings.Join(values, ", ")
	}
	return out
}

func formatTLSBody(body []byte) (string, bool) {
	truncated := false
	if len(body) > tlsMaxBodySize {
		body = body[:tlsMaxBodySize]
		truncated = true
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", truncated
	}
	var decoded any
	if json.Valid(trimmed) && json.Unmarshal(trimmed, &decoded) == nil {
		formatted, err := json.MarshalIndent(decoded, "", "  ")
		if err == nil {
			return string(formatted), truncated
		}
	}
	return string(body), truncated
}

func spacedHex(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	encoded := hex.EncodeToString(payload)
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		parts = append(parts, encoded[i:i+2])
	}
	return strings.Join(parts, " ")
}

func tlsDirectionLabel(direction uint8) string {
	if direction == tlsDirectionRecv {
		return "recv"
	}
	return "send"
}

func tlsLibLabel(lib uint8) string {
	switch lib {
	case tlsLibOpenSSL:
		return "OpenSSL"
	case tlsLibGo:
		return "Go"
	case tlsLibGnuTLS:
		return "GnuTLS"
	case tlsLibNSS:
		return "NSS"
	default:
		return "unknown-" + strconv.Itoa(int(lib))
	}
}
```

- [ ] **Step 4: Run parser tests to verify pass**

Run:
```bash
cd backend && go test -run 'TestParseTLSPlaintext' ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/tls_http_parser.go backend/tls_http_parser_test.go
git commit -m "feat: parse TLS plaintext HTTP payloads"
```

---

### Task 5: Add TLS event store and WebSocket/API handlers

**Files:**
- Create: `backend/tls_capture_store.go`
- Create: `backend/tls_capture_handlers.go`
- Test: `backend/tls_capture_store_test.go`
- Test: `backend/tls_capture_handlers_test.go`

- [ ] **Step 1: Write store tests first**

Create `backend/tls_capture_store_test.go`:
```go
package main

import (
	"testing"
	"time"
)

func TestTLSCaptureStoreKeepsRecentEvents(t *testing.T) {
	store := NewTLSCaptureStore(2)
	store.Add(TLSPlaintextEvent{PID: 1, Timestamp: time.Unix(1, 0)})
	store.Add(TLSPlaintextEvent{PID: 2, Timestamp: time.Unix(2, 0)})
	store.Add(TLSPlaintextEvent{PID: 3, Timestamp: time.Unix(3, 0)})
	recent := store.Recent(10)
	if len(recent) != 2 || recent[0].PID != 2 || recent[1].PID != 3 {
		t.Fatalf("recent = %#v", recent)
	}
}

func TestTLSCaptureStoreTracksLibraryStatus(t *testing.T) {
	store := NewTLSCaptureStore(10)
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/usr/lib/libssl.so.3", Attached: true})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so.30", Error: "missing symbol"})
	statuses := store.LibraryStatuses()
	if len(statuses) != 2 || !statuses[0].Attached || statuses[1].Error != "missing symbol" {
		t.Fatalf("statuses = %#v", statuses)
	}
}
```

- [ ] **Step 2: Write handler tests first**

Create `backend/tls_capture_handlers_test.go`:
```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandleTLSCaptureRecent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewTLSCaptureStore(10)
	store.Add(TLSPlaintextEvent{Type: "tls_plaintext", PID: 42, Comm: "curl", Timestamp: time.Unix(1, 0).UTC()})
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tls-capture/recent?limit=5", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct{ Events []TLSPlaintextEvent `json:"events"` }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Events) != 1 || resp.Events[0].PID != 42 {
		t.Fatalf("events = %#v", resp.Events)
	}
}

func TestHandleTLSCaptureGoBinaryRejectsMissingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tls-capture/go-binary", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 3: Run store/handler tests to verify failure**

Run:
```bash
cd backend && go test -run 'TestTLSCaptureStore|TestHandleTLSCapture' ./...
```
Expected: FAIL with undefined `NewTLSCaptureStore` and `registerTLSCaptureRoutes`.

- [ ] **Step 4: Add store implementation**

Create `backend/tls_capture_store.go`:
```go
package main

import "sync"

type TLSCaptureStore struct {
	mu        sync.RWMutex
	events    []TLSPlaintextEvent
	max       int
	libraries map[string]TLSLibraryStatus
}

func NewTLSCaptureStore(max int) *TLSCaptureStore {
	if max <= 0 {
		max = 1000
	}
	return &TLSCaptureStore{max: max, libraries: make(map[string]TLSLibraryStatus)}
}

func (s *TLSCaptureStore) Add(event TLSPlaintextEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if len(s.events) > s.max {
		copy(s.events, s.events[len(s.events)-s.max:])
		s.events = s.events[:s.max]
	}
}

func (s *TLSCaptureStore) Recent(limit int) []TLSPlaintextEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	out := make([]TLSPlaintextEvent, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out
}

func (s *TLSCaptureStore) SetLibraryStatus(status TLSLibraryStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.libraries[status.Name+"\x00"+status.Path] = status
}

func (s *TLSCaptureStore) LibraryStatuses() []TLSLibraryStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TLSLibraryStatus, 0, len(s.libraries))
	for _, status := range s.libraries {
		out = append(out, status)
	}
	return out
}

func (s *TLSCaptureStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}
```

- [ ] **Step 5: Add handlers and broadcast hub**

Create `backend/tls_capture_handlers.go`:
```go
package main

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type tlsCaptureBroadcaster struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

func newTLSCaptureBroadcaster() *tlsCaptureBroadcaster {
	return &tlsCaptureBroadcaster{clients: make(map[*websocket.Conn]bool)}
}

func (b *tlsCaptureBroadcaster) Serve(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	b.mu.Lock()
	b.clients[conn] = true
	b.mu.Unlock()
	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.clients, conn)
			b.mu.Unlock()
			_ = conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (b *tlsCaptureBroadcaster) Broadcast(event TLSPlaintextEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for conn := range b.clients {
		if err := conn.WriteJSON(event); err != nil {
			_ = conn.Close()
			delete(b.clients, conn)
		}
	}
}

type tlsGoBinaryRegistrar interface {
	AttachGoUprobes(binPath string, pid int) error
}

func registerTLSCaptureRoutes(router gin.IRouter, manager tlsGoBinaryRegistrar, store *TLSCaptureStore) {
	router.GET("/tls-capture/recent", handleTLSCaptureRecent(store))
	router.GET("/tls-capture/libraries", handleTLSCaptureLibraries(store))
	router.POST("/tls-capture/go-binary", handleTLSCaptureGoBinary(manager))
}

func handleTLSCaptureRecent(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 100
		if raw := c.Query("limit"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}
		c.JSON(http.StatusOK, gin.H{"events": store.Recent(limit)})
	}
}

func handleTLSCaptureLibraries(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"libraries": store.LibraryStatuses()})
	}
}

func handleTLSCaptureGoBinary(manager tlsGoBinaryRegistrar) gin.HandlerFunc {
	type request struct {
		Path string `json:"path"`
		PID  int    `json:"pid"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
			return
		}
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture manager is not available"})
			return
		}
		if err := manager.AttachGoUprobes(req.Path, req.PID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "attached"})
	}
}
```

- [ ] **Step 6: Run store/handler tests to verify pass**

Run:
```bash
cd backend && go test -run 'TestTLSCaptureStore|TestHandleTLSCapture' ./...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/tls_capture_store.go backend/tls_capture_handlers.go backend/tls_capture_store_test.go backend/tls_capture_handlers_test.go
git commit -m "feat: expose TLS capture API state"
```

---

### Task 6: Implement TLSProbeManager and uprobe attach logic

**Files:**
- Create: `backend/tls_probe_manager.go`
- Test: `backend/tls_probe_manager_test.go`
- Modify after bpf2go generation: generated `backend/ebpf/agenttlscapture_bpf*.go` imports are generated only by Task 2.

- [ ] **Step 1: Write symbol parsing and library discovery tests first**

Create `backend/tls_probe_manager_test.go`:
```go
package main

import "testing"

func TestFindFirstExistingPath(t *testing.T) {
	missing := t.TempDir() + "/missing.so"
	if got, ok := findFirstExistingPath([]string{missing}); ok || got != "" {
		t.Fatalf("got %q ok %v", got, ok)
	}
}

func TestTLSProgramForSymbol(t *testing.T) {
	if tlsProgramForSymbol("SSL_write") != "uprobe_ssl_write" {
		t.Fatalf("SSL_write program mismatch")
	}
	if tlsProgramForSymbol("crypto/tls.(*Conn).Read") != "uprobe_crypto_tls_conn_read" {
		t.Fatalf("Go Read program mismatch")
	}
	if tlsReturnProgramForSymbol("SSL_read") != "uretprobe_ssl_read" {
		t.Fatalf("SSL_read return program mismatch")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:
```bash
cd backend && go test -run 'TestFindFirstExistingPath|TestTLSProgramForSymbol' ./...
```
Expected: FAIL with undefined helper functions.

- [ ] **Step 3: Add TLSProbeManager implementation**

Create `backend/tls_probe_manager.go`:
```go
package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

var staticTLSLibraries = []struct {
	name    string
	paths   []string
	symbols []string
}{
	{name: "OpenSSL", paths: []string{"/usr/lib/libssl.so.3", "/usr/lib/libssl.so.1.1", "/usr/lib/x86_64-linux-gnu/libssl.so.3", "/usr/lib64/libssl.so.3"}, symbols: []string{"SSL_write", "SSL_read"}},
	{name: "GnuTLS", paths: []string{"/usr/lib/libgnutls.so.30", "/usr/lib/libgnutls.so", "/usr/lib/x86_64-linux-gnu/libgnutls.so.30"}, symbols: []string{"gnutls_record_send", "gnutls_record_recv"}},
	{name: "NSS", paths: []string{"/usr/lib/libssl3.so", "/usr/lib/libnspr4.so", "/usr/lib/x86_64-linux-gnu/libssl3.so", "/usr/lib/x86_64-linux-gnu/libnspr4.so"}, symbols: []string{"PR_Write", "PR_Read"}},
}

type TLSProbeManager struct {
	objs        bpf.AgentTlsCaptureObjects
	links       []link.Link
	assembler   *FragmentAssembler
	store       *TLSCaptureStore
	broadcaster *tlsCaptureBroadcaster
	mu          sync.Mutex
	closed      bool
}

func NewTLSProbeManager(store *TLSCaptureStore, broadcaster *tlsCaptureBroadcaster) (*TLSProbeManager, error) {
	if store == nil {
		store = NewTLSCaptureStore(1000)
	}
	if broadcaster == nil {
		broadcaster = newTLSCaptureBroadcaster()
	}
	manager := &TLSProbeManager{assembler: NewFragmentAssembler(5 * time.Second), store: store, broadcaster: broadcaster}
	if err := bpf.LoadAgentTlsCaptureObjects(&manager.objs, &ebpf.CollectionOptions{}); err != nil {
		return nil, fmt.Errorf("load TLS capture eBPF objects: %w", err)
	}
	return manager, nil
}

func (m *TLSProbeManager) AttachStaticLibs() error {
	var errs []error
	for _, libSpec := range staticTLSLibraries {
		path, ok := findFirstExistingPath(libSpec.paths)
		if !ok {
			m.store.SetLibraryStatus(TLSLibraryStatus{Name: libSpec.name, Attached: false, Error: "library not found"})
			continue
		}
		executable, err := link.OpenExecutable(path)
		if err != nil {
			m.store.SetLibraryStatus(TLSLibraryStatus{Name: libSpec.name, Path: path, Error: err.Error()})
			errs = append(errs, err)
			continue
		}
		attached := 0
		for _, symbol := range libSpec.symbols {
			program := m.programByName(tlsProgramForSymbol(symbol))
			if program != nil {
				lnk, err := executable.Uprobe(symbol, program, nil)
				if err == nil {
					m.links = append(m.links, lnk)
					attached++
				} else {
					errs = append(errs, err)
				}
			}
			retProgram := m.programByName(tlsReturnProgramForSymbol(symbol))
			if retProgram != nil {
				lnk, err := executable.Uretprobe(symbol, retProgram, nil)
				if err == nil {
					m.links = append(m.links, lnk)
					attached++
				} else {
					errs = append(errs, err)
				}
			}
		}
		m.store.SetLibraryStatus(TLSLibraryStatus{Name: libSpec.name, Path: path, Attached: attached > 0})
	}
	return errors.Join(errs...)
}

func (m *TLSProbeManager) AttachGoUprobes(binPath string, pid int) error {
	if binPath == "" {
		return fmt.Errorf("binary path is required")
	}
	if _, err := os.Stat(binPath); err != nil {
		return err
	}
	symbols, err := parseGoTLSSymbols(binPath)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return fmt.Errorf("Go TLS symbols not found in %s", binPath)
	}
	executable, err := link.OpenExecutable(binPath)
	if err != nil {
		return err
	}
	opts := &link.UprobeOptions{}
	if pid > 0 {
		opts.PID = pid
	}
	for symbol := range symbols {
		program := m.programByName(tlsProgramForSymbol(symbol))
		if program != nil {
			lnk, err := executable.Uprobe(symbol, program, opts)
			if err != nil {
				return err
			}
			m.links = append(m.links, lnk)
		}
		retProgram := m.programByName(tlsReturnProgramForSymbol(symbol))
		if retProgram != nil {
			lnk, err := executable.Uretprobe(symbol, retProgram, opts)
			if err != nil {
				return err
			}
			m.links = append(m.links, lnk)
		}
	}
	m.store.SetLibraryStatus(TLSLibraryStatus{Name: "Go", Path: binPath, Attached: true})
	return nil
}

func (m *TLSProbeManager) ReadLoop() {
	rd, err := ringbuf.NewReader(m.objs.TlsEvents)
	if err != nil {
		log.Printf("[TLS] ringbuf reader unavailable: %v", err)
		return
	}
	defer rd.Close()

	cleanup := time.NewTicker(10 * time.Second)
	defer cleanup.Stop()
	go func() {
		for range cleanup.C {
			m.assembler.CleanupExpired(time.Now())
		}
	}()

	var fragment tlsFragment
	for {
		record, err := rd.Read()
		if err != nil {
			return
		}
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &fragment); err != nil {
			log.Printf("[TLS] failed to decode TLS fragment: %v", err)
			continue
		}
		if completed, ok := m.assembler.Add(fragment); ok {
			event := parseTLSPlaintext(*completed)
			m.store.Add(event)
			m.broadcaster.Broadcast(event)
		}
	}
}

func (m *TLSProbeManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	for _, lnk := range m.links {
		_ = lnk.Close()
	}
	m.links = nil
	m.objs.Close()
}

func (m *TLSProbeManager) programByName(name string) *ebpf.Program {
	switch name {
	case "uprobe_ssl_write":
		return m.objs.UprobeSslWrite
	case "uprobe_ssl_read":
		return m.objs.UprobeSslRead
	case "uretprobe_ssl_read":
		return m.objs.UretprobeSslRead
	case "uprobe_gnutls_record_send":
		return m.objs.UprobeGnutlsRecordSend
	case "uprobe_gnutls_record_recv":
		return m.objs.UprobeGnutlsRecordRecv
	case "uretprobe_gnutls_record_recv":
		return m.objs.UretprobeGnutlsRecordRecv
	case "uprobe_pr_write":
		return m.objs.UprobePrWrite
	case "uprobe_pr_read":
		return m.objs.UprobePrRead
	case "uretprobe_pr_read":
		return m.objs.UretprobePrRead
	case "uprobe_crypto_tls_conn_write":
		return m.objs.UprobeCryptoTlsConnWrite
	case "uprobe_crypto_tls_conn_read":
		return m.objs.UprobeCryptoTlsConnRead
	case "uretprobe_crypto_tls_conn_read":
		return m.objs.UretprobeCryptoTlsConnRead
	default:
		return nil
	}
}

func findFirstExistingPath(paths []string) (string, bool) {
	for _, path := range paths {
		matches, _ := filepath.Glob(path)
		if len(matches) == 0 {
			matches = []string{path}
		}
		for _, match := range matches {
			if _, err := os.Stat(match); err == nil {
				return match, true
			}
		}
	}
	return "", false
}

func parseGoTLSSymbols(binPath string) (map[string]uint64, error) {
	file, err := elf.Open(binPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	syms, err := file.Symbols()
	if err != nil {
		return nil, err
	}
	out := make(map[string]uint64)
	for _, sym := range syms {
		if sym.Name == "crypto/tls.(*Conn).Write" || sym.Name == "crypto/tls.(*Conn).Read" {
			out[sym.Name] = sym.Value
		}
	}
	return out, nil
}

func tlsProgramForSymbol(symbol string) string {
	switch symbol {
	case "SSL_write":
		return "uprobe_ssl_write"
	case "SSL_read":
		return "uprobe_ssl_read"
	case "gnutls_record_send":
		return "uprobe_gnutls_record_send"
	case "gnutls_record_recv":
		return "uprobe_gnutls_record_recv"
	case "PR_Write":
		return "uprobe_pr_write"
	case "PR_Read":
		return "uprobe_pr_read"
	case "crypto/tls.(*Conn).Write":
		return "uprobe_crypto_tls_conn_write"
	case "crypto/tls.(*Conn).Read":
		return "uprobe_crypto_tls_conn_read"
	default:
		return ""
	}
}

func tlsReturnProgramForSymbol(symbol string) string {
	switch symbol {
	case "SSL_read":
		return "uretprobe_ssl_read"
	case "gnutls_record_recv":
		return "uretprobe_gnutls_record_recv"
	case "PR_Read":
		return "uretprobe_pr_read"
	case "crypto/tls.(*Conn).Read":
		return "uretprobe_crypto_tls_conn_read"
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run manager helper tests**

Run:
```bash
cd backend && go test -run 'TestFindFirstExistingPath|TestTLSProgramForSymbol' ./...
```
Expected: PASS.

- [ ] **Step 5: Build backend after generated object references**

Run:
```bash
cd backend && go build ./...
```
Expected: PASS. If generated field names differ from the switch cases, inspect `backend/ebpf/agenttlscapture_bpfel.go` generated struct fields and update `programByName()` to match exactly.

- [ ] **Step 6: Commit**

```bash
git add backend/tls_probe_manager.go backend/tls_probe_manager_test.go
git commit -m "feat: attach TLS capture uprobes"
```

---

### Task 7: Add automatic Go TLS process discovery

**Files:**
- Modify: `backend/tls_probe_manager.go`
- Test: `backend/tls_probe_manager_test.go`

- [ ] **Step 1: Add process discovery tests first**

Append to `backend/tls_probe_manager_test.go`:
```go
func TestParseProcPID(t *testing.T) {
	pid, ok := parseProcPID("/proc/1234/exe")
	if !ok || pid != 1234 {
		t.Fatalf("pid = %d ok = %v", pid, ok)
	}
	if pid, ok := parseProcPID("/proc/self/exe"); ok || pid != 0 {
		t.Fatalf("self parsed as pid = %d ok = %v", pid, ok)
	}
}

func TestShouldAttachGoBinaryOnlyOncePerPIDPath(t *testing.T) {
	manager := &TLSProbeManager{attachedGo: make(map[string]bool)}
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("first attach should be allowed")
	}
	if manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("duplicate attach should be skipped")
	}
	if !manager.shouldAttachGoBinary("/tmp/app", 43) {
		t.Fatalf("different pid should be allowed")
	}
}
```

- [ ] **Step 2: Run discovery tests to verify failure**

Run:
```bash
cd backend && go test -run 'TestParseProcPID|TestShouldAttachGoBinary' ./...
```
Expected: FAIL with undefined `parseProcPID`, `attachedGo`, or `shouldAttachGoBinary`.

- [ ] **Step 3: Extend TLSProbeManager state**

Modify `TLSProbeManager` in `backend/tls_probe_manager.go`:
```go
type TLSProbeManager struct {
	objs        bpf.AgentTlsCaptureObjects
	links       []link.Link
	assembler   *FragmentAssembler
	store       *TLSCaptureStore
	broadcaster *tlsCaptureBroadcaster
	mu          sync.Mutex
	closed      bool
	attachedGo  map[string]bool
}
```

Modify `NewTLSProbeManager` initialization:
```go
manager := &TLSProbeManager{
	assembler:   NewFragmentAssembler(5 * time.Second),
	store:       store,
	broadcaster: broadcaster,
	attachedGo:  make(map[string]bool),
}
```

- [ ] **Step 4: Add discovery helpers**

Append to `backend/tls_probe_manager.go`:
```go
func parseProcPID(path string) (int, bool) {
	parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] != "proc" {
			continue
		}
		pid, err := strconv.Atoi(parts[i+1])
		if err != nil || pid <= 0 {
			return 0, false
		}
		return pid, true
	}
	return 0, false
}

func (m *TLSProbeManager) shouldAttachGoBinary(binPath string, pid int) bool {
	key := fmt.Sprintf("%d\x00%s", pid, binPath)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attachedGo == nil {
		m.attachedGo = make(map[string]bool)
	}
	if m.attachedGo[key] {
		return false
	}
	m.attachedGo[key] = true
	return true
}
```

Add imports if missing:
```go
	"strconv"
	"strings"
```

- [ ] **Step 5: Add `/proc` scanner loop**

Append to `backend/tls_probe_manager.go`:
```go
func (m *TLSProbeManager) DiscoverGoProcesses() {
	entries, err := filepath.Glob("/proc/[0-9]*/exe")
	if err != nil {
		return
	}
	for _, exeLink := range entries {
		pid, ok := parseProcPID(exeLink)
		if !ok {
			continue
		}
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}
		if !m.shouldAttachGoBinary(binPath, pid) {
			continue
		}
		if err := m.AttachGoUprobes(binPath, pid); err != nil {
			m.store.SetLibraryStatus(TLSLibraryStatus{Name: "Go", Path: binPath, Attached: false, Error: err.Error()})
		}
	}
}

func (m *TLSProbeManager) StartGoDiscoveryLoop(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		m.DiscoverGoProcesses()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.mu.Lock()
			closed := m.closed
			m.mu.Unlock()
			if closed {
				return
			}
			m.DiscoverGoProcesses()
		}
	}()
}
```

- [ ] **Step 6: Run discovery tests**

Run:
```bash
cd backend && go test -run 'TestParseProcPID|TestShouldAttachGoBinary' ./...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/tls_probe_manager.go backend/tls_probe_manager_test.go
git commit -m "feat: discover Go TLS processes"
```

---

### Task 8: Wire TLS manager into backend runtime

**Files:**
- Modify: `backend/main.go:114-205`, `backend/main.go:241-253`
- Test: `cd backend && go test ./...`

- [ ] **Step 1: Add TLS manager startup in `main.go`**

After `ensureTrackerMapsLoaded()` and the existing syscall ringbuf setup, add:
```go
tlsStore := NewTLSCaptureStore(2000)
tlsBroadcaster := newTLSCaptureBroadcaster()
tlsManager, err := NewTLSProbeManager(tlsStore, tlsBroadcaster)
if err != nil {
	log.Printf("[TLS] capture disabled: %v", err)
} else {
	defer tlsManager.Close()
	if err := tlsManager.AttachStaticLibs(); err != nil {
		log.Printf("[TLS] static library attach completed with warnings: %v", err)
	}
	tlsManager.StartGoDiscoveryLoop(time.Minute)
	go tlsManager.ReadLoop()
}
```

- [ ] **Step 2: Add WebSocket route in `main.go`**

Near existing WebSocket routes:
```go
r.GET("/ws/tls-capture", authMiddleware(), func(c *gin.Context) {
	tlsBroadcaster.Serve(c)
})
```

- [ ] **Step 3: Add TLS API group in `main.go`**

Inside authenticated `api := r.Group("/", authMiddleware())` block:
```go
registerTLSCaptureRoutes(api, tlsManager, tlsStore)
```

- [ ] **Step 4: Run backend tests**

Run:
```bash
cd backend && go test ./...
```
Expected: PASS.

- [ ] **Step 5: Run backend race tests via Makefile**

Run:
```bash
make test
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/main.go
git commit -m "feat: wire TLS capture backend routes"
```

---

### Task 9: Add Vue TLS capture page

**Files:**
- Create: `frontend/src/views/TLSCapture.vue`
- Modify: `frontend/src/router/index.ts:28-33`
- Modify: `frontend/src/App.vue:1-79`
- Test: `cd frontend && bun run build`

- [ ] **Step 1: Add route**

Modify `frontend/src/router/index.ts` and insert before `/execution-graph`:
```ts
  {
    path: '/tls-capture',
    name: 'TLSCapture',
    component: () => import('../views/TLSCapture.vue'),
  },
```

- [ ] **Step 2: Add menu item and selected state**

Modify `frontend/src/App.vue` imports:
```ts
import { DashboardOutlined, SettingOutlined, BarChartOutlined, FolderOpenOutlined, PlaySquareOutlined, LinkOutlined, GlobalOutlined, DeploymentUnitOutlined, ClusterOutlined, SafetyCertificateOutlined } from '@ant-design/icons-vue';
```

Modify route watcher before `/execution-graph`:
```ts
  } else if (path.startsWith('/tls-capture')) {
    selectedKeys.value = ['/tls-capture'];
```

Insert menu item after Traffic:
```vue
        <a-menu-item key="/tls-capture">
          <template #icon><SafetyCertificateOutlined /></template>
          TLS 捕获
        </a-menu-item>
```

- [ ] **Step 3: Create TLSCapture view**

Create `frontend/src/views/TLSCapture.vue`:
```vue
<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import axios from 'axios';
import { CopyOutlined, PauseOutlined, PlayCircleOutlined, SafetyCertificateOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { buildWebSocketUrl } from '../utils/requestContext';

interface TLSPlaintextEvent {
  type: string;
  timestamp: string;
  pid: number;
  tgid: number;
  comm: string;
  direction: 'send' | 'recv' | string;
  lib: string;
  method?: string;
  url?: string;
  host?: string;
  status?: number;
  headers?: Record<string, string>;
  body?: string;
  body_size: number;
  content_type?: string;
  raw_hex_dump?: string;
  raw_available: boolean;
  truncated: boolean;
}

interface TLSLibraryStatus {
  name: string;
  path: string;
  attached: boolean;
  error?: string;
}

const events = ref<TLSPlaintextEvent[]>([]);
const libraries = ref<TLSLibraryStatus[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const autoScroll = ref(true);
const searchQuery = ref('');
const commFilter = ref('');
const hostFilter = ref('');
const selectedLibs = ref<string[]>([]);
const selectedDirections = ref<string[]>([]);
const expandedKeys = ref<Set<string>>(new Set());
const listRef = ref<HTMLElement | null>(null);

let ws: WebSocket | null = null;
let reconnectTimer: number | undefined;
let shouldReconnect = true;

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
};

const eventKey = (event: TLSPlaintextEvent, index: number) => `${event.timestamp}-${event.pid}-${index}`;
const directionColor = (direction: string) => direction === 'send' ? 'green' : 'blue';
const directionLabel = (direction: string) => direction === 'send' ? 'Send' : 'Recv';

const filteredEvents = computed(() => {
  let list = events.value;
  const query = searchQuery.value.trim().toLowerCase();
  if (query) {
    list = list.filter(event =>
      (event.body || '').toLowerCase().includes(query) ||
      (event.url || '').toLowerCase().includes(query) ||
      JSON.stringify(event.headers || {}).toLowerCase().includes(query)
    );
  }
  if (commFilter.value.trim()) {
    const comm = commFilter.value.trim().toLowerCase();
    list = list.filter(event => event.comm.toLowerCase().includes(comm));
  }
  if (hostFilter.value.trim()) {
    const host = hostFilter.value.trim().toLowerCase();
    list = list.filter(event => (event.host || '').toLowerCase().includes(host));
  }
  if (selectedLibs.value.length) {
    list = list.filter(event => selectedLibs.value.includes(event.lib));
  }
  if (selectedDirections.value.length) {
    list = list.filter(event => selectedDirections.value.includes(event.direction));
  }
  return list;
});

const libOptions = computed(() => Array.from(new Set(events.value.map(event => event.lib).filter(Boolean))).map(lib => ({ label: lib, value: lib })));
const directionOptions = [{ label: 'Send', value: 'send' }, { label: 'Recv', value: 'recv' }];
const totalBodyBytes = computed(() => filteredEvents.value.reduce((sum, event) => sum + Number(event.body_size || 0), 0));
const processStats = computed(() => {
  const counts = new Map<string, number>();
  filteredEvents.value.forEach(event => counts.set(event.comm || 'unknown', (counts.get(event.comm || 'unknown') || 0) + 1));
  return Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 8);
});

const fetchRecent = async () => {
  const { data } = await axios.get('/tls-capture/recent?limit=200');
  events.value = data.events || [];
};

const fetchLibraries = async () => {
  const { data } = await axios.get('/tls-capture/libraries');
  libraries.value = data.libraries || [];
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();
  const socket = new WebSocket(buildWebSocketUrl('/ws/tls-capture'));
  ws = socket;
  socket.onopen = () => { isConnected.value = true; };
  socket.onmessage = async (event) => {
    if (isPaused.value) return;
    try {
      const parsed = JSON.parse(event.data) as TLSPlaintextEvent;
      events.value = [parsed, ...events.value].slice(0, 1000);
      if (autoScroll.value) {
        await nextTick();
        listRef.value?.scrollTo({ top: 0, behavior: 'smooth' });
      }
    } catch (err) {
      console.error('TLSCapture: failed to parse event', err);
    }
  };
  socket.onclose = () => {
    isConnected.value = false;
    if (shouldReconnect) reconnectTimer = window.setTimeout(connectWebSocket, 3000);
  };
};

const toggleExpanded = (key: string) => {
  const next = new Set(expandedKeys.value);
  if (next.has(key)) next.delete(key); else next.add(key);
  expandedKeys.value = next;
};

const copyText = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} 已复制`);
};

const buildCurl = (event: TLSPlaintextEvent) => {
  const target = event.host && event.url?.startsWith('/') ? `https://${event.host}${event.url}` : (event.url || 'https://example.invalid');
  const parts = ['curl', '-X', event.method || 'GET'];
  Object.entries(event.headers || {}).forEach(([key, value]) => {
    if (value !== '***REDACTED***') parts.push('-H', `${key}: ${value}`);
  });
  if (event.body) parts.push('--data', event.body);
  parts.push(target);
  return parts.map(part => `'${part.replaceAll("'", "'\\''")}'`).join(' ');
};

onMounted(() => {
  fetchRecent();
  fetchLibraries();
  connectWebSocket();
});

onUnmounted(() => {
  shouldReconnect = false;
  if (reconnectTimer) window.clearTimeout(reconnectTimer);
  if (ws) ws.close();
});
</script>

<template>
  <div class="tls-capture-page">
    <a-row :gutter="16">
      <a-col :xs="24" :xl="18">
        <a-card :bordered="false">
          <template #title><span><SafetyCertificateOutlined /> TLS 明文日志</span></template>
          <template #extra>
            <a-space>
              <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Live' : 'Offline'" />
              <a-switch v-model:checked="autoScroll" checked-children="自动滚动" un-checked-children="手动" />
              <a-button size="small" :type="isPaused ? 'primary' : 'default'" @click="isPaused = !isPaused">
                <template #icon><PauseOutlined v-if="isPaused" /><PlayCircleOutlined v-else /></template>
                {{ isPaused ? '继续' : '暂停' }}
              </a-button>
            </a-space>
          </template>

          <a-space class="filters" wrap>
            <a-input v-model:value="searchQuery" allow-clear placeholder="搜索 body / URL / headers" style="width: 260px" />
            <a-input v-model:value="commFilter" allow-clear placeholder="进程名" style="width: 160px" />
            <a-input v-model:value="hostFilter" allow-clear placeholder="域名" style="width: 220px" />
            <a-select v-model:value="selectedLibs" mode="multiple" allow-clear placeholder="库类型" :options="libOptions" style="width: 220px" />
            <a-select v-model:value="selectedDirections" mode="multiple" allow-clear placeholder="方向" :options="directionOptions" style="width: 180px" />
          </a-space>

          <div ref="listRef" class="event-list">
            <a-empty v-if="filteredEvents.length === 0" description="暂无 TLS 明文事件" />
            <a-card v-for="(event, index) in filteredEvents" :key="eventKey(event, index)" size="small" class="event-card">
              <div class="event-header">
                <a-space wrap>
                  <span>{{ new Date(event.timestamp).toLocaleTimeString() }}</span>
                  <a-tag>{{ event.comm || 'unknown' }}</a-tag>
                  <a-tag :color="directionColor(event.direction)">{{ directionLabel(event.direction) }}</a-tag>
                  <a-tag color="purple">{{ event.lib }}</a-tag>
                  <a-tag>{{ formatBytes(event.body_size) }}</a-tag>
                  <a-tag v-if="event.truncated" color="orange">truncated</a-tag>
                </a-space>
                <a-space>
                  <a-button size="small" @click="toggleExpanded(eventKey(event, index))">{{ expandedKeys.has(eventKey(event, index)) ? '收起' : '展开' }}</a-button>
                  <a-button v-if="event.direction === 'send'" size="small" @click="copyText(buildCurl(event), 'curl')">复制 curl</a-button>
                  <a-button size="small" @click="copyText(event.body || event.raw_hex_dump || '', 'body')"><template #icon><CopyOutlined /></template>复制 body</a-button>
                </a-space>
              </div>
              <div class="event-summary">
                <template v-if="event.direction === 'send'">
                  {{ event.method || 'RAW' }} {{ event.url || '' }} → {{ event.host || 'unknown host' }}
                </template>
                <template v-else>
                  {{ event.status || 'RAW' }} ← {{ event.host || event.comm || 'unknown source' }}
                </template>
              </div>
              <pre class="body-preview">{{ expandedKeys.has(eventKey(event, index)) ? (event.body || event.raw_hex_dump) : (event.body || event.raw_hex_dump || '').slice(0, 500) }}</pre>
            </a-card>
          </div>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="6">
        <a-card title="统计" :bordered="false" class="side-card">
          <a-statistic title="当前过滤事件" :value="filteredEvents.length" />
          <a-statistic title="Body 总量" :value="formatBytes(totalBodyBytes)" />
          <a-divider />
          <a-typography-title :level="5">进程分布</a-typography-title>
          <div v-for="[comm, count] in processStats" :key="comm" class="stat-row">
            <span>{{ comm }}</span><a-tag>{{ count }}</a-tag>
          </div>
        </a-card>

        <a-card title="库 attach 状态" :bordered="false" class="side-card">
          <a-list :data-source="libraries" size="small">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-list-item-meta :title="item.name" :description="item.path || item.error || 'not found'" />
                <a-tag :color="item.attached ? 'green' : 'orange'">{{ item.attached ? 'attached' : 'skipped' }}</a-tag>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.tls-capture-page { min-width: 0; }
.filters { margin-bottom: 16px; }
.event-list { max-height: calc(100vh - 250px); overflow: auto; padding-right: 4px; }
.event-card { margin-bottom: 12px; }
.event-header { display: flex; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.event-summary { margin-top: 8px; color: rgba(0, 0, 0, 0.72); }
.body-preview { margin: 8px 0 0; padding: 12px; border-radius: 6px; background: #0f172a; color: #dbeafe; white-space: pre-wrap; word-break: break-word; max-height: 480px; overflow: auto; }
.side-card { margin-bottom: 16px; }
.stat-row { display: flex; justify-content: space-between; align-items: center; margin: 8px 0; }
</style>
```

- [ ] **Step 4: Run frontend typecheck/build**

Run:
```bash
cd frontend && bun run build
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/TLSCapture.vue frontend/src/router/index.ts frontend/src/App.vue
git commit -m "feat: add TLS capture dashboard"
```

---

### Task 10: Document runtime behavior and security constraints

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture.md`
- Modify: `backend/README.md`
- Modify: `frontend/README.md`

- [ ] **Step 1: Update README feature summary**

Add a concise section describing:
```markdown
### TLS 明文捕获

后端可以通过 eBPF uprobes 挂载 OpenSSL/GnuTLS/NSS 和手动注册的 Go TLS 二进制，在加密发送前或解密接收后捕获 HTTPS 明文片段。片段在 Go 后端拼装后解析 HTTP request/response，并通过 `/ws/tls-capture`、`/tls-capture/recent`、`/tls-capture/libraries` 暴露给前端。

安全边界：该功能不做 MITM、不注入证书、不修改目标进程内存或控制流；Authorization、X-API-KEY、Cookie、Set-Cookie 会在后端脱敏；body 默认只保留 16 KiB 预览。
```

- [ ] **Step 2: Update architecture data flow**

In `docs/architecture.md`, add TLS flow:
```markdown
eBPF uprobes → tls_events ringbuf → TLSProbeManager → FragmentAssembler → HTTP parser → TLSCaptureStore → /ws/tls-capture → Vue TLSCapture
```

- [ ] **Step 3: Update backend README**

Document backend endpoints:
```markdown
- `GET /ws/tls-capture` — JSON WebSocket stream of `tls_plaintext` events.
- `GET /tls-capture/recent?limit=100` — recent in-memory TLS plaintext events.
- `GET /tls-capture/libraries` — current library attach status.
- `POST /tls-capture/go-binary` — manually attach Go TLS uprobes for `{ "path": "/path/to/bin", "pid": 123 }`.
```

- [ ] **Step 4: Update frontend README**

Document the new route:
```markdown
- `/tls-capture` — TLS 明文日志，支持实时 WebSocket、进程/库/方向/域名过滤、body 搜索、复制 body 和 request curl。
```

- [ ] **Step 5: Commit**

```bash
git add README.md docs/architecture.md backend/README.md frontend/README.md
git commit -m "docs: describe TLS plaintext capture"
```

---

### Task 11: Run full verification and browser smoke test

**Files:**
- No source edits unless verification finds defects.

- [ ] **Step 1: Regenerate all generated artifacts**

Run:
```bash
cd backend/ebpf && go generate && go generate gen_tls.go
```
Expected: PASS.

- [ ] **Step 2: Run backend tests**

Run:
```bash
make test
```
Expected: PASS.

- [ ] **Step 3: Build all components**

Run:
```bash
make build
```
Expected: PASS.

- [ ] **Step 4: Start frontend dev server for UI smoke test**

Run:
```bash
make run-frontend
```
Expected: Vite serves the app. Keep the process running for the browser check.

- [ ] **Step 5: Browser smoke test**

Open the Vite URL in Chrome and verify:
- Top navigation contains `TLS 捕获`.
- Navigating to `/tls-capture` renders the TLS page.
- Filters are visible: search, process, host, library, direction.
- With backend unavailable or no TLS events, page shows an empty state and does not throw console errors.
- If the backend is running with auth token configured, `/tls-capture/libraries` populates library attach statuses.

- [ ] **Step 6: Stop dev server**

Stop the Vite process that was started in Step 4.

- [ ] **Step 7: Request code review**

Invoke `superpowers:requesting-code-review` and ask for review of:
- eBPF verifier safety and map sizing.
- Go ringbuf lifetime and handler safety.
- TLS data redaction and memory bounds.
- Vue rendering and WebSocket cleanup.

---

## Self-Review

- Spec coverage: eBPF source/generation, Go fragment assembler, HTTP parser, WebSocket/API, library status, Go binary manual attach, automatic `/proc` Go process discovery, frontend view, route/menu, docs, and verification are all mapped to tasks.
- Scope note: this is a full-plan implementation. Go auto-discovery is included after the manual attach path so it can reuse the same tested attach method and key-based duplicate suppression.
- Security: sensitive headers are redacted in Go before storage/broadcast, body is capped to 16 KiB, endpoints are registered inside the existing authenticated group or route middleware.
- Placeholder scan: no unresolved placeholder tokens are intentionally left for implementers.
- Type consistency: backend uses `TLSPlaintextEvent`, `TLSCaptureStore`, `TLSProbeManager`, `FragmentAssembler`; frontend consumes the same JSON field names emitted by Go tags.
