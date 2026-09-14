from pathlib import Path

p = Path('backend/ebpf/agent_tracker_syscalls.h')
s = p.read_text()


def patch_handler(name: str):
    global s
    marker = f'int tracepoint__syscalls__sys_enter_{name}(struct trace_event_raw_sys_enter *ctx) {{'
    start = s.find(marker)
    if start < 0:
        raise SystemExit(f'missing {name} enter handler')
    end = s.find('\n}\n', start)
    if end < 0:
        raise SystemExit(f'unterminated {name} enter handler')
    block = s[start:end]
    anchor = '    u32 tag_id = get_tag_id(pid, comm, NULL);\n'
    if anchor not in block:
        raise SystemExit(f'{name} tag lookup anchor missing')
    if 'if (tag_id == 0) return 0;' in block:
        raise SystemExit(f'{name} already has fast reject')
    block = block.replace(anchor, anchor + '    if (tag_id == 0) return 0;\n', 1)
    s = s[:start] + block + s[end:]

for name in ('bind', 'recvfrom'):
    patch_handler(name)

p.write_text(s)

doc = Path('docs/backend/generic-api-capture.md')
text = doc.read_text()
addition = '''\n### Untracked network fast reject\n\n`bind()` and `recvfrom()` now return immediately after a zero `get_tag_id()`\nresult, matching the rest of the network capture handlers. Untracked processes\ntherefore avoid enter/exit correlation map traffic and can no longer create\ntag-zero network events through these two tracepoints.\n'''
if addition not in text:
    text += addition
doc.write_text(text)
