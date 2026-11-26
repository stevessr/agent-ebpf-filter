#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { analyze, data } from "./verify_axiomatic_matrix.mjs";
function verifyMatrix(analyze,data,document){
 const assert=(x,msg)=>{if(!x)throw Error(msg)};
 const results=analyze(data);
 assert(data.version===1&&data.names.length===14&&/^[0-9a-f]{40}$/.test(data.baseline),"matrix schema");
 for(const c of [data.currentColumns,data.targetColumns])assert(Object.keys(c).sort((a,b)=>+a-+b).join(",")==="1,2,3,4,5,6,7,8,9,10,11,12,13,14","missing column");
 const row=(a,i)=>`| F${String(i+1).padStart(2,"0")} | ${a.map(x=>x.kind).join(" | ")} |`;
 const expected=[...results.current.map(row),...results.target.map(row)];
 const actual=document.match(/^\| F\d\d \| (?:[DXWHCBI?AZ] \| ){13}[DXWHCBI?AZ] \|$/gm)??[];
 assert(actual.join("\n")===expected.join("\n"),"markdown matrix drift");
 const modes=["observe","wrapper","feedback","combined","experimental_ipc"];
 const tuples=[[false,false,false],[true,false,false],[false,true,false],[true,true,false],[false,false,true]];
 assert(JSON.stringify(data.modes.map(m=>[m.id,m.wrapper,m.feedback,m.ipc]))===JSON.stringify(modes.map((m,i)=>[m,...tuples[i]])),"mode drift");
 const perm=a=>!a.length?[[]]:a.flatMap((v,i)=>perm(a.filter((_,j)=>j!==i)).map(p=>[v,...p]));
 const perms=perm([1,2,3,4]),possible=[];
 for(let i=1;i<=4;i++)for(let j=1;j<=4;j++)if(i!==j)possible.push([i,j]);
 const minimal={names:["1","2","3","4"],modes:[{id:"test",wrapper:false,feedback:false,ipc:false}],evidence:{e:["x","1","test"]},targetColumns:{}};
 let graphs=0;
 for(let mask=0;mask<1<<possible.length;mask++){
  const edges=possible.filter((_,i)=>mask&(1<<i)),cols={};
  for(const [u,v] of edges)(cols[u]??=[]).push([v,"X","e","fixture"]);
  const g=analyze({...minimal,currentColumns:cols}).results[0].current;
  const valid=perms.filter(p=>edges.every(([u,v])=>p.indexOf(u)<p.indexOf(v)));
  const reachable=(u,v)=>{const seen=new Set([u]),queue=[u];while(queue.length){const a=queue.shift();for(const [x,y] of edges)if(x===a&&!seen.has(y)){seen.add(y);queue.push(y)}}return seen.has(v)};
  const groups=[],seen=new Set();for(let i=1;i<=4;i++){if(seen.has(i))continue;const group=[1,2,3,4].filter(j=>reachable(i,j)&&reachable(j,i));group.forEach(j=>seen.add(j));if(group.length>1)groups.push(group)}
  assert(g.orders===String(valid.length)&&g.edges===edges.length&&JSON.stringify(g.scc)===JSON.stringify(groups),"graph mismatch "+mask);
  assert(valid.length?valid.some(p=>p.join()===g.exampleOrder?.join()):g.exampleOrder===null,"invalid example "+mask);
  graphs++;
 }
 // Independent truth table for mode-conditioned edges.
 const kinds=["X","W","C","B","I","H"],expectedByKind=[[1,1,1,1,1],[0,1,0,1,0],[0,0,1,1,0],[0,1,1,1,0],[0,0,0,0,1],[0,0,0,0,0]];
 for(let k=0;k<kinds.length;k++){
  const g=analyze({...minimal,modes:data.modes,currentColumns:{1:[[2,kinds[k],"e","fixture"]]}}).results;
  assert(g.every((m,i)=>m.current.edges===expectedByKind[k][i]&&m.hypothesesActive.edges===(k===5?1:expectedByKind[k][i])),"mode classification "+kinds[k]);
 }
 const mutate=[x=>x.currentColumns[1].push([2,"X","E01","duplicate"]),x=>x.currentColumns[1].push([1.5,"X","E01","fractional"]),x=>x.currentColumns[1].push([7,"X","missing","bad evidence"]),x=>x.targetColumns[1].push([1,"Z","diagonal"]),x=>x.modes[0].wrapper="false",x=>x.names[1]=x.names[0]];
 for(const m of mutate){const bad=JSON.parse(JSON.stringify(data));m(bad);let rejected=false;try{analyze(bad)}catch{rejected=true}assert(rejected,"malformed matrix accepted")}
 return {matrixCells:expected.length*14,graphs,oracle:"all directed loop-free four-node graphs; permutation and BFS oracle",modeCases:kinds.length*data.modes.length,negativeCases:mutate.length};
}
const document=readFileSync(new URL("../docs/architecture/mit-axiomatic-design-matrix.md",import.meta.url),"utf8");
console.log(JSON.stringify(verifyMatrix(analyze,data,document),null,2));
