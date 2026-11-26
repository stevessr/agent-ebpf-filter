#!/usr/bin/env node
// Validates supplied interval metadata and classifications, not empirical provenance.
// Hard safety gates and primary FR acceptance require separate actual tests.
import { readFileSync } from "node:fs";
import { analyze, data } from "./verify_axiomatic_matrix.mjs";
function measurementAudit(input,data,analyze){
 const req=(x,msg)=>{if(!x)throw Error(msg)};
 const text=x=>typeof x==="string"&&x.trim().length>0;
 const finite=x=>typeof x==="number"&&Number.isFinite(x);
 const result=analyze(data),n=data.names.length;
 req(input&&input.schemaVersion===1&&input.baselineCommit===data.baseline,"schema or baseline mismatch");
 req(Array.isArray(input.cells)&&input.cells.length===n*n,"expected all matrix cells");
 const expected=new Map(result.current.flatMap((row,i)=>row.map((cell,j)=>{const f=x=>String(x+1).padStart(2,"0");return [`F${f(i)}/D${f(j)}`,{structure:cell.kind,target:result.target[i][j].kind}]})));
 const seen=new Set(),counts={},out=[];
 const numeric=["delta","ciLower","ciUpper","tolerance","independentRuns"];
 const modes=new Set(data.modes.map(x=>x.id));
 const nonempty=input.cells.some(x=>x.status!=="not_run");
 if(nonempty){
  req(/^[0-9a-f]{40}$/.test(input.experimentCommit??"")&&modes.has(input.mode)&&text(input.workload)&&text(input.host)&&/^[0-9a-f]{64}$/.test(input.configHash??"")&&text(input.metricsVersion)&&/^[0-9a-f]{64}$/.test(input.planHash??"")&&finite(input.confidenceLevel)&&input.confidenceLevel>0&&input.confidenceLevel<1&&text(input.intervalMethod)&&text(input.multiplicityPolicy),"incomplete experiment provenance or statistical plan");
 }
 for(const c of input.cells){
  const id=c.fr+"/"+c.dp, e=expected.get(id);
  req(e&&!seen.has(id)&&c.structure===e.structure&&c.target===e.target,"duplicate, invalid or changed cell: "+id);
  seen.add(id);let classification;
  if(c.status==="not_run"){
   req([...numeric,"metric","unit","evidence"].every(k=>c[k]===null),"unrun cell contains results: "+id);
   classification="not_run";
  }else if(c.status==="unsupported"){
   req(numeric.every(k=>c[k]===null)&&c.metric===null&&c.unit===null&&text(c.evidence),"unsupported must have reason and no results: "+id);
   classification="unsupported";
  }else if(c.status==="measured"){
   req(text(c.metric)&&text(c.unit)&&text(c.evidence)&&numeric.every(k=>finite(c[k])),"incomplete measurements: "+id);
   req(c.ciLower<=c.delta&&c.delta<=c.ciUpper&&c.tolerance>=0&&Number.isInteger(c.independentRuns)&&c.independentRuns>=2,"invalid interval, tolerance or sample size: "+id);
   // The supplied interval must match the predeclared metric's estimand.
   // This validator checks the data contract, not confidence interval provenance.
   if(e.target!=="Z")classification="measured_not_isolation";
   else if(c.ciLower>=-c.tolerance&&c.ciUpper<=c.tolerance)classification="equivalent";
   else if(c.ciLower>c.tolerance||c.ciUpper< -c.tolerance)classification="violated";
   else classification="inconclusive";
  }else throw Error("unsupported status: "+id);
  counts[classification]=(counts[classification]??0)+1;out.push({id,classification});
 }
 const z=out.filter(x=>expected.get(x.id).target==="Z");
 return {counts,isolations:{total:z.length,equivalent:z.filter(x=>x.classification==="equivalent").length,violated:z.filter(x=>x.classification==="violated").length,inconclusive:z.filter(x=>x.classification==="inconclusive").length,notRun:z.filter(x=>x.classification==="not_run").length,unsupported:z.filter(x=>x.classification==="unsupported").length},completeIsolationEvidence:z.every(x=>x.classification==="equivalent"),cells:out};
}
function selfTest(data,analyze,audit){
 const r=analyze(data),f=i=>String(i+1).padStart(2,"0");
 const fixture={schemaVersion:1,baselineCommit:data.baseline,experimentCommit:null,mode:null,workload:null,host:null,configHash:null,metricsVersion:null,planHash:null,confidenceLevel:null,intervalMethod:null,multiplicityPolicy:null,cells:r.current.flatMap((row,i)=>row.map((c,j)=>({fr:"F"+f(i),dp:"D"+f(j),structure:c.kind,target:r.target[i][j].kind,status:"not_run",metric:null,unit:null,delta:null,ciLower:null,ciUpper:null,tolerance:null,independentRuns:null,evidence:null})))};
 const assert=(x,m)=>{if(!x)throw Error(m)};
 const blank=audit(fixture,data,analyze);assert(blank.counts.not_run===196&&!blank.completeIsolationEvidence,"empty template");
 const measured=JSON.parse(JSON.stringify(fixture));Object.assign(measured,{experimentCommit:"a".repeat(40),mode:"observe",workload:"synthetic fixture",host:"synthetic fixture",configHash:"b".repeat(64),metricsVersion:"fixture-v1",planHash:"c".repeat(64),confidenceLevel:0.95,intervalMethod:"synthetic fixture",multiplicityPolicy:"synthetic fixture"});
 const set=(id,lower,delta,upper,tol)=>Object.assign(measured.cells.find(x=>x.fr+"/"+x.dp===id),{status:"measured",metric:"synthetic paired difference",unit:"fixture unit",delta,ciLower:lower,ciUpper:upper,tolerance:tol,independentRuns:20,evidence:"synthetic fixture, not production"});
 set("F01/D02",-1,0,1,1);set("F01/D03",2,3,4,1);set("F01/D04",-2,0,1,1);set("F01/D05",-4,-3,-2,1);set("F01/D01",0,1,2,1);
 Object.assign(measured.cells.find(x=>x.fr+"/"+x.dp==="F01/D06"),{status:"unsupported",evidence:"synthetic capability unavailable"});
 const mixed=audit(measured,data,analyze);assert(mixed.counts.equivalent===1&&mixed.counts.violated===2&&mixed.counts.inconclusive===1&&mixed.counts.measured_not_isolation===1&&mixed.counts.unsupported===1&&!mixed.completeIsolationEvidence,"interval classification");
 const mutate=[x=>x.cells.pop(),x=>x.cells[1]=x.cells[0],x=>x.cells[1].target="A",x=>x.cells[1].ciLower=2,x=>x.cells[1].tolerance=-1,x=>x.cells[1].independentRuns=1,x=>x.cells[1].independentRuns=2.5,x=>x.cells[1].evidence="",x=>x.cells[1].status="pass",x=>x.configHash="",x=>x.cells.at(-1).delta=0,x=>x.cells[1].ciUpper=Infinity,x=>x.cells[5].evidence="",x=>x.planHash="",x=>x.confidenceLevel=1,x=>x.intervalMethod="",x=>x.multiplicityPolicy=""];
 for(const m of mutate){const bad=JSON.parse(JSON.stringify(measured));m(bad);let failed=false;try{audit(bad,data,analyze)}catch{failed=true}assert(failed,"negative fixture accepted")}
 return {scope:"synthetic data contract tests only",blank:blank.counts,mixed:mixed.counts,negativeCases:mutate.length};
}
if(process.argv[2]==="--self-test"){
 console.log(JSON.stringify(selfTest(data,analyze,measurementAudit),null,2));
}else if(process.argv[2]){
 const result=measurementAudit(JSON.parse(readFileSync(process.argv[2],"utf8")),data,analyze);
 console.log(JSON.stringify(result,null,2));
 if(result.isolations.violated>0 || (process.argv.includes("--require-complete-isolation")&&!result.completeIsolationEvidence))process.exitCode=1;
}else{
 console.error("usage: node scripts/verify_axiomatic_measurements.mjs --self-test | FILE [--require-complete-isolation]");
 process.exitCode=2;
}
