# Maps

`ringbuf<T>(bytes)` emits `BPF_MAP_TYPE_RINGBUF`. `hash<K,V>(entries)` emits `BPF_MAP_TYPE_HASH`. `array<V>(entries)` emits `BPF_MAP_TYPE_ARRAY` with a `u32` key. The first backend supports `emit`, `set`, and `increment` (for `u64` map values).
