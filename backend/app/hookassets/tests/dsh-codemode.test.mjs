import assert from 'node:assert/strict';
import { test } from 'node:test';
import { chmod, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { setTimeout as sleep } from 'node:timers/promises';

test('native Cordis hook relays PTC subcall lifecycles without arguments or result bodies', async (t) => {
  const directory = await mkdtemp(join(tmpdir(), 'agent-ebpf-dsh-ptc-'));
  const relayPath = join(directory, 'relay.sh');
  const outputPath = join(directory, 'events.jsonl');
  const oldOutput = process.env.AGENT_EBPF_TEST_LOG;
  process.env.AGENT_EBPF_TEST_LOG = outputPath;
  t.after(async () => {
    if (oldOutput === undefined) delete process.env.AGENT_EBPF_TEST_LOG;
    else process.env.AGENT_EBPF_TEST_LOG = oldOutput;
    await rm(directory, { recursive: true, force: true });
  });

  await writeFile(relayPath, '#!/bin/sh\ndata="$(cat)"\nprintf \'%s\\n\' "$data" >> "$AGENT_EBPF_TEST_LOG"\n');
  await chmod(relayPath, 0o700);
  const template = await readFile(new URL('../dsh.mjs', import.meta.url), 'utf8');
  assert.match(template, /__RELAY_PATH__/);
  const generated = template.replace('__RELAY_PATH__', JSON.stringify(relayPath));
  const generatedPath = join(directory, 'plugin.mjs');
  await writeFile(generatedPath, generated);
  const { apply } = await import(pathToFileURL(generatedPath).href);

  const listeners = new Map();
  let cleanup = () => {};
  apply({
    on(name, listener) { listeners.set(name, listener); },
    effect(register) { cleanup = register(); },
  });
  t.after(() => cleanup());

  const session = { id: 'session-test', header: { cwd: '/workspace/project' } };
  const emit = (type, data) => listeners.get('session/event')(session, { type, data });
  const secret = 'DO_NOT_LEAK_PTC_ARGUMENTS_OR_OUTPUT';
  emit('tool/call', { name: 'run_code', callId: 'root-call' });
  emit('tool/ptc-dispatch-start', {
    rootCallId: 'root-call', parentCallId: 'root-call',
    subCallId: 'root-call:ptc:1', name: 'read_file', arguments: { code: secret },
  });
  emit('tool/ptc-dispatch', {
    rootCallId: 'root-call', parentCallId: 'root-call',
    subCallId: 'root-call:ptc:1', name: 'read_file', arguments: { code: secret },
    isError: true, content: [{ type: 'text', text: secret }],
  });
  emit('tool/result', {
    message: { content: [{ type: 'tool-result', toolCallId: 'root-call', isError: true }] },
  });

  let events = [];
  for (let attempts = 0; attempts < 150; attempts++) {
    try {
      const lines = (await readFile(outputPath, 'utf8')).trim().split('\n');
      events = lines.filter(Boolean).map((line) => JSON.parse(line));
    } catch {
      // The first relay child may not have created the output file yet.
    }
    if (events.length >= 4) break;
    await sleep(20);
  }
  assert.equal(events.length, 4, 'all outer and PTC events should be relayed');
  const start = events.find((event) => event.hook_event_name === 'ptc_dispatch_start');
  const settle = events.find((event) => event.hook_event_name === 'ptc_dispatch');
  assert.equal(start?.parent_tool_call_id, 'root-call');
  assert.equal(start?.root_tool_call_id, 'root-call');
  assert.equal(start?.tool_call_id, settle?.tool_call_id);
  assert.equal(settle?.tool_result?.is_error, true);
  assert.equal(start?.code_mode_engine, 'dsh-ptc');
  for (const event of events) {
    assert.equal(event.pid, process.pid);
    assert.equal(event.session_id, 'session-test');
    assert.equal(event.tool_input, undefined);
    assert.equal(event.arguments, undefined);
    assert.equal(event.content, undefined);
  }
  assert.doesNotMatch(JSON.stringify(events), /DO_NOT_LEAK_PTC_ARGUMENTS_OR_OUTPUT/);
});
