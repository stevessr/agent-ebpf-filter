// agent-ebpf-hook-active
// Generated integration: metadata-only, best-effort, no tool interception.
import { spawn } from 'node:child_process';
export const name = 'agent-ebpf-hook-active';
const relayPath = __RELAY_PATH__;

export function apply(ctx) {
  const children = new Set();
  let disposed = false;
  function relay(eventName, payload) {
    if (disposed || children.size >= 8) return;
    try {
      const body = JSON.stringify({ hook_event_name: eventName, pid: process.pid, ...payload });
      const child = spawn(relayPath, [eventName], { stdio: ['pipe', 'ignore', 'ignore'] });
      children.add(child);
      const timer = setTimeout(() => { child.kill(); }, 3000);
      timer.unref();
      const done = () => { clearTimeout(timer); children.delete(child); };
      child.once('error', done);
      child.once('close', done);
      child.stdin.on('error', () => {});
      child.stdin.end(body);
      child.unref();
    } catch { /* Observability must not interrupt the agent. */ }
  }
  const context = (session) => ({ session_id: session.id, cwd: session.header?.cwd ?? '' });
  // PTC events are log-only sub-dispatches. Never forward their arguments or
  // result bodies: the host owns those and only its PID/syscalls are OS facts.
  const ptcContext = (session, data) => ({
    ...context(session),
    execution_mode: 'code_mode',
    code_mode_engine: 'dsh-ptc',
    parent_tool_call_id: data.parentCallId ?? '',
    tool_call_id: data.subCallId ?? '',
    tool_name: data.name ?? '',
  });
  ctx.on('session/created', (session) => relay('session_start', context(session)));
  ctx.on('session/event', (session, event) => {
    const data = event.data ?? {};
    if (event.type === 'tool/ptc-dispatch-start') {
      relay('ptc_dispatch_start', ptcContext(session, data));
    } else if (event.type === 'tool/ptc-dispatch') {
      relay('ptc_dispatch', {
        ...ptcContext(session, data),
        tool_result: { is_error: data.isError === true },
      });
    } else if (event.type === 'tool/call') {
      relay('tool_call', { ...context(session), tool_name: data.name, tool_call_id: data.callId });
    } else if (event.type === 'tool/result') {
      for (const block of data.message?.content ?? []) {
        if (block.type !== 'tool-result') continue;
        relay('tool_result', { ...context(session), tool_call_id: block.toolCallId, tool_result: { is_error: block.isError === true } });
      }
    }
  });
  ctx.effect(() => () => {
    disposed = true;
    for (const child of children) child.kill();
    children.clear();
  }, "agent-ebpf-hook: relay cleanup");
}
