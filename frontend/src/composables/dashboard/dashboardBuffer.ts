// Both live reception and history catch-up use bounded queues. Avoid spreading
// an arbitrarily large WebSocket batch into push() (argument limit / memory).
export function appendBounded<T>(
  queue: T[],
  incoming: readonly T[],
  capacity: number,
): number {
  const cap = Math.max(1, Math.floor(capacity));
  const removed = Math.max(0, queue.length + incoming.length - cap);
  if (incoming.length >= cap) {
    queue.length = 0;
    for (let i = incoming.length - cap; i < incoming.length; i++)
      queue.push(incoming[i]!);
  } else {
    if (removed) queue.splice(0, removed);
    for (const item of incoming) queue.push(item);
  }
  return removed;
}

// Inputs may overlap after reconnect. IDs, not display/grouping keys, identify
// duplicates. The set is scoped to the retained window, never session-global.
export function mergeDashboardRecords<
  T extends { eventId?: string; receivedAtMs?: number },
>(
  current: readonly T[],
  incoming: readonly T[],
  capacity: number,
): { records: T[]; evicted: number } {
  const seen = new Set<string>();
  const records: T[] = [];
  for (const record of [...incoming, ...current]) {
    if (record.eventId) {
      if (seen.has(record.eventId)) continue;
      seen.add(record.eventId);
    }
    records.push(record);
  }
  records.sort((a, b) => (b.receivedAtMs ?? 0) - (a.receivedAtMs ?? 0));
  const cap = Math.max(1, Math.floor(capacity));
  const evicted = Math.max(0, records.length - cap);
  records.length = Math.min(records.length, cap);
  return { records, evicted };
}
