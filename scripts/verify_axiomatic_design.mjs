#!/usr/bin/env node
// Reproduce the pinned-source evidence inventory; not a Go parser or a runtime test.
// A failed anchor means review is required, not necessarily a regression.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
const paths = [
  "backend/app/jobs_background.go",
  "backend/app/events/events_kernel_builder.go",
  "backend/app/networkflowsbridge.go",
  "backend/app/kernelriskbridge.go",
  "backend/app/events/kernel_risk.go",
  "backend/app/events/kernel_risk_feedback.go",
  "backend/app/sandboxbridge.go",
  "backend/app/sandbox/cgroup_ops.go",
  "backend/app/ws_api.go",
  "backend/app/statepersistence_events.go",
  "backend/app/redaction_engine.go",
  "backend/app/helpers_auth.go",
  "backend/app/http_security_test.go",
  "backend/app/agent_scope_policy.go"
];
function audit(files, requireMigrated = false) {
  // Preserve the baseline audit while also checking the migrated production path.
  const migrated=files[8].includes("admission := evaluateAgentEventScope(event)");
  const scopeAnchors=migrated?["admission := evaluateAgentEventScope(event)","if !admission.Capture","monitorAllowed := admission.Monitor"]:["if !captureAgentEvent(event)","monitorAllowed := monitorAgentEvent(event)"];
  const checks=[
    ["reader builds before enqueue",0,["enqueueBroadcastEvent(broadcast, buildKernelEventFromRaw(event)"]],
    ["builder invokes risk before return",1,["Deps.KernelRisk(event, out)","return out"]],
    ["bridge injects risk and enforcer",2,["events.Deps.KernelRisk = func","applyKernelRiskDecision((*bpfEvent)(raw), event)","events.Deps.Enforcer = sandboxEnforcer{}"]],
    ["bridge invokes risk",3,["func applyKernelRiskDecision","events.ApplyKernelRiskDecision(raw, event)"]],
    ["risk invokes feedback",4,["decision := evaluateKernelRiskDecision(raw, event)","queueKernelRiskFeedback(event, decision)"]],
    ["worker checks broad gates before applying",5,["case action := <-kernelRiskFeedbackQueue:","!settings.PolicyManagementEnabled || !settings.KernelRiskFeedback.Enabled","applyKernelRiskFeedbackAction(action)"]],
    ["feedback constructs then queues",5,["func queueKernelRiskFeedback","actions := kernelRiskFeedbackActions(settings, event, decision)","case kernelRiskFeedbackQueue <- action:"]],
    ["feedback invokes enforcer",5,["func applyKernelRiskFeedbackAction","return enforcer.BlockIP(action.Target)","return enforcer.BlockPort(uint16(port))"]],
    ["sandbox bridge",6,["sandbox.BlockIP","sandbox.BlockPort"]],
    ["cgroup map mutation",7,["func BlockIP","snap.IPBlocklist.Put","snap.IP6Blocklist.Put","func BlockPort","snap.PortBlocklist.Put"]],
    ["scope and semantic ordering",8,["event = enrichEventContext(event)",...scopeAnchors,"alerts = buildSemanticAlerts(event)","appendRecord(recordCapturedEventWithMonitoring(event, monitorAllowed))"]],
    ["normal record redacted before fanout/storage",9,["func recordCapturedEventWithMonitoring","redactCapturedEventRecord(record, globalRedactionEngine)","session.publishProto","capturedEventArchive.Add(record)","runtimeSettingsStore.enqueueEvent(record)","recording.Default().Record(record)","otelExporterStore.Record(record)"]],
    ["monitor scope before downstream research",9,["if monitorAllowed {","queueLoopDetectionRecord(record)","research.QueueProcessingRecord(record)"]],
    ["nil engine no-op",10,["func redactEvent","if event == nil || engine == nil","return"]],
    ["mode independent auth",11,["func authEnforced() bool","return os.Getenv(\"DISABLE_AUTH\") != \"true\""]],
    ["existing mode regression test",12,["func TestAuthenticationIndependentOfGinMode","gin.DebugMode, gin.ReleaseMode, gin.TestMode"]]
  ];
  if(migrated) checks.push(["single scope snapshot",13,["func evaluateAgentEventScope","agentRootScopes.Observe(event)","policy := currentAgentScopePolicy()","owner := agentRootScopes.OwnerComm(event)","return policy.Evaluate(event.GetComm(), event.GetTag(), owner)"]]);
  if(requireMigrated || files[5].includes("func kernelRiskFeedbackActionAllowed")) checks.push(["feedback reauthorization before budget and apply",5,["case action := <-kernelRiskFeedbackQueue:","settings := Deps.RuntimeSettingsSnapshot()","if !kernelRiskFeedbackActionAllowed(settings, action)","kernelRiskFeedbackDedup.Allow(action","applyKernelRiskFeedbackAction(action)"]]);
  const evidence=checks.map(([name,index,terms])=>{
    const src=files[index]; let pos=0; const lines=[];
    for(const term of terms){
      const at=src.indexOf(term,pos);
      if(at<0)return {name,file:paths[index],pass:false,missing:term};
      pos=at+term.length;lines.push(src.slice(0,at).split("\n").length);
    }
    return {name,file:paths[index],pass:true,lines};
  });
  // Rows FR1..FR6, columns DP1..DP6. X = structural edge;
  // ? = unresolved; . = excluded from this graph, not proven zero.
  const matrices={
    feedbackOff:["X?X?.?","XX..?.","XXX??.","..?X?.","???.X?","XXX?XX"],
    feedbackOn:["X?XX.?","XX..?.","XXX??.","X?XX?.","???.X?","XXX?XX"],
    targetStatic:["X..X..","XX....","XXX...","...X..","XXX.X.","XXX.XX"]
  };
  function permutations(a){
    return a.length===0?[[]]:a.flatMap((v,i)=>permutations(a.filter((_,j)=>i!==j)).map(p=>[v,...p]));
  }
  const orders=permutations([1,2,3,4,5,6]);
  const graph=Object.entries(matrices).map(([mode,rows])=>{
    const edges=rows.flatMap((row,i)=>[...row].flatMap((v,j)=>v==="X"&&i!==j?[[j+1,i+1]]:[]));
    const valid=orders.filter(p=>edges.every(([a,b])=>p.indexOf(a)<p.indexOf(b)));
    const reach=Array.from({length:6},(_,i)=>Array.from({length:6},(_,j)=>i===j||edges.some(([a,b])=>a===i+1&&b===j+1)));
    for(let k=0;k<6;k++)for(let i=0;i<6;i++)for(let j=0;j<6;j++)reach[i][j] ||= reach[i][k]&&reach[k][j];
    const groups=[];const seen=new Set();
    for(let i=0;i<6;i++){if(seen.has(i))continue;const group=[];for(let j=0;j<6;j++)if(reach[i][j]&&reach[j][i]){group.push(j+1);seen.add(j);}if(group.length>1)groups.push(group);}
    return {mode,rows,edges,permutations:orders.length,validOrders:valid.length,orders:valid,cyclicGroups:groups};
  });
  return {evidence,graph,migrated,expectedGraph:graph[0].validOrders===0&&graph[1].validOrders===0&&graph[2].validOrders===1&&graph[2].orders[0].join(",")==="4,1,2,3,5,6"};
}
const requireMigrated=process.argv.includes("--require-migrated");
const root = resolve(process.argv.slice(2).find(x=>!x.startsWith("--")) ?? ".");
const files = paths.map(path => readFileSync(resolve(root,path),"utf8"));
const result = audit(files, requireMigrated);
const negative = files.slice();
negative[1] = negative[1].replace("Deps.KernelRisk(event, out)","removed()");
const negativeDetected = audit(negative).evidence.some(x => !x.pass);
const migrationNegative = files.slice();
migrationNegative[5] = migrationNegative[5].replace("if !kernelRiskFeedbackActionAllowed(settings, action)", "if false");
const migrationNegativeDetected = !requireMigrated || audit(migrationNegative,true).evidence.some(x=>!x.pass);
console.log(JSON.stringify({...result,negativeDetected,migrationNegativeDetected},null,2));
if(!result.evidence.every(x=>x.pass)||!result.expectedGraph||!negativeDetected||!migrationNegativeDetected||(requireMigrated&&!result.migrated)) process.exitCode=1;
