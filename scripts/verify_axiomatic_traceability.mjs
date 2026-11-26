#!/usr/bin/env node
// Checks requirements traceability only, not runtime correctness.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
function check(source) {
 const sections=source.split(/^## /m);
 const find=(name)=>sections.find(s=>s.startsWith(name))??"";
 const req=find("2. 全域需求追踪"), test=find("6. 全域验证计划");
 const ids=[...req.matchAll(/^\| (F\d\d) \/ FR/gm)].map(m=>m[1]);
 const rows=[...test.matchAll(/^\| (T\d\d) [^|]+\| ([^|]+)\|/gm)].map(m=>({id:m[1],refs:[...m[2].matchAll(/F\d\d/g)].map(x=>x[0])}));
 const expected=Array.from({length:14},(_,i)=>"F"+String(i+1).padStart(2,"0"));
 const expectedTests=Array.from({length:10},(_,i)=>"T"+String(i+1).padStart(2,"0"));
 const uncovered=expected.filter(f=>!rows.some(t=>t.refs.includes(f)));
 const unknown=[...new Set(rows.flatMap(t=>t.refs))].filter(f=>!expected.includes(f));
 const pass=ids.join(",")===expected.join(",")&&rows.map(t=>t.id).join(",")===expectedTests.join(",")&&uncovered.length===0&&unknown.length===0&&rows.every(t=>t.refs.length>0);
 return {pass,requirements:ids,tests:rows,uncovered,unknown};
}
const path = resolve(process.argv[2] ?? ".", "docs/architecture/mit-axiomatic-design-system-review.md");
const source = readFileSync(path, "utf8");
const result = check(source);
const negativeDetected = !check(source.replace(/^\| T10 delivery.*$/m,"")).pass;
console.log(JSON.stringify({...result, negativeDetected}, null, 2));
if(!result.pass || !negativeDetected) process.exitCode=1;
