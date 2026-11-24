import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { BEHAVIOR_LANES, laneForEvent, type BehaviorLane } from "../../composables/behavior/behaviorModel";

export const THREE_CAMERA = [7.2, 6.4, 8.8] as const;
export const THREE_TARGET = [0, 0, 0] as const;
export const THREE_MARKS_PER_LANE = 24;
export function laneRadius(index: number): number { return 2.5 + index * .7; }
export function laneHeight(index: number): number { return (index - 1.5) * .68; }
export function eventAngle(now: number, windowMs: number, received: number): number {
  return (-80 + Math.max(0, Math.min(1, (now - received) / Math.max(1, windowMs))) * 300) * Math.PI / 180;
}
export function localOrbitPoint(index: number, angle: number, offset = 0): [number, number, number] {
  const r = laneRadius(index) + offset;
  return [Math.cos(angle) * r, 0, Math.sin(angle) * r];
}
export function worldOrbitPoint(index: number, angle: number, offset = 0): [number, number, number] {
  const [x, , z] = localOrbitPoint(index, angle, offset);
  const tilt = (index - 1.5) * .1;
  return [x * Math.cos(tilt), x * Math.sin(tilt) + laneHeight(index), z];
}
export function boundedThreeEvents(events: readonly AgentEvent[], selectedKey?: string): { event: AgentEvent; lane: BehaviorLane; index: number }[] {
  const counts = [0, 0, 0, 0];
  const result: { event: AgentEvent; lane: BehaviorLane; index: number }[] = [];
  const selected = selectedKey ? events.filter(event => event.key === selectedKey) : [];
  const critical = events.filter(event => Number.isFinite(event.riskScore) && event.riskScore! >= 80).sort((a,b) => b.riskScore! - a.riskScore! || (b.receivedAtMs ?? 0) - (a.receivedAtMs ?? 0));
  const seen = new Set<string>();
  for (const event of [...selected, ...critical, ...events]) {
    if (seen.has(event.key)) continue;
    seen.add(event.key);
    const lane = laneForEvent(event);
    const index = BEHAVIOR_LANES.indexOf(lane);
    if (index < 0 || counts[index]! >= THREE_MARKS_PER_LANE) continue;
    counts[index]!++;
    result.push({ event, lane, index });
  }
  return result;
}
