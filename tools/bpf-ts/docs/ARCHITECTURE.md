# Architecture

`source.ts -> TypeScript AST -> verifier-aware BPF IR -> restricted libbpf C -> clang -target bpf -> ELF/BTF`

The AST parser owns language restrictions. The IR is backend-neutral. The current C backend is deliberately simple and debuggable; generated C is a first-class diagnostic artifact. Attach metadata is emitted separately as a JSON manifest so userspace loaders do not need to reverse-engineer section names or comments.
