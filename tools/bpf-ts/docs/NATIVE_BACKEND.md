# Native backend direction

A later backend may lower the same verifier-aware IR directly into eBPF instructions, removing clang from runtime compilation paths. It should remain optional until instruction selection, relocations, BTF and verifier diagnostics reach parity with the C backend.
