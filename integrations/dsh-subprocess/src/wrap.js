const DEFAULT_WRAPPER = 'agent-wrapper'

export function wrapperExecutable(env = process.env) {
  const configured = env.AGENT_EBPF_WRAPPER?.trim()
  return configured || DEFAULT_WRAPPER
}

export function wrapArgv(argv, env = process.env) {
  if (!Array.isArray(argv) || argv.length === 0 || typeof argv[0] !== 'string' || argv[0].length === 0) {
    throw new Error('agent-ebpf dsh subprocess: argv must contain an executable')
  }
  const wrapper = wrapperExecutable(env)
  if (argv[0] === wrapper || argv[0].endsWith('/agent-wrapper')) return [...argv]
  return [wrapper, '--dsh-exec', '--verbatim', '--', ...argv]
}

export function wrapEnvironment(original, pid = process.pid) {
  const next = { ...original }
  next.AGENT_EBPF_DSH_EXEC = '1'
  next.AGENT_EBPF_TOOL_NAME ??= 'dsh.exec'
  next.AGENT_EBPF_ROOT_AGENT_PID ??= String(pid)
  return next
}

export function wrapSpawnSpec(spec, env = process.env, pid = process.pid) {
  return {
    ...spec,
    argv: wrapArgv(spec.argv, env),
    env: wrapEnvironment(spec.env, pid),
  }
}
