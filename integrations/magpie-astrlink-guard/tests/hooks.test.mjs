import assert from 'node:assert/strict';
import { test } from 'node:test';
import { onRequest } from '../hooks.js';
import { onRequest as middleware } from '../guard.middleware.js';
import { redactText, collectTextSlots } from '../redaction_core.js';

const context = (options = {}) => ({ options, report() {} });
const sample = () => ({
  model: 'openai/gpt-test', stream: true, tool_choice: 'auto',
  tools: [{ type: 'function', function: { name: 'lookup', description: 'User jane@example.com', parameters: {} } }],
  messages: [{ role: 'user', content: 'Hi Alice, write to jane@example.com and use sk-ABCDEFGHIJKLMNOPQRST' }],
});

test('collects free text without modifying tool definitions', () => {
  const { slots } = collectTextSlots(sample());
  assert.equal(slots.length, 1);
  assert.match(slots[0].text, /jane@example.com/);
});

test('model spans are applied to content; model and tools remain unchanged', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async (_url, req) => {
    assert.match(String(_url), /^http:\/\/127\.0\.0\.1:/);
    const payload = JSON.parse(req.body);
    const start = payload.texts[0].indexOf('Alice');
    return { ok: true, json: async () => ({ spans: [[{start, end: start+5, label:'private_person', score:.99}]] }) };
  };
  try {
    const result = await onRequest(sample(), context({ action:'redact' }));
    assert.match(result.messages[0].content, /\[REDACTED:private_person\]/);
    assert.doesNotMatch(result.messages[0].content, /jane@example.com/);
    assert.equal(result.model, 'openai/gpt-test');
    assert.equal(result.stream, true);
    assert.match(result.tools[0].function.description, /jane@example.com/);
  } finally { globalThis.fetch = original; }
});

test('server failure uses deterministic local regex backup, not raw egress from the hook', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async () => { throw Error('down'); };
  try {
    const result = await onRequest(sample(), context({action:'redact'}));
    assert.doesNotMatch(result.messages[0].content, /jane@example.com/);
    assert.doesNotMatch(result.messages[0].content, /sk-ABCDEFGHIJKLMNOPQRST/);
  } finally { globalThis.fetch = original; }
});

test('non-local endpoint is rejected and never used', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async () => { throw Error('should not fetch'); };
  try {
    const result = await onRequest(sample(), context({serverUrl:'https://evil.example/scan'}));
    assert.doesNotMatch(result.messages[0].content, /jane@example.com/);
  } finally { globalThis.fetch = original; }
});

test('middleware independently redacts if async hook is skipped', () => {
  const b = sample();
  middleware(b, { options:{action:'redact'} });
  assert.doesNotMatch(b.messages[0].content, /jane@example.com/);
  assert.match(b.tools[0].function.description, /jane@example.com/);
});

test('oversized input is rejected by synchronous middleware', () => {
  const b = { messages: [{ content: 'a'.repeat(200) }] };
  const output = middleware(b, { options: {maxTextChars: 10}, reject: (status, message) => ({status,message}) });
  assert.equal(output.status, 413);
});

test('span offsets use JavaScript UTF-16 and overlapping spans are not duplicated', () => {
  const text = '😀Alice';
  assert.equal(redactText(text, [{start:2,end:7,label:'private_person',score:.99}, {start:2,end:7,label:'private_person',score:.9}]).count, 1);
});

test('audit mode does not mutate request even when model finds sensitive data', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async () => ({ ok:true, json: async () => ({ spans: [[{start:3,end:8,label:'private_person',score:1}]] }) });
  try {
    assert.equal(await onRequest(sample(), context({action:'audit'})), undefined);
  } finally { globalThis.fetch = original; }
});

test('nested Responses content receives redaction without touching image or tools', () => {
  const b = { model:'x', input:[{role:'user',content:[{type:'input_text',text:'alice@example.com'},
    {type:'input_image',image_url:'data:image/png;base64,aaaa'}]}], tools:[{name:'hello'}] };
  middleware(b, { options: {} });
  assert.match(b.input[0].content[0].text, /\[REDACTED:email\]/);
  assert.equal(b.input[0].content[1].image_url, 'data:image/png;base64,aaaa');
  assert.equal(b.tools[0].name, 'hello');
});
