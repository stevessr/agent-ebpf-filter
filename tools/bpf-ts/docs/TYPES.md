# Types

Scalar aliases: `u8/u16/u32/u64`, `i8/i16/i32/i64`, `bool`. Fixed byte arrays use `bytes<N>` with `1 <= N <= 4096`. Interfaces lower to fixed-layout C structs; optional fields, inheritance and generic structs are rejected.
