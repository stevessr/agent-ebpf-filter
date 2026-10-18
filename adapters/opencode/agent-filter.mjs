// Opt-in OpenCode v1 execution bridge. Not a syscall sandbox.
import { spawn } from 'node:child_process';
import { appendFileSync } from 'node:fs';
import { createHash } from 'node:crypto';

const MAX_BYTES = 256 * 1024;
export async function queryDecision(argv, request, timeoutMs = 2000) {
  if (!Array.isArray(argv) || !argv.length || !argv.every(x => typeof x === 'string') || !argv[0].startsWith('/')) throw new Error('invalid decision command');
  const payload = JSON.stringify(request);
  if (Buffer.byteLength(payload) > MAX_BYTES) throw new Error('request limit');
  return new Promise((resolve, reject) => {
    const child = spawn(argv[0], argv.slice(1), { stdio: ['pipe', 'pipe', 'ignore'], shell: false });
    let output = [], length = 0, settled = false;
    const finish = (err, value) => { if (settled) return; settled = true; clearTimeout(timer); if (err) { child.kill('SIGKILL'); reject(new Error('decision unavailable')); } else resolve(value); };
    const timer = setTimeout(() => finish(new Error('timeout')), timeoutMs);
    child.on('error', err => finish(err));
    child.stdin.on('error', err => finish(err));
    child.stdout.on('data', b => { length += b.length; if (length > MAX_BYTES) finish(new Error('response limit')); else output.push(b); });
    child.on('close', code => {
      if (settled) return;
      try {
        if (code !== 0) throw new Error('failed');
        const value = JSON.parse(Buffer.concat(output).toString('utf8'));
        if (!value || !['ALLOW','ALERT','BLOCK'].includes(value.action) || value.mode === 'shadow_only') throw new Error('unsupported verdict');
        finish(null, { action: value.action });
      } catch (err) { finish(err); }
    });
    child.stdin.end(payload);
  });
}

export function createHooks({ argv, mode, auditPath }) {
  if (!['observe','enforce'].includes(mode)) throw new Error('explicit bridge mode required');
  const audit = row => appendFileSync(auditPath, JSON.stringify({schema:'opencode-intercept.v1',...row})+'\n', {mode:0o600});
  return {
    'tool.execute.before': async (input, output) => {
      const request = { tool: input.tool, args: output.args, session_id: input.sessionID, call_id: input.callID };
      const digest = createHash('sha256').update(JSON.stringify(request.args)).digest('hex');
      const started = performance.now();
      let decision, error = false;
      try { decision = await queryDecision(argv, request); } catch { error = true; decision = {action:'BLOCK'}; }
      const blocked = mode === 'enforce' && decision.action === 'BLOCK';
      // Failure to record enforcement evidence also aborts the tool call.
      audit({phase:'before',tool:input.tool,call_id:input.callID,session_id:input.sessionID,args_sha256:digest,mode,action:decision.action,blocked,transport_error:error,latency_ms:performance.now()-started});
      if (blocked) throw new Error(error ? 'Agent filter: decision service unavailable' : 'Agent filter: policy denied this tool call');
    },
    'tool.execute.after': async (input) => {
      audit({phase:'after',tool:input.tool,call_id:input.callID,session_id:input.sessionID,mode});
    },
  };
}

export const AgentFilter = async () => createHooks({
  argv: JSON.parse(process.env.AGENT_OPENCODE_DECISION_ARGV || 'null'),
  mode: process.env.AGENT_OPENCODE_MODE,
  auditPath: process.env.AGENT_OPENCODE_AUDIT,
});
