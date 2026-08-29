# CI gate

`bpf-ts smoke` runs Bun unit tests, compiles both checked-in examples to generated libbpf C, then invokes clang's BPF backend for each generated program. This catches both DSL lowering regressions and invalid C/BPF ABI assumptions.
