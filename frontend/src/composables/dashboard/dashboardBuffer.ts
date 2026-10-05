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
