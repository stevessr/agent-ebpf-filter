# Initial limits

- Bounded loop policy: 64 iterations.
- Fixed byte-array fields: up to 4096 bytes.
- `bpf.arg`: argument indexes 1 through 5.
- No implicit pointer arithmetic or dynamic indexing.
- Map capacities must be compile-time integers.
