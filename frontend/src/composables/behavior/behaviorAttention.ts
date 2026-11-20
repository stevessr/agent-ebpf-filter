import type { AgentEvent } from "../dashboard/dashboardConstants";
import { isAttentionEvent } from "./behaviorModel";
export type AttentionOrder = "risk" | "recent";
export interface AttentionGroup {
 key: string; representative: AgentEvent; members: AgentEvent[]; firstAt: number; lastAt: number; maxRisk: number | null;
}
export interface AttentionBucket { start: number; end: number; count: number; representative: AgentEvent | null }
const score = (event: AgentEvent) => typeof event.riskScore === "number" && Number.isFinite(event.riskScore) ? event.riskScore : null;
export function compareAttention(a: AgentEvent, b: AgentEvent): number {
 return (score(b) ?? -1) - (score(a) ?? -1) || (b.receivedAtMs ?? 0) - (a.receivedAtMs ?? 0) || a.key.localeCompare(b.key);
}
function inWindow(event: AgentEvent, now: number, windowMs: number): boolean {
 const time = event.receivedAtMs;
 return Number.isFinite(time) && time! >= now - windowMs && time! <= now && isAttentionEvent(event);
}
/** Fold repeated activity without erasing distinct evidence or policy/session boundaries. */
export function groupAttention(events: readonly AgentEvent[], now: number, windowMs: number, order: AttentionOrder = "risk"): AttentionGroup[] {
 const groups = new Map<string, AttentionGroup>();
 for (const event of events) {
  if (!inWindow(event, now, windowMs)) continue;
  const key = JSON.stringify([event.pid,event.ppid,event.uid,event.rootAgentPid,event.comm,event.type,event.path,event.netEndpoint,event.decision,event.agentRunId,event.conversationId,event.traceId]);
  let group = groups.get(key);
  if (!group) { group = { key, representative:event, members:[], firstAt:event.receivedAtMs!, lastAt:event.receivedAtMs!, maxRisk:score(event) }; groups.set(key,group); }
  group.members.push(event);
  group.firstAt=Math.min(group.firstAt,event.receivedAtMs!); group.lastAt=Math.max(group.lastAt,event.receivedAtMs!);
  if (compareAttention(event,group.representative)<0) group.representative=event;
  const risk=score(event); if(risk!==null) group.maxRisk=group.maxRisk===null?risk:Math.max(group.maxRisk,risk);
 }
 return [...groups.values()].sort((a,b)=>order==="recent" ? b.lastAt-a.lastAt || compareAttention(a.representative,b.representative) : (b.maxRisk??-1)-(a.maxRisk??-1) || b.lastAt-a.lastAt || a.key.localeCompare(b.key));
}
export function attentionTimeline(events: readonly AgentEvent[], now: number, windowMs: number, size = 24): AttentionBucket[] {
 const count = Math.max(1,Math.min(120,Math.floor(size)||1));
 const duration=Math.max(1,windowMs), start=now-duration;
 const buckets=Array.from({length:count},(_,i)=>({start:start+i*duration/count,end:start+(i+1)*duration/count,count:0,representative:null as AgentEvent|null}));
 for(const event of events){
  if(!inWindow(event,now,duration))continue;
  const bucket=buckets[Math.min(count-1,Math.floor((event.receivedAtMs!-start)/duration*count))]!;
  bucket.count++; if(!bucket.representative || compareAttention(event,bucket.representative)<0)bucket.representative=event;
 }
 return buckets;
}
