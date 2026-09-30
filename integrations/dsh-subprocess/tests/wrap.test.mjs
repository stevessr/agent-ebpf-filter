import test from 'node:test'
import assert from 'node:assert/strict'
import { wrapArgv, wrapEnvironment, wrapSpawnSpec } from '../src/wrap.js'

test('wrapArgv preserves exact argument boundaries', () => {
  const argv = ['python', '', '  spaced  ', '--flag=value']
  assert.deepEqual(
    wrapArgv(argv, { AGENT_EBPF_WRAPPER: '/opt/agent-wrapper' }),
    ['/opt/agent-wrapper', '--dsh-exec', '--verbatim', '--', ...argv],
  )
  assert.deepEqual(argv, ['python', '', '  spaced  ', '--flag=value'])
})

test('wrapEnvironment stamps dsh exec attribution without overwriting explicit context', () => {
  assert.deepEqual(
    wrapEnvironment({ AGENT_EBPF_TOOL_NAME: 'existing', KEEP: '1' }, 4242),
    {
      AGENT_EBPF_TOOL_NAME: 'existing',
      KEEP: '1',
      AGENT_EBPF_DSH_EXEC: '1',
      AGENT_EBPF_ROOT_AGENT_PID: '4242',
    },
  )
})

test('wrapSpawnSpec leaves non-argv subprocess semantics untouched', () => {
  const signal = new AbortController().signal
  const spec = {
    argv: ['git', 'status'],
    cwd: '/work',
    env: { CUSTOM: 'yes' },
    stdio: { stdin: 'ignore', stdout: 'inherit', stderr: 'inherit' },
    graceMs: 3000,
    signal,
  }
  const wrapped = wrapSpawnSpec(spec, { AGENT_EBPF_WRAPPER: 'agent-wrapper' }, 99)
  assert.equal(wrapped.cwd, spec.cwd)
  assert.equal(wrapped.stdio, spec.stdio)
  assert.equal(wrapped.signal, signal)
  assert.equal(wrapped.graceMs, 3000)
  assert.deepEqual(wrapped.argv, ['agent-wrapper', '--dsh-exec', '--verbatim', '--', 'git', 'status'])
  assert.equal(wrapped.env.CUSTOM, 'yes')
  assert.equal(wrapped.env.AGENT_EBPF_ROOT_AGENT_PID, '99')
})
