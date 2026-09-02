# Testing

The PoC has TypeScript-side unit tests for lowering and rejection policies. CI additionally compiles generated C with clang's BPF target so syntax and libbpf macro regressions are caught before loader integration.
