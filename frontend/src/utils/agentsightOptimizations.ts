// AgentSight Performance Optimizations

/**
 * Virtual scrolling helper for large lists
 * Only renders visible items + buffer
 */
export interface VirtualScrollConfig {
  itemHeight: number;
  containerHeight: number;
  buffer?: number;
}

export function calculateVisibleRange(
  scrollTop: number,
  config: VirtualScrollConfig,
  totalItems: number
): { start: number; end: number; offsetY: number } {
  const { itemHeight, containerHeight, buffer = 5 } = config;
  const visibleCount = Math.ceil(containerHeight / itemHeight);
  const start = Math.max(0, Math.floor(scrollTop / itemHeight) - buffer);
  const end = Math.min(totalItems, start + visibleCount + buffer * 2);
  const offsetY = start * itemHeight;

  return { start, end, offsetY };
}

/**
 * Debounce helper for expensive operations
 */
export function debounce<T extends (...args: any[]) => any>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: ReturnType<typeof setTimeout> | null = null;

  return function executedFunction(...args: Parameters<T>) {
    const later = () => {
      timeout = null;
      func(...args);
    };

    if (timeout) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(later, wait);
  };
}

/**
 * Throttle helper for frequent events (scroll, resize)
 */
export function throttle<T extends (...args: any[]) => any>(
  func: T,
  limit: number
): (...args: Parameters<T>) => void {
  let inThrottle: boolean;

  return function executedFunction(...args: Parameters<T>) {
    if (!inThrottle) {
      func(...args);
      inThrottle = true;
      setTimeout(() => (inThrottle = false), limit);
    }
  };
}

/**
 * Memoization cache with size limit (LRU-like)
 */
export class MemoCache<K, V> {
  private cache = new Map<K, V>();
  private maxSize: number;

  constructor(maxSize = 100) {
    this.maxSize = maxSize;
  }

  get(key: K): V | undefined {
    if (!this.cache.has(key)) return undefined;
    // Move to end (most recently used)
    const value = this.cache.get(key)!;
    this.cache.delete(key);
    this.cache.set(key, value);
    return value;
  }

  set(key: K, value: V): void {
    if (this.cache.has(key)) {
      this.cache.delete(key);
    } else if (this.cache.size >= this.maxSize) {
      // Remove oldest (first) entry
      const firstKey = this.cache.keys().next().value;
      if (firstKey !== undefined) {
        this.cache.delete(firstKey);
      }
    }
    this.cache.set(key, value);
  }

  clear(): void {
    this.cache.clear();
  }

  get size(): number {
    return this.cache.size;
  }
}

/**
 * Batch DOM updates using requestAnimationFrame
 */
export class BatchUpdater {
  private pending = new Set<() => void>();
  private scheduled = false;

  add(callback: () => void): void {
    this.pending.add(callback);
    if (!this.scheduled) {
      this.scheduled = true;
      requestAnimationFrame(() => this.flush());
    }
  }

  private flush(): void {
    this.scheduled = false;
    const callbacks = Array.from(this.pending);
    this.pending.clear();
    callbacks.forEach((cb) => cb());
  }
}

/**
 * Optimize array operations for large datasets
 */
export function fastFilter<T>(
  array: T[],
  predicate: (item: T, index: number) => boolean
): T[] {
  const result: T[] = [];
  for (let i = 0; i < array.length; i++) {
    if (predicate(array[i], i)) {
      result.push(array[i]);
    }
  }
  return result;
}

export function fastMap<T, U>(
  array: T[],
  mapper: (item: T, index: number) => U
): U[] {
  const result = new Array<U>(array.length);
  for (let i = 0; i < array.length; i++) {
    result[i] = mapper(array[i], i);
  }
  return result;
}

/**
 * String search optimization using indexOf (faster than includes for large strings)
 */
export function fastStringSearch(text: string, search: string): boolean {
  return text.indexOf(search) !== -1;
}

/**
 * Object pool for reducing allocations
 */
export class ObjectPool<T> {
  private available: T[] = [];
  private factory: () => T;
  private reset: (obj: T) => void;

  constructor(factory: () => T, reset: (obj: T) => void, initialSize = 10) {
    this.factory = factory;
    this.reset = reset;
    for (let i = 0; i < initialSize; i++) {
      this.available.push(factory());
    }
  }

  acquire(): T {
    return this.available.pop() || this.factory();
  }

  release(obj: T): void {
    this.reset(obj);
    this.available.push(obj);
  }

  clear(): void {
    this.available = [];
  }
}
