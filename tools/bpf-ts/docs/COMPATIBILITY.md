# Compatibility

The current C backend targets the same Linux/libbpf/clang toolchain already used by agent-ebpf-filter. The CI backend initially validates x86_64 argument-register lowering; ARM64 register lowering will be introduced explicitly rather than guessed.
