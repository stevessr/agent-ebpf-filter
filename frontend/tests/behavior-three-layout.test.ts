import { expect, test } from "bun:test";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import { boundedThreeEvents, eventAngle, worldOrbitPoint, laneHeight, laneRadius } from "../src/components/behavior/behaviorThreeLayout";
function event(index: number, type: string): AgentEvent { return { key: type + index, pid: 42, ppid: 1, uid: 1000, type, comm: "node", tag: "Agent", path: "", time: "", receivedAtMs: 100000 - index }; }
test("3D projection preserves distinct syscall evidence and caps each family independently", () => {
 const input = Array.from({length:1000}, (_,i)=> event(i, ["read","network_connect","execve","stdio"][i%4]!));
 const order = input.map(e=>e.key);
 const marks = boundedThreeEvents(input);
 expect(marks).toHaveLength(96);
 for (let i=0;i<4;i++) expect(marks.filter(mark=>mark.index===i)).toHaveLength(24);
 expect(new Set(marks.map(mark=>mark.event.key)).size).toBe(96);
 expect(input.map(e=>e.key)).toEqual(order);
 expect(marks[0]?.event).toBe(input[0]);
});
test("selected and high-risk evidence stay visible inside the bounded 3D budget", () => {
 const input=Array.from({length:80},(_,i)=>event(i,"read"));
 input[70]!.riskScore=95;
 const selected=input[79]!;
 const marks=boundedThreeEvents(input,selected.key);
 expect(marks).toHaveLength(24);
 expect(marks.some(mark=>mark.event===selected)).toBe(true);
 expect(marks.some(mark=>mark.event===input[70])).toBe(true);
 expect(marks.some(mark=>mark.event===input[0])).toBe(true);
 expect(new Set(marks.map(mark=>mark.event.key)).size).toBe(24);
});

test("3D event age has bounded chronological positions, independent of animation rate", () => {
 expect(eventAngle(100000,60000,100000)).toBeCloseTo(-80*Math.PI/180);
 expect(eventAngle(100000,60000,40000)).toBeCloseTo(220*Math.PI/180);
 expect(eventAngle(100000,60000,100001)).toBeCloseTo(eventAngle(100000,60000,100000));
 expect(eventAngle(100000,60000,1)).toBeCloseTo(eventAngle(100000,60000,40000));
 expect(eventAngle(100000,30000,70000)).toBeCloseTo(eventAngle(100000,60000,40000));
});
test("inclined 3D lanes retain radius and distinct height rather than encoding false relationships", () => {
 for(let i=0;i<4;i++){
  const [x,y,z] = worldOrbitPoint(i, .9);
  expect(Math.hypot(x,y-laneHeight(i),z)).toBeCloseTo(laneRadius(i));
  expect([x,y,z].every(Number.isFinite)).toBe(true);
 }
 expect(boundedThreeEvents([event(0,"opaque_signal")])[0]?.lane).toBe("other");
});
