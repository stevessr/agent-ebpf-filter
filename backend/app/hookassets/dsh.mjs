// agent-ebpf-hook-active
// Generated integration: metadata-only, best-effort, no tool interception.
import { spawn } from 'node:child_process';
export const name = 'agent-ebpf-hook-active';
const relayPath = __RELAY_PATH__;

export function apply(ctx) {
  const children = new Set();
  const pending = [];
  const maxPending = 256;
  const maxInflight = 8;
  let disposed = false;
  let dropped = 0;

  function relay(eventName, payload) {
    if (disposed) return;
    try {
      const body = JSON.stringify({ hook_event_name: eventName, pid: process.pid, ...payload });
      if (pending.length >= maxPending) {
        dropped++;
        return;
      }
      pending.push({ eventName, body });
      drain();
    } catch { /* Observability must not interrupt the agent. */ }
  }

  function drain() {
    if (disposed) return;
    // Explicitly surface loss after bursts; a truncated PTC trace is not complete evidence.
    if (dropped && pending.length < maxPending) {
      const count = dropped;
      dropped = 0;
      pending.push({
        eventName: 'telemetry_dropped',
        body: JSON.stringify({ hook_event_name: 'telemetry_dropped', pid: process.pid, dropped_events: count }),
      });
    }
    while (children.size < maxInflight && pending.length) {
      const { eventName, body } = pending.shift();
      try {
        const child = spawn(relayPath, [eventName], { stdio: ['pipe', 'ignore', 'ignore'] });
        children.add(child);
        const timer = setTimeout(() => child.kill(), 3000);
        timer.unref();
        let finished = false;
        const done = () => {
          if (finished) return;
          finished = true;
          clearTimeout(timer);
          children.delete(child);
          drain();
        };
        child.once('error', done);
        child.once('close', done);
        child.stdin.on('error', () => {});
        child.stdin.end(body);
        child.unref();
      } catch {
        dropped++;
      }
    }
  }
  const context = (session) => ({ session_id: session.id, cwd: session.header?.cwd ?? '' });
  // PTC events are log-only sub-dispatches. Never forward their arguments or
  // result bodies: the host owns those and only its PID/syscalls are OS facts.
  const ptcContext = (session, data) => ({
    ...context(session),
    execution_mode: 'code_mode',
    code_mode_engine: 'dsh-ptc',
    parent_tool_call_id: data.parentCallId ?? '',
    root_tool_call_id: data.rootCallId ?? '',
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
    pending.length = 0;
    for (const child of children) child.kill();
    children.clear();
  }, "agent-ebpf-hook: relay cleanup");
}
