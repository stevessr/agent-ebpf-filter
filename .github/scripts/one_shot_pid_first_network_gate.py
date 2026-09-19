from pathlib import Path

def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:260]!r}")
    return text.replace(old, new, 1)

def patch_handler(text: str, name: str, pid_expr: str) -> str:
    marker = f'int tracepoint__syscalls__sys_enter_{name}(struct trace_event_raw_sys_enter *ctx) {{'
    start = text.find(marker)
    if start < 0:
        raise SystemExit(f'missing sys_enter handler {name}')
    end = text.find('\n}\n', start)
    if end < 0:
        raise SystemExit(f'missing end of sys_enter handler {name}')
    end += 3
    block = text[start:end]
    block = replace_once(
        block,
        '''    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
''',
        '',
        f'{name} unconditional comm read',
    )
    block = replace_once(
        block,
        f'    u32 tag_id = get_tag_id({pid_expr}, comm, NULL);',
        f'    u32 tag_id = get_enter_tag_id_nopath({pid_expr});',
        f'{name} PID-first tag lookup',
    )
    return text[:start] + block + text[end:]

sys_path = Path('backend/ebpf/agent_tracker_syscalls.h')
sys = sys_path.read_text()
for name, pid_expr in (
    ('socket', 'tgid'),
    ('bind', 'pid'),
    ('sendto', 'pid'),
    ('recvfrom', 'pid'),
    ('close', 'tgid'),
    ('read', 'tgid'),
    ('write', 'tgid'),
    ('writev', 'tgid'),
    ('readv', 'tgid'),
    ('sendmsg', 'tgid'),
    ('recvmsg', 'tgid'),
):
    sys = patch_handler(sys, name, pid_expr)
sys_path.write_text(sys)

common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()
common = patch_handler(common, 'connect', 'pid')
common_path.write_text(common)

test_path = Path('backend/app/runtime_tracking_mode_source_test.go')
test = test_path.read_text()
addition = r'''

func TestPIDFirstNetworkEnterSourceContract(t *testing.T) {
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

	assertLazy := func(src, name string) {
		t.Helper()
		marker := "int tracepoint__syscalls__sys_enter_" + name + "(struct trace_event_raw_sys_enter *ctx) {"
		block := sourceBlock(t, src, marker, "\n}\n")
		if !strings.Contains(block, "get_enter_tag_id_nopath") {
			t.Fatalf("%s bypasses PID-first no-path matcher", name)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", name)
		}
	}

	assertLazy(common, "connect")
	for _, name := range []string{
		"socket", "bind", "sendto", "recvfrom", "close",
		"read", "write", "writev", "readv", "sendmsg", "recvmsg",
	} {
		assertLazy(syscalls, name)
	}
}
'''
if 'func TestPIDFirstNetworkEnterSourceContract' not in test:
    test += addition
test_path.write_text(test)

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''

The same PID-first selector path is applied to correlation-only explicit network
and descriptor syscalls: socket, connect, bind, sendto/recvfrom, sendmsg/recvmsg,
read/readv, write/writev, and close. These enter programs only need comm for
selector fallback, while their eventual sys_exit event reconstructs comm
independently. Immediate TCP tracepoint emitters remain outside this optimization.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
