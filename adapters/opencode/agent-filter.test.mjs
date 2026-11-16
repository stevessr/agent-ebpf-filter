import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { queryDecision, createHooks } from './agent-filter.mjs';
const command = value => [process.execPath,'-e',`process.stdin.resume();process.stdin.on('end',()=>process.stdout.write(${JSON.stringify(value)}))`];
test('only explicit policy decisions; reject shadow result and extra output', async()=>{
  assert.deepEqual(await queryDecision(command('{"action":"ALERT"}'),{tool:'read'}),{action:'ALERT'});
  for(const value of ['{"status":"above_threshold","mode":"shadow_only","action":"BLOCK"}','{"action":"ALLOW"}\n{}','{"action":"OTHER"}']) await assert.rejects(queryDecision(command(value),{}));
  await assert.rejects(queryDecision(['relative-command'],{}));
});
test('timeout, process failure, response and input limits fail closed',async()=>{
  await assert.rejects(queryDecision([process.execPath,'-e','setTimeout(()=>{},10000)'],{},30));
  await assert.rejects(queryDecision([process.execPath,'-e','process.exit(1)'],{}));
  await assert.rejects(queryDecision(command('x'.repeat(270000)),{}));
  await assert.rejects(queryDecision(command('{}'),{x:'x'.repeat(270000)}));
});
test('enforce BLOCK aborts, ALERT proceeds, observe does not block; logs omit args',async()=>{
  const dir=mkdtempSync(join(tmpdir(),'opencode-hook-'));const auditPath=join(dir,'audit.jsonl');
  try {
    const input={tool:'read',callID:'call1',sessionID:'session1'},output={args:{filePath:'DUMMY_SECRET_9541'}};
    for(const [mode,action,blocked] of [['enforce','BLOCK',true],['enforce','ALERT',false],['observe','BLOCK',false]]){
      const h=createHooks({argv:command(JSON.stringify({action})),mode,auditPath});
      if(blocked) await assert.rejects(h['tool.execute.before'](input,output));else await h['tool.execute.before'](input,output);
    }
    const h=createHooks({argv:['/definitely/missing'],mode:'enforce',auditPath});await assert.rejects(h['tool.execute.before'](input,output));
    const raw=readFileSync(auditPath,'utf8');assert.ok(!raw.includes('DUMMY_SECRET'));const rows=raw.trim().split('\n').map(JSON.parse);assert.equal(rows.filter(x=>x.blocked).length,2);assert.equal(rows.filter(x=>x.transport_error).length,1);
  } finally {rmSync(dir,{recursive:true,force:true});}
});
