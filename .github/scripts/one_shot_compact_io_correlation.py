from pathlib import Path

def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:320]!r}")
    return text.replace(old, new, 1)

p = Path('backend/ebpf/agent_tracker_common.h')
c = p.read_text()

c = replace_once(c,
'''    u64 socket_fd_update_failures;
    u64 socket_parent_update_failures;
};

_Static_assert(sizeof(struct context_pressure_stats) == 48, "context pressure stats ABI changed");
''',
'''    u64 socket_fd_update_failures;
    u64 socket_parent_update_failures;
    u64 exit_io_update_failures;
};

_Static_assert(sizeof(struct context_pressure_stats) == 56, "context pressure stats ABI changed");
''', 'context pressure IO counter')

c = replace_once(c,
'''#define CONTEXT_PRESSURE_SOCKET_FD     5
#define CONTEXT_PRESSURE_SOCKET_PARENT 6
''',
'''#define CONTEXT_PRESSURE_SOCKET_FD     5
#define CONTEXT_PRESSURE_SOCKET_PARENT 6
#define CONTEXT_PRESSURE_EXIT_IO       7
''', 'context pressure IO kind')

c = replace_once(c,
'''    else if (kind == CONTEXT_PRESSURE_SOCKET_FD) stats->socket_fd_update_failures++;
    else if (kind == CONTEXT_PRESSURE_SOCKET_PARENT) stats->socket_parent_update_failures++;
}
''',
'''    else if (kind == CONTEXT_PRESSURE_SOCKET_FD) stats->socket_fd_update_failures++;
    else if (kind == CONTEXT_PRESSURE_SOCKET_PARENT) stats->socket_parent_update_failures++;
    else if (kind == CONTEXT_PRESSURE_EXIT_IO) stats->exit_io_update_failures++;
}
''', 'context pressure IO accounting')

c = replace_once(c,
'''struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, u64);
    __type(value, struct exit_meta);
} exit_ctx SEC(".maps");

// Most filesystem/process/descriptor correlation needs only scalar metadata.
''',
'''_Static_assert(sizeof(struct exit_meta) == 88, "full exit context ABI changed");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 6144);
    __type(key, u64);
    __type(value, struct exit_meta);
} exit_ctx SEC(".maps");

// High-frequency read/write correlation needs socket provenance and L7 capture
// state, but not timing/direction/byte fields from the full 88-byte context.
struct exit_io_meta {
    u32 type;
    u32 tag_id;
    u32 extra1;
    u32 extra2;
    u64 extra3;
    u64 addr_ptr;
    u32 net_family;
    u32 net_port;
    char net_addr[16];
    u32 capture_flags;
    u32 socket_type;
};

_Static_assert(sizeof(struct exit_io_meta) == 64, "I/O exit context ABI changed");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, u64);
    __type(value, struct exit_io_meta);
} exit_io_ctx SEC(".maps");

// Most filesystem/process/descriptor correlation needs only scalar metadata.
''', 'split full and IO exit maps')

c = replace_once(c,
'''static __always_inline int store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    long rc = bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_EXIT_FULL);
    return rc == 0;
}

static __always_inline int store_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
''',
'''static __always_inline int store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    long rc = bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_EXIT_FULL);
    return rc == 0;
}

static __always_inline int store_exit_io_meta(u64 pid_tgid, struct exit_io_meta *meta) {
    long rc = bpf_map_update_elem(&exit_io_ctx, &pid_tgid, meta, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_EXIT_IO);
    return rc == 0;
}

static __always_inline int store_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
''', 'store IO context')

anchor = '''static __always_inline void fill_network_meta_from_socket(struct exit_meta *meta, struct socket_fd_meta *socket, u32 bytes) {
    fill_network_meta_from_socket_direction(meta, socket, bytes, NET_DIR_OUTGOING);
}
'''
insert = anchor + '''
static __always_inline void fill_io_meta_from_socket(struct exit_io_meta *meta, struct socket_fd_meta *socket) {
    if (!meta || !socket) return;
    meta->net_family = socket->family;
    meta->net_port = socket->remote_port;
    meta->capture_flags |= socket->provenance_flags;
    meta->socket_type = socket->sock_type;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));
}
'''
c = replace_once(c, anchor, insert, 'fill IO socket provenance')

anchor = '''// Convenience inline for sys_exit handlers that only need pid_tgid correlation
// Returns 0 if no context was found (not a tracked syscall)
static __always_inline u32 consume_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
'''
insert = '''static __always_inline u32 consume_exit_io_meta(u64 pid_tgid, struct exit_io_meta *meta) {
    struct exit_io_meta *m = bpf_map_lookup_elem(&exit_io_ctx, &pid_tgid);
    if (!m) return 0;
    __builtin_memcpy(meta, m, sizeof(*meta));
    bpf_map_delete_elem(&exit_io_ctx, &pid_tgid);
    return meta->tag_id;
}

static __always_inline void fill_from_exit_io_meta(struct event *e, u64 pid_tgid,
                                                   struct exit_io_meta *meta,
                                                   u32 direction, u32 net_bytes) {
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    fill_base_info(e, (u32)(pid_tgid >> 32), meta->tag_id, comm);
    e->type = meta->type;
    e->retval = 0;
    e->duration_ns = 0;
    e->extra1 = meta->extra1;
    e->extra2 = meta->extra2;
    e->extra3 = meta->extra3;
    e->net_family = meta->net_family;
    e->net_direction = meta->socket_type ? direction : 0;
    e->net_bytes = net_bytes;
    e->net_port = meta->net_port;
    e->kernel_capture_flags = meta->capture_flags;
    e->kernel_capture_reserved = meta->socket_type;
    __builtin_memcpy(e->net_addr, meta->net_addr, sizeof(meta->net_addr));
}

// Convenience inline for sys_exit handlers that only need pid_tgid correlation
// Returns 0 if no context was found (not a tracked syscall)
static __always_inline u32 consume_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
'''
c = replace_once(c, anchor, insert, 'consume/fill IO context')
p.write_text(c)

p = Path('backend/ebpf/agent_tracker_syscalls.h')
s = p.read_text()

def handler_block(src: str, name: str, side: str):
    marker = f'int tracepoint__syscalls__sys_{side}_{name}('
    start = src.find(marker)
    if start < 0:
        raise SystemExit(f'missing {side} handler {name}')
    end = src.find('\n}\n', start)
    if end < 0:
        raise SystemExit(f'missing end for {side} handler {name}')
    end += 3
    return start, end, src[start:end]

def replace_handler(src: str, name: str, side: str, fn):
    start, end, block = handler_block(src, name, side)
    block = fn(block)
    return src[:start] + block + src[end:]

def enter_io(block: str, name: str):
    block = replace_once(block, 'struct exit_meta meta =', 'struct exit_io_meta meta =', f'{name} enter IO struct')
    block = block.replace('fill_network_meta_from_socket_direction(&meta, socket, (u32)ctx->args[2], NET_DIR_INCOMING);',
                          'fill_io_meta_from_socket(&meta, socket);')
    block = block.replace('fill_network_meta_from_socket(&meta, socket, requested);',
                          'fill_io_meta_from_socket(&meta, socket);')
    block = block.replace('fill_network_meta_from_socket(&meta, socket, first_len);',
                          'fill_io_meta_from_socket(&meta, socket);')
    block = block.replace('fill_network_meta_from_socket_direction(&meta, socket, 0, NET_DIR_INCOMING);',
                          'fill_io_meta_from_socket(&meta, socket);')
    block = replace_once(block, 'store_exit_meta(pid_tgid, &meta)', 'store_exit_io_meta(pid_tgid, &meta)', f'{name} enter IO store')
    return block

for name in ('read','write','writev','readv'):
    s = replace_handler(s, name, 'enter', lambda b, n=name: enter_io(b,n))

def patch_readv_enter(block: str):
    old = 'struct exit_io_meta meta = {.type = TYPE_READ, .tag_id = tag_id, .extra1 = (u32)fd, .addr_ptr = ctx->args[1], .capture_reserved = (u32)ctx->args[2]};'
    new = 'struct exit_io_meta meta = {.type = TYPE_READ, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = (u32)ctx->args[2], .addr_ptr = ctx->args[1]};'
    return replace_once(block, old, new, 'readv temporary iovcnt')
s = replace_handler(s, 'readv', 'enter', patch_readv_enter)

def patch_read_exit(block: str):
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_io_meta meta = {};', 'read exit IO struct')
    block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_io_meta(pid_tgid, &meta)', 'read exit IO consume')
    old = '''    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
'''
    new = '''    u32 net_bytes = meta.socket_type ? (u32)meta.extra3 : 0;
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
'''
    block = replace_once(block, old, new, 'read net bytes derivation')
    block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);',
                         'fill_from_exit_io_meta(e, pid_tgid, &meta, NET_DIR_INCOMING, net_bytes);',
                         'read IO fill')
    return block
s = replace_handler(s, 'read', 'exit', patch_read_exit)

def patch_write_exit(block: str):
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_io_meta meta = {};', 'write exit IO struct')
    block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_io_meta(pid_tgid, &meta)', 'write exit IO consume')
    block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);',
                         'fill_from_exit_io_meta(e, pid_tgid, &meta, NET_DIR_OUTGOING, meta.socket_type ? (u32)meta.extra3 : 0);',
                         'write IO fill')
    return block
s = replace_handler(s, 'write', 'exit', patch_write_exit)

def patch_writev_exit(block: str):
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_io_meta meta = {};', 'writev exit IO struct')
    block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_io_meta(pid_tgid, &meta)', 'writev exit IO consume')
    old = '''    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
'''
    new = '''    u32 net_bytes = meta.socket_type ? (u32)meta.extra3 : 0;
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
'''
    block = replace_once(block, old, new, 'writev net bytes derivation')
    block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);',
                         'fill_from_exit_io_meta(e, pid_tgid, &meta, NET_DIR_OUTGOING, net_bytes);',
                         'writev IO fill')
    return block
s = replace_handler(s, 'writev', 'exit', patch_writev_exit)

def patch_readv_exit(block: str):
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_io_meta meta = {};', 'readv exit IO struct')
    block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_io_meta(pid_tgid, &meta)', 'readv exit IO consume')
    block = replace_once(block, 'meta.addr_ptr != 0 && meta.capture_reserved > 0)',
                         'meta.addr_ptr != 0 && meta.extra3 > 0)', 'readv iovcnt condition')
    block = replace_once(block, 'capture_first_iovec((const void *)meta.addr_ptr, meta.capture_reserved, &iov)',
                         'capture_first_iovec((const void *)meta.addr_ptr, meta.extra3, &iov)', 'readv iovcnt consume')
    old = '''    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
'''
    new = '''    u32 net_bytes = 0;
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        net_bytes = (u32)ctx->ret;
    } else {
        meta.extra3 = 0;
    }
    struct event *e = reserve_event();
'''
    block = replace_once(block, old, new, 'readv restore event extra3')
    block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);',
                         'fill_from_exit_io_meta(e, pid_tgid, &meta, NET_DIR_INCOMING, net_bytes);',
                         'readv IO fill')
    return block
s = replace_handler(s, 'readv', 'exit', patch_readv_exit)
p.write_text(s)

p = Path('backend/core/types.go')
x = p.read_text()
x = replace_once(x, '''	ExitCtx              *ebpf.Map
	ExitCompactCtx       *ebpf.Map
''', '''	ExitCtx              *ebpf.Map
	ExitIoCtx            *ebpf.Map
	ExitCompactCtx       *ebpf.Map
''', 'core tracker IO map')
p.write_text(x)

p = Path('backend/app/runtime_ebpf.go')
x = p.read_text()
x = replace_once(x, '"tracking_mode", "exit_ctx", "exit_compact_ctx",',
                 '"tracking_mode", "exit_ctx", "exit_io_ctx", "exit_compact_ctx",', 'mapNames IO map')
x = replace_once(x,
'''		"tracked_prefixes": objs.TrackedPrefixes, "tracking_mode": objs.TrackingMode, "exit_ctx": objs.ExitCtx, "exit_compact_ctx": objs.ExitCompactCtx,
''',
'''		"tracked_prefixes": objs.TrackedPrefixes, "tracking_mode": objs.TrackingMode, "exit_ctx": objs.ExitCtx, "exit_io_ctx": objs.ExitIoCtx, "exit_compact_ctx": objs.ExitCompactCtx,
''', 'pin IO map')
x = replace_once(x, '''		ExitCtx:              maps["exit_ctx"],
		ExitCompactCtx:       maps["exit_compact_ctx"],
''', '''		ExitCtx:              maps["exit_ctx"],
		ExitIoCtx:            maps["exit_io_ctx"],
		ExitCompactCtx:       maps["exit_compact_ctx"],
''', 'load IO map')
x = replace_once(x, '''		&set.ExitCtx, &set.ExitCompactCtx, &set.ExitSinglePathBuf, &set.ExitPathBuf,
''', '''		&set.ExitCtx, &set.ExitIoCtx, &set.ExitCompactCtx, &set.ExitSinglePathBuf, &set.ExitPathBuf,
''', 'close IO map')
p.write_text(x)

p = Path('backend/app/observabilitybridge.go')
x = p.read_text()
x = replace_once(x, '''		"exit_ctx":             trackerMaps.ExitCtx,
		"exit_compact_ctx":     trackerMaps.ExitCompactCtx,
''', '''		"exit_ctx":             trackerMaps.ExitCtx,
		"exit_io_ctx":          trackerMaps.ExitIoCtx,
		"exit_compact_ctx":     trackerMaps.ExitCompactCtx,
''', 'observe IO map')
p.write_text(x)

p = Path('backend/app/observability/metrics_collector.go')
x = p.read_text()
x = replace_once(x, '''	SocketFDUpdateFailures     uint64
	SocketParentUpdateFailures uint64
}
''', '''	SocketFDUpdateFailures     uint64
	SocketParentUpdateFailures uint64
	ExitIOUpdateFailures       uint64
}
''', 'Go pressure IO field')
x = replace_once(x, '''		"socket_fds":           stats.SocketFDUpdateFailures,
		"socket_fd_parents":    stats.SocketParentUpdateFailures,
''', '''		"socket_fds":           stats.SocketFDUpdateFailures,
		"socket_fd_parents":    stats.SocketParentUpdateFailures,
		"exit_io_ctx":          stats.ExitIOUpdateFailures,
''', 'Go pressure IO map')
x = replace_once(x, '''		total.SocketFDUpdateFailures += value.SocketFDUpdateFailures
		total.SocketParentUpdateFailures += value.SocketParentUpdateFailures
''', '''		total.SocketFDUpdateFailures += value.SocketFDUpdateFailures
		total.SocketParentUpdateFailures += value.SocketParentUpdateFailures
		total.ExitIOUpdateFailures += value.ExitIOUpdateFailures
''', 'Go pressure IO aggregate')
p.write_text(x)

p = Path('backend/app/observability/metrics_collector_test.go')
x = p.read_text()
x = replace_once(x,
'''{ExitFullUpdateFailures: 1, ExitCompactUpdateFailures: 2, SinglePathUpdateFailures: 3, PairPathUpdateFailures: 4, SocketFDUpdateFailures: 5, SocketParentUpdateFailures: 6},
		{ExitFullUpdateFailures: 10, ExitCompactUpdateFailures: 20, SinglePathUpdateFailures: 30, PairPathUpdateFailures: 40, SocketFDUpdateFailures: 50, SocketParentUpdateFailures: 60},
''',
'''{ExitFullUpdateFailures: 1, ExitCompactUpdateFailures: 2, SinglePathUpdateFailures: 3, PairPathUpdateFailures: 4, SocketFDUpdateFailures: 5, SocketParentUpdateFailures: 6, ExitIOUpdateFailures: 7},
		{ExitFullUpdateFailures: 10, ExitCompactUpdateFailures: 20, SinglePathUpdateFailures: 30, PairPathUpdateFailures: 40, SocketFDUpdateFailures: 50, SocketParentUpdateFailures: 60, ExitIOUpdateFailures: 70},
''', 'pressure test IO inputs')
x = replace_once(x, 'got.SocketFDUpdateFailures != 55 || got.SocketParentUpdateFailures != 66 {',
                 'got.SocketFDUpdateFailures != 55 || got.SocketParentUpdateFailures != 66 || got.ExitIOUpdateFailures != 77 {',
                 'pressure test IO aggregate')
x = replace_once(x, 'failures["socket_fds"] != 55 || failures["socket_fd_parents"] != 66 {',
                 'failures["socket_fds"] != 55 || failures["socket_fd_parents"] != 66 || failures["exit_io_ctx"] != 77 {',
                 'pressure test IO map')
p.write_text(x)

test = '''package app

import (
	"os"
	"strings"
	"testing"
)

func ioCorrelationBlock(t *testing.T, src, name, side string) string {
	t.Helper()
	marker := "int tracepoint__syscalls__sys_" + side + "_" + name + "("
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("missing %s %s handler", side, name)
	}
	end := strings.Index(src[start:], "\\n}\\n")
	if end < 0 {
		t.Fatalf("missing end of %s %s handler", side, name)
	}
	return src[start : start+end]
}

func TestCompactIOCorrelationSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	syscallBytes, err := os.ReadFile("../ebpf/agent_tracker_syscalls.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	syscalls := string(syscallBytes)

	for _, needle := range []string{
		"_Static_assert(sizeof(struct exit_meta) == 88",
		"_Static_assert(sizeof(struct exit_io_meta) == 64",
		"__uint(max_entries, 6144);",
		"} exit_io_ctx SEC(\\\".maps\\\");",
		"__uint(max_entries, 4096);",
		"store_exit_io_meta",
		"consume_exit_io_meta",
		"fill_from_exit_io_meta",
	} {
		if !strings.Contains(common, needle) {
			t.Fatalf("missing compact I/O contract %q", needle)
		}
	}

	for _, name := range []string{"read", "write", "readv", "writev"} {
		enter := ioCorrelationBlock(t, syscalls, name, "enter")
		exit := ioCorrelationBlock(t, syscalls, name, "exit")
		if !strings.Contains(enter, "struct exit_io_meta") ||
			!strings.Contains(enter, "store_exit_io_meta") ||
			strings.Contains(enter, "store_exit_meta(") {
			t.Fatalf("%s enter does not exclusively use exit_io_ctx", name)
		}
		if !strings.Contains(exit, "struct exit_io_meta") ||
			!strings.Contains(exit, "consume_exit_io_meta") ||
			!strings.Contains(exit, "fill_from_exit_io_meta") ||
			strings.Contains(exit, "consume_exit_meta(") {
			t.Fatalf("%s exit does not exclusively use exit_io_ctx", name)
		}
	}
}
'''
Path('backend/app/runtime_io_correlation_source_test.go').write_text(test)

doc = Path('docs/backend/generic-api-capture.md')
d = doc.read_text()
addition = '''

### Compact I/O correlation context

High-frequency read/write/readv/writev enter/exit correlation uses a dedicated
64-byte exit_io_ctx instead of the 88-byte full network exit_ctx. The compact
I/O value retains fd, L7 capture state, user-buffer pointer, remote endpoint,
capture provenance, and socket type, while direction/byte count are reconstructed
at exit from the syscall and return value.

The full and I/O hash maps split the previous 10,240-entry budget into 6,144
full-network entries plus 4,096 I/O entries. Total entry count is unchanged,
while aggregate preallocated value payload decreases and each I/O map update /
lookup moves about 27% fewer value bytes. exit_io_ctx has independent failure
and occupancy telemetry in collector health/Prometheus output.
'''
if addition not in d:
    d += addition
doc.write_text(d)
