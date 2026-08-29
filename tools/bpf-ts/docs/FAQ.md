# FAQ

**Is this full TypeScript?** No. It is a verifier-aware TypeScript-like surface language.

**Does it bypass the kernel verifier?** No. Generated ELF still goes through normal verifier checks.

**Why C first?** It reuses the repository's existing clang/libbpf/bpf2go toolchain and makes lowering easy to inspect.
