#!/usr/bin/env node
// Statistical tool for predeclared bounded independent paired differences.
// Does not establish independence, verify measurement authenticity, or prove safety.
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { analyze, data } from "./verify_axiomatic_matrix.mjs";
function derive(plan,raw,planHash,data,analyze){
 const assert=(x,m)=>{if(!x)throw Error(m)}, str=x=>typeof x==="string"&&x.trim().length>0, num=x=>typeof x==="number"&&Number.isFinite(x);
 assert(plan?.schemaVersion===1&&plan.baselineCommit===data.baseline&&/^[0-9a-f]{40}$/.test(plan.experimentCommit??"")&&data.modes.some(x=>x.id===plan.mode)&&str(plan.workload)&&str(plan.host)&&/^[0-9a-f]{64}$/.test(plan.configHash??"")&&str(plan.metricsVersion),"invalid plan provenance");
 assert(num(plan.alpha)&&plan.alpha>0&&plan.alpha<1,"invalid alpha");
 assert(raw?.schemaVersion===1&&raw.planHash===planHash&&/^[0-9a-f]{64}$/.test(planHash),"raw samples not bound to plan");
 const result=analyze(data),f=i=>String(i+1).padStart(2,"0");
 const cells=result.current.flatMap((row,i)=>row.map((c,j)=>({fr:"F"+f(i),dp:"D"+f(j),structure:c.kind,target:result.target[i][j].kind,status:"not_run",metric:null,unit:null,delta:null,ciLower:null,ciUpper:null,tolerance:null,independentRuns:null,evidence:null})));
 const key=x=>x.fr+"/"+x.dp, cellMap=new Map(cells.map(c=>[key(c),c])), experiments=new Map();
 assert(Array.isArray(plan.experiments)&&plan.experiments.length>=1&&plan.experiments.length<=cells.length,"invalid experiment family");
 for(const p of plan.experiments){
  const id=key(p);
  assert(cellMap.has(id)&&!experiments.has(id)&&str(p.metric)&&str(p.unit)&&str(p.intervention)&&str(p.runUnit)&&num(p.minDifference)&&num(p.maxDifference)&&p.minDifference<p.maxDifference&&num(p.maxDifference-p.minDifference)&&num(p.tolerance)&&p.tolerance>=0&&Number.isSafeInteger(p.expectedPairs)&&p.expectedPairs>=2&&p.expectedPairs<=1000000,"invalid experiment "+id);
  experiments.set(id,p);
 }
 const m=experiments.size, factor=Math.log(2*m/plan.alpha);
 assert(num(factor),"nonfinite confidence factor");
 assert(Array.isArray(raw.records)&&raw.records.length<=m,"invalid raw records");
 const seen=new Set(),planning=[];
 for(const p of experiments.values()){
  const width=p.maxDifference-p.minDifference,half=width*Math.sqrt(factor/(2*p.expectedPairs));
  const required=p.tolerance>0?Math.ceil((width/p.tolerance)**2*factor/2):null;
  planning.push({id:key(p),expectedPairs:p.expectedPairs,halfWidth:half,pairsForHalfWidthAtMostTolerance:Number.isSafeInteger(required)?required:null});
 }
 for(const record of raw.records){
  const id=key(record),p=experiments.get(id);
  assert(p&&!seen.has(id)&&Array.isArray(record.pairs)&&record.pairs.length===p.expectedPairs&&str(record.evidence),"unknown, duplicate or incomplete record "+id);
  seen.add(id);
  const runs=new Set();let mean=0;
  for(let i=0;i<record.pairs.length;i++){
   const v=record.pairs[i];
   assert(str(v.runId)&&!runs.has(v.runId)&&num(v.baseline)&&num(v.intervention),"invalid or duplicate pair "+id);
   runs.add(v.runId);
   const delta=v.intervention-v.baseline;
   assert(num(delta)&&delta>=p.minDifference&&delta<=p.maxDifference,"difference outside preregistered support "+id);
   mean+=(delta-mean)/(i+1);
  }
  const half=(p.maxDifference-p.minDifference)*Math.sqrt(factor/(2*p.expectedPairs));
  assert(num(mean)&&num(half),"nonfinite estimate "+id);
  Object.assign(cellMap.get(id),{status:"measured",metric:p.metric,unit:p.unit,delta:mean,ciLower:Math.max(p.minDifference,mean-half),ciUpper:Math.min(p.maxDifference,mean+half),tolerance:p.tolerance,independentRuns:p.expectedPairs,evidence:record.evidence});
 }
 return {schemaVersion:1,baselineCommit:plan.baselineCommit,experimentCommit:plan.experimentCommit,mode:plan.mode,workload:plan.workload,host:plan.host,configHash:plan.configHash,metricsVersion:plan.metricsVersion,planHash,confidenceLevel:1-plan.alpha,intervalMethod:"bounded paired mean Hoeffding",multiplicityPolicy:"two-sided simultaneous union bound across "+m+" predeclared cells",cells,planning};
}
function verify(derive,data,analyze){
 const assert=(x,m)=>{if(!x)throw Error(m)}, h="a".repeat(64);
 const exp={fr:"F01",dp:"D02",metric:"synthetic binary difference",unit:"fraction",intervention:"fixture",runUnit:"synthetic paired run",minDifference:-1,maxDifference:1,tolerance:.25,expectedPairs:100};
 const plan={schemaVersion:1,baselineCommit:data.baseline,experimentCommit:"b".repeat(40),mode:"observe",workload:"synthetic",host:"synthetic",configHash:"c".repeat(64),metricsVersion:"fixture",alpha:.05,experiments:[exp,{...exp,dp:"D03"}]};
 const raw={schemaVersion:1,planHash:h,records:[{fr:"F01",dp:"D02",evidence:"synthetic",pairs:Array.from({length:100},(_,i)=>({runId:"pair-"+i,baseline:i%2,intervention:i%2}))}]};
 const v=derive(plan,raw,h,data,analyze),c=v.cells.find(x=>x.fr==="F01"&&x.dp==="D02");
 const expected=2*Math.sqrt(Math.log(80)/200);
 assert(c.delta===0&&Math.abs(c.ciLower+expected)<1e-12&&Math.abs(c.ciUpper-expected)<1e-12,"Hoeffding calculation");
 assert(v.cells.filter(x=>x.status==="not_run").length===195&&v.planning.length===2,"missing record not_run");
 assert(v.planning[0].pairsForHalfWidthAtMostTolerance===Math.ceil(32*Math.log(80)),"precision budget");
 const changes=[(p,s)=>s.planHash="d".repeat(64),(p,s)=>s.records[0].pairs.pop(),(p,s)=>s.records[0].pairs[1].runId=s.records[0].pairs[0].runId,(p,s)=>s.records[0].pairs[0].intervention=2,(p,s)=>p.experiments.push(p.experiments[0]),(p,s)=>p.alpha=0,(p,s)=>p.experiments[0].maxDifference=-2,(p,s)=>p.experiments[0].runUnit="",(p,s)=>s.records.push(s.records[0]),(p,s)=>s.records[0].evidence="",(p,s)=>s.records[0].pairs[0].baseline="0",(p,s)=>p.experiments[0].expectedPairs=1];
 for(const mutate of changes){const p=JSON.parse(JSON.stringify(plan)),s=JSON.parse(JSON.stringify(raw));mutate(p,s);let rejected=false;try{derive(p,s,h,data,analyze)}catch{rejected=true}assert(rejected,"invalid raw sample or plan accepted")}
 return {scope:"synthetic calculation test",expectedHalfWidth:expected,observedHalfWidth:c.ciUpper,notRun:195,negativeCases:changes.length};
}
if(process.argv[2]==="--self-test"){
 console.log(JSON.stringify(verify(derive,data,analyze),null,2));
}else if(process.argv.length===4){
 const planBytes=readFileSync(process.argv[2]);
 const hash=createHash("sha256").update(planBytes).digest("hex");
 const plan=JSON.parse(planBytes.toString("utf8")),raw=JSON.parse(readFileSync(process.argv[3],"utf8"));
 console.log(JSON.stringify(derive(plan,raw,hash,data,analyze),null,2));
}else{
 console.error("usage: node scripts/derive_axiomatic_measurements.mjs --self-test | PLAN.json RAW.json");
 process.exitCode=2;
}
