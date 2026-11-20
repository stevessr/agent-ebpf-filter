import { expect, test } from "bun:test";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import { attentionTimeline, groupAttention } from "../src/composables/behavior/behaviorAttention";
const make=(key:string,overrides:Partial<AgentEvent>={}):AgentEvent=>({key,pid:42,ppid:1,uid:1000,type:"openat",comm:"node",tag:"Agent",path:"/workspace/file",time:"",receivedAtMs:10000,riskScore:70,...overrides});
test("attention grouping preserves evidence while folding true repeats and picks highest-risk record",()=>{
 const input=[make("old-high",{riskScore:95,receivedAtMs:8000}),make("new-low",{riskScore:65,receivedAtMs:9900}),make("separate-session",{agentRunId:"other",riskScore:80}),make("separate-decision",{decision:"BLOCK"})];
 const before=input.map(e=>e.key);
 const groups=groupAttention(input,10000,5000);
 expect(groups).toHaveLength(3);
 const folded=groups.find(g=>g.members.length===2)!;
 expect(folded.representative.key).toBe("old-high");expect(folded.lastAt).toBe(9900);expect(folded.firstAt).toBe(8000);expect(folded.maxRisk).toBe(95);
 expect(folded.members.map(e=>e.key)).toEqual(["old-high","new-low"]);expect(input.map(e=>e.key)).toEqual(before);
});
test("risk and recency ordering are explicit, and policy-only evidence is retained without claiming enforcement",()=>{
 const input=[make("critical",{path:"/critical",riskScore:90,receivedAtMs:8000}),make("recent",{path:"/recent",riskScore:65,receivedAtMs:9999}),make("policy",{path:"/policy",riskScore:undefined,decision:"DENY",retval:0,receivedAtMs:10000})];
 expect(groupAttention(input,10000,5000).map(g=>g.representative.key)).toEqual(["critical","recent","policy"]);
 expect(groupAttention(input,10000,5000,"recent").map(g=>g.representative.key)).toEqual(["policy","recent","critical"]);
 expect(groupAttention(input,10000,5000)[2]?.representative.retval).toBe(0);
});
test("attention timeline excludes unknown/future/stale and routine records, retaining exact boundary evidence",()=>{
 const input=[make("start",{receivedAtMs:5000}),make("end",{receivedAtMs:10000}),make("future",{receivedAtMs:10001}),make("old",{receivedAtMs:4999}),make("unknown",{receivedAtMs:undefined}),make("routine",{riskScore:10})];
 const buckets=attentionTimeline(input,10000,5000,5);
 expect(buckets.reduce((n,b)=>n+b.count,0)).toBe(2);expect(buckets[0]?.representative?.key).toBe("start");expect(buckets.at(-1)?.representative?.key).toBe("end");
 expect(groupAttention(input,10000,5000).reduce((n,g)=>n+g.members.length,0)).toBe(2);
});
test("timeline points to the highest scored evidence in a burst, not an invented threat or a random animation",()=>{
 const buckets=attentionTimeline([make("low",{riskScore:65,receivedAtMs:9900}),make("high",{riskScore:92,receivedAtMs:9800})],10000,5000,2);
 expect(buckets[1]?.count).toBe(2);expect(buckets[1]?.representative?.key).toBe("high");
 expect(attentionTimeline([],10000,5000,999)).toHaveLength(120);
});
