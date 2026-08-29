# Debugging

Keep generated `.bpf.c` as the primary lowering diagnostic. Compiler errors should point to TypeScript source locations; clang errors point to generated C until source mapping is implemented. The CI smoke test compiles generated C with clang in addition to running TypeScript-side unit tests.
