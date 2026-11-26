#!/usr/bin/env node
// Qualitative structural graphs and desired contracts, not a measured Jacobian.
import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { resolve } from "node:path";
export const data = JSON.parse(readFileSync(fileURLToPath(new URL("./axiomatic_design_matrix.json", import.meta.url)), "utf8"));
export function analyze(d){
 if(!Array.isArray(d.names)||d.names.length<1||d.names.length>20||new Set(d.names).size!==d.names.length||!d.names.every(x=>typeof x==="string"&&x.trim()))throw Error("bad names");
 if(!Array.isArray(d.modes)||!d.modes.length||new Set(d.modes.map(m=>m.id)).size!==d.modes.length||!d.modes.every(m=>typeof m.id==="string"&&["wrapper","feedback","ipc"].every(k=>typeof m[k]==="boolean")))throw Error("bad modes");
 const n=d.names.length, matrix=(columns,target=false)=>{
  const a=Array.from({length:n},(_,i)=>Array.from({length:n},(_,j)=>i===j?{kind:"D",reason:"主责任映射，未证明达到指标"}:{kind:target?"Z":"?",reason:target?"在冻结适用范围内应满足交叉影响容差；未经测量":"未获得足够直接证据，不等于零"}));
  for(const [col,entries] of Object.entries(columns)){let seen=new Set();for(const entry of entries){if(!Array.isArray(entry)||entry.length!==(target?3:4))throw Error("bad entry shape");const [row,kind,x,y]=entry;if(!Number.isInteger(row)||String(+col)!==col||!Number.isInteger(+col)||row<1||row>n||+col<1||+col>n||row===+col||seen.has(row))throw Error("bad/duplicate cell");seen.add(row);if(target){if(!["A","W","C","B","I"].includes(kind)||typeof x!=="string"||!x.trim())throw Error("bad target");a[row-1][+col-1]={kind,reason:x};}else{if(!["X","H","W","C","B","I"].includes(kind)||typeof y!=="string"||!y.trim()||typeof x!=="string"||!x.split(" ").every(id=>Object.hasOwn(d.evidence,id)))throw Error("bad evidence");a[row-1][+col-1]={kind,evidence:x.split(" "),reason:y};}}}
  return a;
 };
 const current=matrix(d.currentColumns),target=matrix(d.targetColumns,true);
 const active=(kind,mode,hyp=false)=>["X","A"].includes(kind)||(kind==="W"&&mode.wrapper)||(kind==="C"&&mode.feedback)||(kind==="B"&&(mode.wrapper||mode.feedback))||(kind==="I"&&mode.ipc)||(kind==="H"&&hyp);
 function graph(a,m,hyp=false){
  const edges=[],pred=Array(n).fill(0);
  for(let i=0;i<n;i++)for(let j=0;j<n;j++)if(i!==j&&active(a[i][j].kind,m,hyp)){edges.push([j+1,i+1]);pred[i]|=1<<j;}
  const reach=Array.from({length:n},(_,i)=>Array.from({length:n},(_,j)=>i===j||edges.some(([u,v])=>u===i+1&&v===j+1)));
  for(let k=0;k<n;k++)for(let i=0;i<n;i++)for(let j=0;j<n;j++)reach[i][j] ||= reach[i][k]&&reach[k][j];
  const scc=[],seen=new Set();for(let i=0;i<n;i++){if(seen.has(i))continue;const group=[];for(let j=0;j<n;j++)if(reach[i][j]&&reach[j][i]){group.push(j+1);seen.add(j);}if(group.length>1)scc.push(group);}
  const dp=Array(1<<n).fill(0n);dp[0]=1n;
  for(let mask=0;mask<dp.length;mask++)if(dp[mask])for(let i=0;i<n;i++)if(!(mask&(1<<i))&&(pred[i]&mask)===pred[i])dp[mask|(1<<i)]+=dp[mask];
  let mask=0,order=[];while(order.length<n){let i=pred.findIndex((v,j)=>!(mask&(1<<j))&&(v&mask)===v);if(i<0)break;order.push(i+1);mask|=1<<i;}
  return {edges:edges.length,scc,orders:dp.at(-1).toString(),exampleOrder:order.length===n?order:null};
 }
 function counts(a){const c={};for(const row of a)for(const cell of row)c[cell.kind]=(c[cell.kind]??0)+1;return c;}
 const results=d.modes.map(m=>({mode:m.id,current:graph(current,m),hypothesesActive:graph(current,m,true),target:graph(target,m)}));
 return {current,target,counts:{current:counts(current),target:counts(target)},results};
}
if(process.argv[1] && import.meta.url===pathToFileURL(resolve(process.argv[1])).href){
const result = analyze(data);
const f = n => String(n).padStart(2,"0");
const bad = JSON.parse(JSON.stringify(data));
bad.currentColumns[1].push([2,"X","E01","duplicate"]);
let negativeDetected=false;
try { analyze(bad); } catch { negativeDetected=true; }
if(!negativeDetected || data.names.length!==14) process.exitCode=1;
if(process.argv.includes("--template")){
  const template={schemaVersion:1,baselineCommit:data.baseline,experimentCommit:null,mode:null,workload:null,host:null,configHash:null,metricsVersion:null,planHash:null,confidenceLevel:null,intervalMethod:null,multiplicityPolicy:null,cells:result.current.flatMap((row,i)=>row.map((cell,j)=>({fr:"F"+f(i+1),dp:"D"+f(j+1),structure:cell.kind,target:result.target[i][j].kind,status:"not_run",metric:null,unit:null,delta:null,ciLower:null,ciUpper:null,tolerance:null,independentRuns:null,evidence:null})))};
  console.log(JSON.stringify(template,null,2));
}else{
  console.log(JSON.stringify({counts:result.counts,results:result.results,negativeDetected,current:result.current.map(row=>row.map(c=>c.kind)),target:result.target.map(row=>row.map(c=>c.kind))},null,2));
}

}
