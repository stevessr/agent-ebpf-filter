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
declare function kretprobe(symbol: string): ProbeDecorator;
declare function uprobe(symbol: string): ProbeDecorator;
declare function uretprobe(symbol: string): ProbeDecorator;
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
  getOr(key: K, fallback: V): V;
  /** Copy the current hash value and delete the entry before returning it. */
  takeOr(key: K, fallback: V): V;
  delete(key: K): void;
  increment(key: K): void;
}

type CoreFieldReader = (pointer: u64) => u64;
type CoreStructReader = Record<string, CoreFieldReader>;

declare function ringbuf<T>(bytes: number): RingbufMap<T>;
declare function hash<K, V>(maxEntries: number): KeyValueMap<K, V>;
declare function array<V>(maxEntries: number): KeyValueMap<u32, V>;

declare const bpf: {
  pid(): u32;
  tid(): u32;
  uid(): u32;
  gid(): u32;
  ktimeNs(): u64;
  currentTask(): u64;
  arg(ctx: ProbeContext, index: 1 | 2 | 3 | 4 | 5): u64;
  /** Read an ABI register argument and truncate/sign-extend it as a C int. */
  argI32(ctx: ProbeContext, index: 1 | 2 | 3 | 4 | 5): i32;
  ret(ctx: ProbeContext): i64;
  retI32(ctx: ProbeContext): i32;
  comm(): bytes<16>;
  userString(pointer: u64): bytes<256>;
  userBytes(pointer: u64, length: u64): bytes<4096>;
  readUser<T>(target: T, pointer: u64): void;
  coreRead: Record<string, CoreStructReader>;
};
