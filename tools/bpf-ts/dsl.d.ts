// Editor-only declarations for the bpf-ts DSL. The compiler parses source
// directly with the TypeScript compiler API and accepts only a strict subset.

type u8 = number;
type u16 = number;
type u32 = number;
type u64 = number;
type i8 = number;
type i16 = number;
type i32 = number;
type i64 = number;
type bytes<N extends number> = Uint8Array & { readonly __bpfLength?: N };

type ProbeDecorator = (...args: any[]) => any;

declare function kprobe(symbol: string): ProbeDecorator;
declare function uprobe(symbol: string): ProbeDecorator;
declare function tracepoint(category: string, event: string): ProbeDecorator;

interface ProbeContext {}
interface KProbeContext extends ProbeContext {}
interface UProbeContext extends ProbeContext {}
interface TracepointContext extends ProbeContext {}

interface RingbufMap<T> {
  emit(value: T): void;
}

interface KeyValueMap<K, V> {
  set(key: K, value: V): void;
  increment(key: K): void;
}

declare function ringbuf<T>(bytes: number): RingbufMap<T>;
declare function hash<K, V>(maxEntries: number): KeyValueMap<K, V>;
declare function array<V>(maxEntries: number): KeyValueMap<u32, V>;

declare const bpf: {
  pid(): u32;
  tid(): u32;
  uid(): u32;
  gid(): u32;
  ktimeNs(): u64;
  arg(ctx: ProbeContext, index: 1 | 2 | 3 | 4 | 5): u64;
  comm(): bytes<16>;
  userString(pointer: u64): bytes<256>;
  readUser<T>(target: T, pointer: u64): void;
};
