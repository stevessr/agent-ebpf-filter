# Reproducibility

The package pins the TypeScript compiler version. CI uses a fixed Ubuntu runner family and explicit clang/libbpf packages; generated C is deterministic for a given source and compiler revision.
