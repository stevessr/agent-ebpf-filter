/**
 * Bounded top-K selection for live AgentSight graph previews.
 *
 * Retains the same ordering as a full priority sort without sorting/copying
 * the entire capture history. A max-heap keeps the *worst* retained entry
 * at the root, so a new high-priority event replaces it in O(log K).
 *
 * Priority is ascending (0 = most important), timestamp is descending,
 * and stable event identity breaks ties deterministically.
 */
export interface CaptureSelectionItem {
  timestamp: number;
}

interface RankedCapture<T> {
  event: T;
  priority: number;
  timestamp: number;
  key: string;
}

function compareRank<T>(a: RankedCapture<T>, b: RankedCapture<T>): number {
  return a.priority - b.priority ||
    b.timestamp - a.timestamp ||
    a.key.localeCompare(b.key);
}

const chronology = <T>(a: RankedCapture<T>, b: RankedCapture<T>): number =>
  a.timestamp - b.timestamp || a.key.localeCompare(b.key);

export function selectBoundedCaptureEvents<T extends CaptureSelectionItem>(
  events: readonly T[],
  limit: number,
  priority: (event: T) => number,
  identity: (event: T) => string,
): T[] {
  const budget = Math.max(0, Math.floor(Number.isFinite(limit) ? limit : 0));
  if (!budget || !events.length) return [];
  const ranked = (event: T): RankedCapture<T> => ({
    event,
    priority: priority(event),
    timestamp: Number.isFinite(event.timestamp) ? event.timestamp : 0,
    key: identity(event),
  });

  if (events.length <= budget) {
    return events.map(ranked).sort(chronology).map((entry) => entry.event);
  }

  // Capture buffers are often append-only and strictly chronological.
  // For those snapshots, a reverse scan plus four small priority buckets
  // beats both full sorting and a heap: O(N + K log K), O(K) memory.
  // Duplicate timestamps or reordered imports use the general heap below,
  // whose identity tie-breaker remains fully deterministic.
  let direction = 0;
  for (let i = 1; i < events.length; i++) {
    const current = events[i].timestamp;
    const previous = events[i - 1].timestamp;
    const delta = current - previous;
    if (!Number.isFinite(delta) || delta === 0) {
      direction = 0;
      break;
    }
    const sign = delta > 0 ? 1 : -1;
    if (direction && direction !== sign) {
      direction = 0;
      break;
    }
    direction = sign;
  }
  if (direction) {
    const buckets: T[][] = [[], [], [], []];
    let categorical = true;
    for (let i = direction === 1 ? events.length - 1 : 0;
      direction === 1 ? i >= 0 : i < events.length;
      i += direction === 1 ? -1 : 1) {
      const event = events[i];
      const level = priority(event);
      if (!Number.isInteger(level) || level < 0 || level > 3) {
        categorical = false;
        break;
      }
      if (buckets[level].length < budget) buckets[level].push(event);
    }
    if (categorical) {
      const top: T[] = [];
      for (const bucket of buckets) {
        for (const event of bucket) {
          top.push(event);
          if (top.length >= budget) break;
        }
        if (top.length >= budget) break;
      }
      return top.sort((a, b) => a.timestamp - b.timestamp);
    }
  }

  const heap: RankedCapture<T>[] = [];
  const bubbleUp = (start: number) => {
    let index = start;
    while (index > 0) {
      const parent = (index - 1) >> 1;
      if (compareRank(heap[index], heap[parent]) <= 0) break;
      [heap[index], heap[parent]] = [heap[parent], heap[index]];
      index = parent;
    }
  };
  const sinkRoot = () => {
    let index = 0;
    for (;;) {
      const left = index * 2 + 1;
      if (left >= heap.length) return;
      const right = left + 1;
      const worst = right < heap.length &&
        compareRank(heap[right], heap[left]) > 0 ? right : left;
      if (compareRank(heap[worst], heap[index]) <= 0) return;
      [heap[index], heap[worst]] = [heap[worst], heap[index]];
      index = worst;
    }
  };
  for (const event of events) {
    const item = ranked(event);
    if (heap.length < budget) {
      heap.push(item);
      bubbleUp(heap.length - 1);
    } else if (compareRank(item, heap[0]) < 0) {
      heap[0] = item;
      sinkRoot();
    }
  }
  return heap.sort(chronology).map((entry) => entry.event);
}
