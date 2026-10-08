import LocalSubprocessRuntime from '@deepseek-ai/dsh-subprocess-local'
import { wrapSpawnSpec } from './wrap.js'

/**
 * DeepSeek Harness subprocess provider that keeps the official local provider's
 * containment, stdio, cancellation, output and PTY semantics while inserting
 * Agent eBPF's policy boundary immediately before every Harness-owned exec.
 */
export class AgentEbpfSubprocessRuntime extends LocalSubprocessRuntime {
  spawn(spec) {
    return super.spawn(wrapSpawnSpec(spec))
  }

  async spawnTerminal(spec) {
    return super.spawnTerminal(wrapSpawnSpec(spec))
  }
}

export default AgentEbpfSubprocessRuntime
